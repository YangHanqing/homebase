package api

import (
	"errors"
	"net/http"

	"github.com/yanghanqing/homebase/internal/ident"
	"github.com/yanghanqing/homebase/internal/projects"
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

// handleProject removes one tracked project. It never touches tmux — see
// internal/projects's package doc.
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
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
