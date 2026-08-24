package projects

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAddListFindRemove(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(filepath.Join(dir, "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	if list, err := s.List(); err != nil || len(list) != 0 {
		t.Fatalf("fresh store should be empty: %+v %v", list, err)
	}

	p, err := s.Add(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Path != dir || p.Name != filepath.Base(dir) || p.ID != ID(dir) {
		t.Fatalf("unexpected project: %+v", p)
	}

	// Re-adding the same path is idempotent, not an error.
	if p2, err := s.Add(dir); err != nil || p2 != p {
		t.Fatalf("re-add: %+v %v", p2, err)
	}

	list, err := s.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("got %+v %v", list, err)
	}

	found, err := s.Find(p.ID)
	if err != nil || found != p {
		t.Fatalf("find: %+v %v", found, err)
	}

	if err := s.Remove(p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Find(p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := s.Remove(p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound on second remove, got %v", err)
	}
}

func TestAddRejectsBadPaths(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(filepath.Join(dir, "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	plainFile := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(plainFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"relative/path",
		filepath.Join(dir, "does-not-exist"),
		plainFile,
	} {
		if _, err := s.Add(path); err == nil {
			t.Errorf("%q: expected an error", path)
		}
	}
}

func TestListPersistsAcrossLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")
	s1, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s1.Add(dir); err != nil {
		t.Fatal(err)
	}

	s2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	list, err := s2.List()
	if err != nil || len(list) != 1 || list[0].Path != dir {
		t.Fatalf("got %+v %v", list, err)
	}
}

func TestIDIsStableAndFixedLength(t *testing.T) {
	a := ID("/Users/x/foo")
	b := ID("/Users/x/foo")
	c := ID("/Users/x/bar")
	if a != b {
		t.Fatal("ID must be deterministic")
	}
	if a == c {
		t.Fatal("different paths should not collide in this test")
	}
	if len(a) != idLen {
		t.Fatalf("ID length = %d, want %d", len(a), idLen)
	}
}
