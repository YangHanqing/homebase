// Package config loads and atomically persists ~/.config/homebase/config.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	SchemaVersion = 4
	DefaultPort   = 1990
	fileMode      = 0o600
	dirMode       = 0o700
	// MaxTrustedRanges caps the trusted_ranges list so a user cannot paste in
	// an unbounded file and turn every request into a CIDR scan.
	MaxTrustedRanges = 8
)

// ErrCorrupt is returned when the file exists but is not usable.
var (
	ErrCorrupt      = errors.New("config file is corrupt")
	ErrUnsupported  = errors.New("config version is newer than this binary")
	ErrAccess       = errors.New("invalid access tier")
	ErrTrustedRange = errors.New("invalid trusted range")
)

// Access is the single knob that decides where Homebase binds. There is no
// HTTPS in this build — see docs — so Access only decides which addresses the
// listener accepts connections on, and which of those addresses is treated as
// already-encrypted for the purpose of the risk warning shown in Settings.
type Access string

const (
	// AccessLocal binds 127.0.0.1. Nothing leaves the machine.
	AccessLocal Access = "local"
	// AccessPrivate binds the first local address that falls inside
	// TrustedRanges (Tailscale/WireGuard/etc — see TrustedRanges). Falls back
	// to loopback if no such address is found.
	AccessPrivate Access = "private"
	// AccessLAN binds 0.0.0.0. Traffic is plaintext HTTP; anyone who can reach
	// the machine on the network can reach the shell. Requires the user to
	// have explicitly acknowledged the risk (see Settings/README).
	AccessLAN Access = "lan"
)

// ValidAccess reports whether a is a known tier.
func ValidAccess(a Access) bool {
	switch a {
	case AccessLocal, AccessPrivate, AccessLAN:
		return true
	}
	return false
}

// DefaultTrustedRanges is the out-of-the-box trusted range: Tailscale's CGNAT
// allocation. Users on a different overlay network (self-hosted WireGuard,
// Headscale, ZeroTier, ...) replace this via Settings with their own CIDR(s).
var DefaultTrustedRanges = []string{"100.64.0.0/10"}

// File is the on-disk document.
type File struct {
	Version       int      `json:"version"`
	Access        Access   `json:"access"`
	ListenAddr    string   `json:"listen_addr"`
	ListenPort    int      `json:"listen_port"`
	TrustedRanges []string `json:"trusted_ranges"`
}

// Store is the process-wide config mutex + path.
type Store struct {
	mu   sync.Mutex
	path string
	file File
}

// DefaultPath is $XDG_CONFIG_HOME/homebase/config.json or ~/.config/homebase/config.json.
func DefaultPath() string {
	return filepath.Join(DefaultDir(), "config.json")
}

// DefaultDir is the directory holding config.json and devices.json.
func DefaultDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "homebase")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".config", "homebase")
}

// DefaultFile is the empty document written when the file is missing.
// A fresh install is loopback-only: usable immediately, exposed to nothing.
func DefaultFile() File {
	return File{
		Version:       SchemaVersion,
		Access:        AccessLocal,
		ListenAddr:    "",
		ListenPort:    DefaultPort,
		TrustedRanges: append([]string{}, DefaultTrustedRanges...),
	}
}

// legacy mirrors fields from older schema versions that this version dropped,
// so an old config file loads instead of erroring. Their values are not
// migrated into anything — v4 has no TLS and no multi-host — they are simply
// dropped silently.
type legacy struct {
	AllowPublicBind bool `json:"allow_public_bind"`
}

