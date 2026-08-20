package devices

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "devices.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return s, path
}

func TestMintRedeemRoundTrip(t *testing.T) {
	s, _ := newStore(t)
	now := time.Now()
	token, expires, err := s.Mint(now)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("empty token")
	}
	if got := expires.Sub(now); got != TokenTTL {
		t.Fatalf("ttl = %v, want %v", got, TokenTTL)
	}

	secret, dev, err := s.Redeem(token, "Firefox", now)
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" || dev.ID == "" || dev.Name != "Firefox" {
		t.Fatalf("bad device: %+v", dev)
	}
	if _, ok := s.Verify(secret, now); !ok {
		t.Fatal("session secret should verify")
	}
	if _, ok := s.Verify("wrong", now); ok {
		t.Fatal("random secret must not verify")
	}
}

// The plaintext token and session secret must never touch the disk: devices.json
// is not supposed to be a bearer credential.
func TestOnlyHashesArePersisted(t *testing.T) {
	s, path := newStore(t)
	now := time.Now()
	token, _, err := s.Mint(now)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), token) {
		t.Fatal("pending token stored in plaintext")
	}
	secret, _, err := s.Redeem(token, "b", now)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("session secret stored in plaintext")
	}
}

func TestRedeemIsSingleUse(t *testing.T) {
	s, _ := newStore(t)
	now := time.Now()
	token, _, err := s.Mint(now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Redeem(token, "a", now); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Redeem(token, "b", now); err != ErrBadToken {
		t.Fatalf("replay err = %v, want ErrBadToken", err)
	}
}

// Two browsers racing the same link must not both end up enrolled.
func TestConcurrentRedeemElectsOneWinner(t *testing.T) {
	s, _ := newStore(t)
	now := time.Now()
	token, _, err := s.Mint(now)
	if err != nil {
		t.Fatal(err)
	}

	const n = 8
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		winners int
	)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if _, _, err := s.Redeem(token, "racer", now); err == nil {
				mu.Lock()
				winners++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if winners != 1 {
		t.Fatalf("winners = %d, want 1", winners)
	}
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("devices = %d, want 1", len(list))
	}
}

func TestExpiredTokenRejectedAndPruned(t *testing.T) {
	s, _ := newStore(t)
	old := time.Now().Add(-2 * TokenTTL)
	token, _, err := s.Mint(old)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Redeem(token, "a", time.Now()); err != ErrBadToken {
		t.Fatalf("err = %v, want ErrBadToken", err)
	}
	// A later mint prunes the dead entry rather than letting it accumulate.
	if _, _, err := s.Mint(time.Now()); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	pending := len(s.f.Pending)
	s.mu.Unlock()
	if pending != 1 {
		t.Fatalf("pending = %d, want 1 (expired one pruned)", pending)
	}
}

func TestRevoke(t *testing.T) {
	s, _ := newStore(t)
	now := time.Now()
	token, _, _ := s.Mint(now)
	secret, dev, err := s.Redeem(token, "a", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(dev.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Verify(secret, now); ok {
		t.Fatal("revoked device still verifies")
	}
	if err := s.Revoke("nope"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// The `pair` CLI and the server are separate processes sharing this file, so a
// token minted elsewhere must be visible without a restart.
func TestSecondProcessSeesMintedToken(t *testing.T) {
	server, path := newStore(t)
	cli, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	token, _, err := cli.Mint(now)
	if err != nil {
		t.Fatal(err)
	}
	secret, _, err := server.Redeem(token, "browser", now)
	if err != nil {
		t.Fatalf("server could not redeem a CLI-minted token: %v", err)
	}
	if _, ok := server.Verify(secret, now); !ok {
		t.Fatal("verify failed")
	}
	// And a revocation from the CLI side reaches the server on cache miss.
	if err := cli.RevokeAll(); err != nil {
		t.Fatal(err)
	}
	if _, ok := server.Verify(secret, now); ok {
		t.Fatal("revocation did not propagate")
	}
}

func TestCorruptFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("corrupt file should not silently revoke every device")
	}
}

func TestMissingFileIsEmpty(t *testing.T) {
	s, path := newStore(t)
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("got %d devices", len(list))
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("List should not create the file")
	}
}

func TestPersistedShape(t *testing.T) {
	s, path := newStore(t)
	now := time.Now()
	token, _, _ := s.Mint(now)
	if _, _, err := s.Redeem(token, "a", now); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Version int               `json:"version"`
		Pending []json.RawMessage `json:"pending"`
		Devices []json.RawMessage `json:"devices"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if f.Version != SchemaVersion || len(f.Pending) != 0 || len(f.Devices) != 1 {
		t.Fatalf("unexpected file: %s", raw)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", st.Mode().Perm())
	}
}

func TestDeviceNameSanitized(t *testing.T) {
	if got := deviceName("ok\x00\x1bname"); got != "okname" {
		t.Fatalf("got %q", got)
	}
	if got := deviceName(""); got != "unnamed device" {
		t.Fatalf("got %q", got)
	}
	if got := deviceName(strings.Repeat("x", 200)); len(got) != 64 {
		t.Fatalf("len = %d", len(got))
	}
}
