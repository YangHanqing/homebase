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
	MaxTrustedRanges = 5
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
	// AccessLocal is accepted only when loading an old file, and is rewritten
	// to AccessPrivate. It is not a valid write value.
	AccessLocal Access = "local"
	// AccessPrivate binds 127.0.0.1 plus the first local address that falls
	// inside TrustedRanges (Tailscale/WireGuard/etc). Falls back to loopback
	// if no such address is found.
	AccessPrivate Access = "private"
	// AccessLAN binds 127.0.0.1 plus every non-public IPv4 on this machine.
	// Public and unspecified addresses are never bound.
	AccessLAN Access = "lan"
)

// ValidAccess reports whether a is a tier that may be written.
func ValidAccess(a Access) bool {
	switch a {
	case AccessPrivate, AccessLAN:
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
// A fresh install binds loopback plus the trusted range (Tailscale by default).
func DefaultFile() File {
	return File{
		Version:       SchemaVersion,
		Access:        AccessPrivate,
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
	if f.Access == AccessLocal {
		f.Access = AccessPrivate
	}
	if !ValidAccess(f.Access) {
		return nil, fmt.Errorf("%w: %q (want private or lan)", ErrAccess, f.Access)
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

// SetListenPort persists the listen port (used when start walks up from a
// busy 1990).
func (s *Store) SetListenPort(port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid listen port %d", port)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.file.ListenPort
	s.file.ListenPort = port
	if err := s.persistLocked(); err != nil {
		s.file.ListenPort = prev
		return err
	}
	return nil
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
		_, n, err := net.ParseCIDR(r)
		if err != nil {
			return nil, fmt.Errorf("%w: %q: %v", ErrTrustedRange, r, err)
		}
		// Say why, rather than letting an IPv6 entry fall through to "not a
		// private network" — the range may well be private; Homebase simply
		// does not bind IPv6.
		if n.IP.To4() == nil {
			return nil, fmt.Errorf("%w: %q is IPv6, and Homebase binds IPv4 only", ErrTrustedRange, r)
		}
		if !CIDRIsNonPublic(n) {
			return nil, fmt.Errorf("%w: %q is not a private network", ErrTrustedRange, r)
		}
		out = append(out, r)
	}
	return out, nil
}

// NonPublicIPv4CIDRs is every IPv4 space Homebase will bind. Public unicast
// and all IPv6 are refused — this build is plain HTTP and is not a public
// service.
var NonPublicIPv4CIDRs = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
}

var nonPublicIPv4Nets = mustParseCIDRs(NonPublicIPv4CIDRs)

func mustParseCIDRs(cidrs []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, s := range cidrs {
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			panic("config: bad NonPublicIPv4CIDR " + s)
		}
		out = append(out, n)
	}
	return out
}

// IPIsNonPublic reports whether ip is loopback, RFC1918, CGNAT (100.64/10),
// or IPv4 link-local. IPv6 is never non-public for our purposes.
func IPIsNonPublic(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	for _, n := range nonPublicIPv4Nets {
		if n.Contains(v4) {
			return true
		}
	}
	return false
}

// CIDRIsNonPublic reports whether n is an IPv4 network wholly inside one
// entry of NonPublicIPv4CIDRs. A wider net that spills into public space
// (10.0.0.0/7, 0.0.0.0/0) is rejected.
func CIDRIsNonPublic(n *net.IPNet) bool {
	if n == nil {
		return false
	}
	v4 := n.IP.To4()
	if v4 == nil {
		return false
	}
	ones, bits := n.Mask.Size()
	if bits != 32 {
		return false
	}
	for _, outer := range nonPublicIPv4Nets {
		oOnes, _ := outer.Mask.Size()
		if ones < oOnes {
			continue
		}
		if outer.Contains(v4) {
			return true
		}
	}
	return false
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
//
// The temp file gets a unique name, which matters because two Homebase
// processes write these files: the server and the `homebase pair` CLI. A
// fixed "<path>.tmp" let one process reopen the other's half-written scratch
// file with O_TRUNC and then rename the result into place — and a devices.json
// mangled that way is a startup error, which revokes every paired device at
// once. Distinct names make the worst case a lost update instead: whichever
// rename lands second wins whole, so the file is always some writer's complete
// document.
func AtomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
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
	// fsync the directory so the rename itself survives a crash. Without it
	// the data is durable but the name may still point at the old inode.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