// Load opens path. Missing file: create parent 0700, write default 0600, continue.
// Existing corrupt JSON or version newer than this binary: error, do not overwrite.
//
// A pre-v4 file loads fine: unknown/removed fields (tls, hosts) are simply
// dropped, and a pre-v3 allow_public_bind is translated into an access tier
// that preserves the old bind behavior.
func Load(path string) (*Store, error) {
	s := &Store{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		s.file = DefaultFile()
		if err := s.persistLocked(); err != nil {
			return nil, err
		}
		return s, nil
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if f.Version == 0 {
		f.Version = SchemaVersion
	}
	if f.Version > SchemaVersion {
		return nil, fmt.Errorf("%w: got %d", ErrUnsupported, f.Version)
	}
	if f.Access == "" {
		var l legacy
		_ = json.Unmarshal(data, &l)
		if l.AllowPublicBind {
			f.Access = AccessLAN
		} else {
			f.Access = AccessPrivate
		}
	}
	if !ValidAccess(f.Access) {
		return nil, fmt.Errorf("%w: %q (want local, private, or lan)", ErrAccess, f.Access)
	}
	f.Version = SchemaVersion
	if f.ListenPort == 0 {
		f.ListenPort = DefaultPort
	}
	if len(f.TrustedRanges) == 0 {
		f.TrustedRanges = append([]string{}, DefaultTrustedRanges...)
	}
	s.file = f
	return s, nil
}

// Path returns the backing file path.
func (s *Store) Path() string {
	return s.path
}

// Dir returns the directory holding the config file.
func (s *Store) Dir() string {
	return filepath.Dir(s.path)
}

// Snapshot returns a copy of the document.
func (s *Store) Snapshot() File {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneFile(s.file)
}

// SetAccess switches tiers and persists. The caller is responsible for telling
// the operator to restart: rebinding a live listener is not supported.
func (s *Store) SetAccess(a Access) error {
	if !ValidAccess(a) {
		return fmt.Errorf("%w: %q", ErrAccess, a)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.file.Access
	s.file.Access = a
	if err := s.persistLocked(); err != nil {
		s.file.Access = prev
		return err
	}
	return nil
}

// SetTrustedRanges validates and persists the trusted CIDR list. Each entry
// must parse as a CIDR; ValidateTrustedRanges is exported so the Settings
// handler can validate before ever touching the store.
func (s *Store) SetTrustedRanges(ranges []string) error {
	clean, err := ValidateTrustedRanges(ranges)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.file.TrustedRanges
	s.file.TrustedRanges = clean
	if err := s.persistLocked(); err != nil {
		s.file.TrustedRanges = prev
		return err
	}
	return nil
}

// ValidateTrustedRanges trims, caps, and parses each entry as a CIDR. Bare IPs
// are accepted and widened to a /32 (or /128) so users can paste a single
// address without knowing CIDR notation.
func ValidateTrustedRanges(ranges []string) ([]string, error) {
	if len(ranges) > MaxTrustedRanges {
		return nil, fmt.Errorf("%w: at most %d ranges", ErrTrustedRange, MaxTrustedRanges)
	}
	out := make([]string, 0, len(ranges))
	for _, r := range ranges {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if !strings.Contains(r, "/") {
			ip := net.ParseIP(r)
			if ip == nil {
				return nil, fmt.Errorf("%w: %q is not an IP or CIDR", ErrTrustedRange, r)
			}
			if ip.To4() != nil {
				r += "/32"
			} else {
				r += "/128"
			}
		}
		if _, _, err := net.ParseCIDR(r); err != nil {
			return nil, fmt.Errorf("%w: %q: %v", ErrTrustedRange, r, err)
		}
		out = append(out, r)
	}
	return out, nil
}

func cloneFile(f File) File {
	out := f
	out.TrustedRanges = append([]string{}, f.TrustedRanges...)
	return out
}

func (s *Store) persistLocked() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	// Re-assert 0700 on a directory we just created or already owned.
	if err := os.Chmod(dir, dirMode); err != nil {
		// Non-fatal if we cannot chmod an existing dir we don't own.
		_ = err
	}
	data, err := json.MarshalIndent(s.file, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return AtomicWrite(s.path, data)
}

// AtomicWrite writes data to path via a same-directory temp file, fsync, rename.
func AtomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	tmp := filepath.Join(dir, filepath.Base(path)+".tmp")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, fileMode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}
