package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestMissingFileWritesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "homebase", "config.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("file mode %o", st.Mode().Perm())
	}
	dst, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if dst.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %o", dst.Mode().Perm())
	}
	f := s.Snapshot()
	if f.Version != SchemaVersion || f.ListenPort != DefaultPort || f.Access != AccessPrivate {
		t.Fatalf("unexpected default: %+v", f)
	}
	if len(f.TrustedRanges) != 1 || f.TrustedRanges[0] != "100.64.0.0/10" {
		t.Fatalf("unexpected default trusted_ranges: %+v", f.TrustedRanges)
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.file.ListenAddr = "100.1.2.3"
	s.file.ListenPort = 9000
	if err := s.persistLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()

	s2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	f := s2.Snapshot()
	if f.ListenAddr != "100.1.2.3" || f.ListenPort != 9000 {
		t.Fatalf("got %+v", f)
	}
}

func TestCorruptFileError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected corrupt error")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "{not json" {
		t.Fatal("corrupt file must not be overwritten")
	}
}

func TestUnsupportedVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"version":99}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected unsupported version")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != `{"version":99}` {
		t.Fatal("must not overwrite higher version")
	}
}

func TestMissingVersionTreatedAsCurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"listen_port":1990}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Version != SchemaVersion {
		t.Fatalf("got version %d", s.Snapshot().Version)
	}
}

func TestAtomicWriteLeavesNoTmp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.Name() != "config.json" {
			t.Fatalf("tmp leftover: %s", e.Name())
		}
	}
}

// Two Homebase processes write these files — the server and the `homebase
// pair` CLI — and they share no lock. Concurrent writers may lose an update,
// but must never leave a torn document behind: a corrupt devices.json is a
// startup error, which would revoke every paired device at once.
func TestAtomicWriteConcurrentWritersNeverTear(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "devices.json")

	docs := make([][]byte, 8)
	for i := range docs {
		// Different lengths, so a torn write shows up as a short or spliced file.
		docs[i] = []byte(`{"version":1,"who":"` + strings.Repeat(string(rune('a'+i)), 200*(i+1)) + `"}`)
	}

	var wg sync.WaitGroup
	for round := 0; round < 20; round++ {
		for i := range docs {
			wg.Add(1)
			go func(data []byte) {
				defer wg.Done()
				if err := AtomicWrite(path, data); err != nil {
					t.Errorf("AtomicWrite: %v", err)
				}
			}(docs[i])
		}
	}
	wg.Wait()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range docs {
		if bytes.Equal(got, want) {
			return
		}
	}
	t.Fatalf("file is not any writer's complete document: %q", got)
}

// An old v1/v2/v3 file must still open: unknown fields (windows, tls, hosts)
// are simply dropped, and legacy allow_public_bind still maps to a tier.
func TestOldFileLoadsWithDroppedFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	old := `{"version":1,"listen_port":7681,"windows":[{"id":"a"}],"tls":{"cert_path":"x"},"hosts":[{"id":"h1"}]}`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("old file should load, got %v", err)
	}
	if f := s.Snapshot(); f.Version != SchemaVersion {
		t.Fatalf("want version migrated to %d, got %d", SchemaVersion, f.Version)
	}
}

func TestValidateTrustedRanges(t *testing.T) {
	got, err := ValidateTrustedRanges([]string{" 192.168.1.0/24 ", "100.64.0.0/10", "10.0.0.5"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"192.168.1.0/24", "100.64.0.0/10", "10.0.0.5/32"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}

	if _, err := ValidateTrustedRanges([]string{"not-a-range"}); err == nil {
		t.Fatal("expected error for invalid range")
	}

	for _, pub := range []string{"0.0.0.0/0", "8.8.8.8", "1.1.1.1/32", "10.0.0.0/7", "203.0.113.0/24", "2001:db8::/32", "::1"} {
		if _, err := ValidateTrustedRanges([]string{pub}); err == nil {
			t.Errorf("expected public/IPv6 range %q to be rejected", pub)
		}
	}

	// An IPv6 range is rejected for being IPv6, not for being public — the
	// message has to say so or the user retries with a "more private" prefix.
	for _, v6 := range []string{"2001:db8::/32", "fd00::/8", "::1"} {
		_, err := ValidateTrustedRanges([]string{v6})
		if err == nil {
			t.Errorf("expected %q to be rejected", v6)
			continue
		}
		if !strings.Contains(err.Error(), "IPv6") {
			t.Errorf("%q: message should name IPv6, got %v", v6, err)
		}
	}

	tooMany := make([]string, MaxTrustedRanges+1)
	for i := range tooMany {
		tooMany[i] = "10.0.0.0/8"
	}
	if _, err := ValidateTrustedRanges(tooMany); err == nil {
		t.Fatal("expected error for too many ranges")
	}
}

func TestLoadMigratesLocalAccessToPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"version":4,"access":"local","listen_port":1990}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Snapshot().Access; got != AccessPrivate {
		t.Fatalf("access=%q, want private", got)
	}
}

func TestSetAccessRejectsLocal(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAccess(AccessLocal); err == nil {
		t.Fatal("expected local to be rejected on write")
	}
}

func TestSetTrustedRangesRejectsInvalidWithoutPersisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	before := s.Snapshot().TrustedRanges
	if err := s.SetTrustedRanges([]string{"garbage"}); err == nil {
		t.Fatal("expected error")
	}
	after := s.Snapshot().TrustedRanges
	if len(before) != len(after) || before[0] != after[0] {
		t.Fatalf("trusted ranges changed on failed validation: %v -> %v", before, after)
	}

	if err := s.SetTrustedRanges([]string{"10.0.0.0/8"}); err != nil {
		t.Fatal(err)
	}
	got := s.Snapshot().TrustedRanges
	if len(got) != 1 || got[0] != "10.0.0.0/8" {
		t.Fatalf("got %v", got)
	}

	var decoded File
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.TrustedRanges) != 1 || decoded.TrustedRanges[0] != "10.0.0.0/8" {
		t.Fatalf("on-disk trusted_ranges = %v", decoded.TrustedRanges)
	}
}
