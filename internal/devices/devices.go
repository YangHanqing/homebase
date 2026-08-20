// Package devices implements enrollment: a one-time, short-lived pairing token
// minted by someone who can already run commands on this machine, redeemed by a
// browser for a long-lived session cookie.
//
// The security argument is that enrollment grants nothing new. Homebase hands
// out a shell, so the bar for getting in is "already able to run a command
// here" — which is exactly what minting a token requires. Nobody escalates by
// pairing.
//
// State lives in devices.json next to config.json because the `homebase pair`
// CLI and the running server are different processes. Both sides re-read the
// file before mutating; the only writers are human-triggered (mint, redeem,
// revoke), so there is no routine write contention. LastSeen is deliberately
// in-memory only — persisting it would add a background writer racing the CLI.
package devices

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yanghanqing/homebase/internal/config"
)

// TokenTTL bounds how long a minted pairing token stays redeemable.
const TokenTTL = 10 * time.Minute

// SchemaVersion of devices.json.
const SchemaVersion = 1

var (
	// ErrBadToken covers unknown, expired, and already-consumed tokens alike:
	// the caller must not be told which, and none of them are recoverable
	// without minting a new one.
	ErrBadToken = errors.New("pairing token is invalid or expired")
	ErrNotFound = errors.New("device not found")
)

