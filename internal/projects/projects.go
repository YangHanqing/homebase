// Package projects persists the folders shown in the sidebar as projects.
//
// tmux still owns all session/window state — this file only remembers which
// folders the user asked Homebase to track, so a project with no open
// windows is not forgotten. A project's tmux session (tmux.ProjectSession)
// is independent of this list: closing a project's last window lets tmux
// tear the session down as usual, and the next window simply recreates it.
// Removing a project from this list does not touch tmux either way. See
// AGENT.md "Projects".
package projects

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/yanghanqing/homebase/internal/config"
)

// SchemaVersion is this file's schema, independent of config.json's.
const SchemaVersion = 1

// idLen matches internal/ident.ValidateProjectID.
const idLen = 12

// ErrNotFound is returned by Remove and Find for an id not currently tracked.
var ErrNotFound = errors.New("project not found")

type file struct {
	Version int      `json:"version"`
	Paths   []string `json:"paths"`
}

// Project is a read-only, derived view: only Path is persisted. ID and Name
// are computed from it on every read, so the store stays a single ordered
// list of paths with nothing to keep in sync.
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// Store is the process-wide projects.json mutex + path.
type Store struct {
	mu   sync.Mutex
	path string
}

// DefaultPath is projects.json beside config.json.
func DefaultPath() string {
	return filepath.Join(config.DefaultDir(), "projects.json")
}

// Load opens path. A missing file is not an error: List returns empty until
// the first project is added, which is what creates the file.
func Load(path string) (*Store, error) {
	s := &Store{path: path}
	if _, err := s.readLocked(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) readLocked() (file, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return file{Version: SchemaVersion}, nil
		}
		return file{}, err
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return file{}, fmt.Errorf("projects file is corrupt: %w", err)
	}
	if f.Version == 0 {
		f.Version = SchemaVersion
	}
	return f, nil
}

func (s *Store) writeLocked(f file) error {
	f.Version = SchemaVersion
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return config.AtomicWrite(s.path, data)
}

// List returns tracked projects in the order they were added.
func (s *Store) List() ([]Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.readLocked()
	if err != nil {
		return nil, err
	}
	out := make([]Project, 0, len(f.Paths))
	for _, p := range f.Paths {
		out = append(out, fromPath(p))
	}
	return out, nil
}

// Find returns the tracked project with the given id.
func (s *Store) Find(id string) (Project, error) {
	list, err := s.List()
	if err != nil {
		return Project{}, err
	}
	for _, p := range list {
		if p.ID == id {
			return p, nil
		}
	}
	return Project{}, ErrNotFound
}

// Add validates path (absolute, existing directory) and appends it.
// Re-adding the same path returns the existing entry rather than erroring —
// re-submitting the same folder from the directory picker is a normal
// double-click, not a mistake worth surfacing.
func (s *Store) Add(path string) (Project, error) {
	clean, err := ValidatePath(path)
	if err != nil {
		return Project{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.readLocked()
	if err != nil {
		return Project{}, err
	}
	for _, p := range f.Paths {
		if p == clean {
			return fromPath(p), nil
		}
	}
	f.Paths = append(f.Paths, clean)
	if err := s.writeLocked(f); err != nil {
		return Project{}, err
	}
	return fromPath(clean), nil
}

// Remove drops a project by id. It does not touch tmux — see the package
// doc — so a session that is still running under that path is simply no
// longer listed, exactly like any other untracked "homebase-*" session.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.readLocked()
	if err != nil {
		return err
	}
	idx := -1
	for i, p := range f.Paths {
		if ID(p) == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrNotFound
	}
	f.Paths = append(f.Paths[:idx], f.Paths[idx+1:]...)
	return s.writeLocked(f)
}

// ValidatePath checks a candidate project folder: must be an absolute,
// existing directory. The browser never gets to pick an arbitrary command
// argument this way — same rationale as the new-window "dir" mode in
// internal/tmux — but a project's whole point is to name a real path, so
// here that path is the validated, first-class input rather than a mode.
func ValidatePath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be absolute")
	}
	clean := filepath.Clean(path)
	info, err := os.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("path does not exist")
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory")
	}
	return clean, nil
}

// ID derives a stable, tmux-safe identifier from a path: idLen hex
// characters of its SHA-256. Short enough to read in a session name, long
// enough that a collision between two folders on the same machine is not
// worth guarding against.
func ID(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])[:idLen]
}

func fromPath(path string) Project {
	return Project{ID: ID(path), Name: filepath.Base(path), Path: path}
}
