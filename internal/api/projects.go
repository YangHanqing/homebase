package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/yanghanqing/homebase/internal/ident"
	"github.com/yanghanqing/homebase/internal/projects"
	"github.com/yanghanqing/homebase/internal/tmux"
)

// handleProjects lists or creates tracked projects. A project is just a
// folder Homebase remembers; its tmux session (tmux.ProjectSession) is
// created lazily on first attach, not here.
func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := s.Projects.List()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"projects": list})
	case http.MethodPost:
		var body struct {
			Path string `json:"path"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		p, err := s.Projects.Add(body.Path)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, p)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProject removes one tracked project. Deleting a project is explicit
// user intent to also end its tmux session — unlike closing a window, which
// must never take a session down by accident (AGENT.md hard constraint 3) —
// so this also issues kill-session for it. That is best-effort: the project
// is already untracked by the time it runs, and a session that never
// existed, or fails to die for some other reason, must not turn a completed
// deletion into an error response.
func (s *Server) handleProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := ident.ValidateProjectID(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := s.Projects.Remove(id); err != nil {
			if errors.Is(err, projects.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), controlTimeout)
		defer cancel()
		client := s.tmuxClient()
		client.Session = tmux.ProjectSession(id)
		if err := client.KillSession(ctx); err != nil && s.Log != nil {
			s.Log.Info("kill project session failed", "project", id, "err", err)
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
