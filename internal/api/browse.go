package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// dirEntry is one subdirectory offered by the project-folder picker. Only
// directories are listed — this is not a general file browser, it exists
// solely to let the browser choose a project's path without ever supplying
// one directly (the same reason /api/windows takes a "dir" mode rather than
// a path: reaching this page already implies a local shell, but a plain text
// field inviting an arbitrary path is still a worse interface than a list of
// what is actually there).
type dirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// handleBrowse lists the subdirectories of one folder, defaulting to $HOME
// so the picker never opens on "/" — many of the folders under root need
// permissions Homebase does not have, and none of them are a sane place to
// start choosing a project.
func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "cannot resolve home directory")
			return
		}
		path = home
	}
	if !filepath.IsAbs(path) {
		writeError(w, http.StatusBadRequest, "path must be absolute")
		return
	}
	clean := filepath.Clean(path)
	info, err := os.Stat(clean)
	if err != nil || !info.IsDir() {
		writeError(w, http.StatusBadRequest, "not a directory")
		return
	}
	items, err := os.ReadDir(clean)
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read directory")
		return
	}
	entries := make([]dirEntry, 0, len(items))
	for _, item := range items {
		// A dangling symlink or a permission-denied entry fails Type(); skip
		// it rather than erroring the whole listing over one bad child.
		if item.Type()&os.ModeSymlink != 0 {
			if st, err := os.Stat(filepath.Join(clean, item.Name())); err != nil || !st.IsDir() {
				continue
			}
		} else if !item.IsDir() {
			continue
		}
		entries = append(entries, dirEntry{Name: item.Name(), Path: filepath.Join(clean, item.Name())})
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	parent := filepath.Dir(clean)
	if parent == clean {
		parent = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":    clean,
		"parent":  parent,
		"entries": entries,
	})
}