// Token is a pending, unredeemed pairing token. Only its hash is stored, for
// the same reason passwords are hashed: devices.json should not be a bearer
// credential.
type Token struct {
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Device is one enrolled browser. Hash is the SHA-256 of its session secret.
type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"created_at"`

	// LastSeen is process-local and never persisted; see package doc.
	LastSeen time.Time `json:"-"`
}

type file struct {
	Version int      `json:"version"`
	Pending []Token  `json:"pending"`
	Devices []Device `json:"devices"`
}

// Store is the devices.json accessor, safe for concurrent use in one process.
type Store struct {
	mu   sync.Mutex
	path string
	f    file

	// stamp is the mtime+size of the loaded file. Cached lookups are only
	// served while it still matches what is on disk, so a revocation made by
	// another process takes effect on the next request rather than whenever
	// this one happens to miss its cache.
	stamp string
}

// DefaultPath returns devices.json alongside the given config.json.
func DefaultPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "devices.json")
}

// Open loads path, treating a missing file as empty. A corrupt file is an
// error: silently starting over would revoke every paired device without
// saying so.
func Open(path string) (*Store, error) {
	s := &Store{path: path}
	if err := s.reloadLocked(); err != nil {
		return nil, err
	}
	return s, nil
}

// stampOf identifies the on-disk revision cheaply, without reading contents.
func (s *Store) stampOf() string {
	st, err := os.Stat(s.path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d/%d", st.ModTime().UnixNano(), st.Size())
}

// reloadIfChangedLocked re-reads only when the file moved on from what is
// cached. Returns false when the cache is known to be stale but unreadable.
func (s *Store) reloadIfChangedLocked() bool {
	if s.stampOf() == s.stamp {
		return true
	}
	return s.reloadLocked() == nil
}

func (s *Store) reloadLocked() error {
	stamp := s.stampOf()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.f = file{Version: SchemaVersion}
			s.stamp = stamp
			return nil
		}
		return err
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("devices file is corrupt: %w", err)
	}
	if f.Version == 0 {
		f.Version = SchemaVersion
	}
	// Carry over in-memory LastSeen for devices that survived the reload.
	prev := make(map[string]time.Time, len(s.f.Devices))
	for _, d := range s.f.Devices {
		prev[d.ID] = d.LastSeen
	}
	for i := range f.Devices {
		f.Devices[i].LastSeen = prev[f.Devices[i].ID]
	}
	s.f = f
	s.stamp = stamp
	return nil
}

func (s *Store) persistLocked() error {
	s.f.Version = SchemaVersion
	if s.f.Pending == nil {
		s.f.Pending = []Token{}
	}
	if s.f.Devices == nil {
		s.f.Devices = []Device{}
	}
	data, err := json.MarshalIndent(s.f, "", "  ")
	if err != nil {
		return err
	}
	return config.AtomicWrite(s.path, append(data, '\n'))
}

// Mint creates a pairing token and returns the plaintext, which is the only
// time it exists outside a hash. Expired tokens are pruned on the way.
func (s *Store) Mint(now time.Time) (string, time.Time, error) {
	tok, err := randomSecret()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := now.Add(TokenTTL)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reloadLocked(); err != nil {
		return "", time.Time{}, err
	}
	s.f.Pending = prune(s.f.Pending, now)
	s.f.Pending = append(s.f.Pending, Token{
		Hash:      hashSecret(tok),
		CreatedAt: now,
		ExpiresAt: expires,
	})
	if err := s.persistLocked(); err != nil {
		return "", time.Time{}, err
	}
	return tok, expires, nil
}

// Redeem consumes token exactly once and enrolls a device, returning the
// session secret the browser should store in its cookie.
//
// Consumption and enrollment happen under one lock and one atomic write, so two
// browsers racing the same token cannot both win.
func (s *Store) Redeem(token, name string, now time.Time) (secret string, dev Device, err error) {
	if token == "" {
		return "", Device{}, ErrBadToken
	}
	want := hashSecret(token)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reloadLocked(); err != nil {
		return "", Device{}, err
	}

	idx := -1
	for i, t := range s.f.Pending {
		if t.ExpiresAt.After(now) && constEq(t.Hash, want) {
			idx = i
			break
		}
	}
	if idx < 0 {
		// Still prune, so a burst of failed attempts cannot keep dead tokens
		// around, but do not persist just for that.
		return "", Device{}, ErrBadToken
	}

	secret, err = randomSecret()
	if err != nil {
		return "", Device{}, err
	}
	dev = Device{
		ID:        shortID(),
		Name:      deviceName(name),
		Hash:      hashSecret(secret),
		CreatedAt: now,
		LastSeen:  now,
	}

	prevPending, prevDevices := s.f.Pending, s.f.Devices
	s.f.Pending = prune(append(append([]Token{}, prevPending[:idx]...), prevPending[idx+1:]...), now)
	s.f.Devices = append(append([]Device{}, prevDevices...), dev)
	if err := s.persistLocked(); err != nil {
		s.f.Pending, s.f.Devices = prevPending, prevDevices
		return "", Device{}, err
	}
	return secret, dev, nil
}

// Verify resolves a session secret to a device. It re-reads devices.json
// whenever the file has changed, so a token redeemed by another process — or a
// revocation — takes effect immediately, while an unchanged file costs one stat.
func (s *Store) Verify(secret string, now time.Time) (Device, bool) {
	if secret == "" {
		return Device{}, false
	}
	want := hashSecret(secret)

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.reloadIfChangedLocked() {
		return Device{}, false
	}
	return s.findLocked(want, now)
}

func (s *Store) findLocked(wantHash string, now time.Time) (Device, bool) {
	for i := range s.f.Devices {
		if constEq(s.f.Devices[i].Hash, wantHash) {
			s.f.Devices[i].LastSeen = now
			return s.f.Devices[i], true
		}
	}
	return Device{}, false
}

// List returns the enrolled devices, freshest state first read from disk.
func (s *Store) List() ([]Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reloadLocked(); err != nil {
		return nil, err
	}
	out := make([]Device, len(s.f.Devices))
	copy(out, s.f.Devices)
	return out, nil
}

// Revoke removes one device by id. Its cookie stops working on the next
// request that misses the in-memory cache.
func (s *Store) Revoke(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reloadLocked(); err != nil {
		return err
	}
	idx := -1
	for i, d := range s.f.Devices {
		if d.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrNotFound
	}
	prev := s.f.Devices
	s.f.Devices = append(append([]Device{}, prev[:idx]...), prev[idx+1:]...)
	if err := s.persistLocked(); err != nil {
		s.f.Devices = prev
		return err
	}
	return nil
}

// RevokeAll drops every device and pending token.
func (s *Store) RevokeAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reloadLocked(); err != nil {
		return err
	}
	s.f.Pending = []Token{}
	s.f.Devices = []Device{}
	return s.persistLocked()
}

func prune(in []Token, now time.Time) []Token {
	out := in[:0:0]
	for _, t := range in {
		if t.ExpiresAt.After(now) {
			out = append(out, t)
		}
	}
	return out
}

func randomSecret() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func hashSecret(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func constEq(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func shortID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("dev-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func deviceName(s string) string {
	const max = 64
	out := make([]rune, 0, max)
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			continue
		}
		out = append(out, r)
		if len(out) == max {
			break
		}
	}
	if len(out) == 0 {
		return "unnamed device"
	}
	return string(out)
}
