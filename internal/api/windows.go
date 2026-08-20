package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/yanghanqing/homebase/internal/ident"
	"github.com/yanghanqing/homebase/internal/tmux"
)

// controlTimeout bounds one short-lived tmux command.
const controlTimeout = 15 * time.Second

// tmuxClient builds the control-channel client, honoring the test hook.
func (s *Server) tmuxClient() tmux.Client {
	if s.NewTmux != nil {
		return s.NewTmux()
	}
	return tmux.NewClient()
}

func (s *Server) handleTmuxWindows(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), controlTimeout)
	defer cancel()
	client := s.tmuxClient()

	switch r.Method {
	case http.MethodGet:
		windows, err := client.ListWindows(ctx)
		if err != nil {
			s.writeTmuxError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"windows": windows})
	case http.MethodPost:
		index, err := client.NewWindow(ctx)
		if err != nil {
			s.writeTmuxError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]int{"index": index})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type windowBody struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

func (s *Server) handleTmuxWindow(w http.ResponseWriter, r *http.Request) {
	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil || ident.ValidateWindowIndex(index) != nil {
		writeError(w, http.StatusBadRequest, "invalid window index")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), controlTimeout)
	defer cancel()
	client := s.tmuxClient()

	switch r.Method {
	case http.MethodPut:
		var body windowBody
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if body.Name != "" {
			if err := ident.ValidateName(body.Name); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if err := client.RenameWindow(ctx, index, body.Name); err != nil {
				s.writeTmuxError(w, err)
				return
			}
		}
		if body.Active {
			if err := client.SelectWindow(ctx, index); err != nil {
				s.writeTmuxError(w, err)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := client.KillWindow(ctx, index); err != nil {
			s.writeTmuxError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// writeTmuxError maps control-channel failures onto the same status codes the
// PTY channel reports, so the UI has one vocabulary.
func (s *Server) writeTmuxError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, tmux.ErrLastWindow):
		writeCodedError(w, http.StatusBadRequest, "last_window", "this is the last window; closing it would end the session")
	case errors.Is(err, tmux.ErrNoTmux):
		writeCodedError(w, http.StatusBadGateway, "enotmux", "tmux is not installed on this machine")
	case errors.Is(err, tmux.ErrNoSession):
		// The session vanished between the list and the action. Not fatal:
		// the PTY reconnect recreates it.
		writeCodedError(w, http.StatusConflict, "no_session", "session is gone, reconnecting")
	default:
		if s.Log != nil {
			s.Log.Info("tmux control failed", "err", err)
		}
		writeCodedError(w, http.StatusBadGateway, "unknown", err.Error())
	}
}

func writeCodedError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": msg, "code": code})
}
