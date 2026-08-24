package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/yanghanqing/homebase/internal/ident"
	"github.com/yanghanqing/homebase/internal/session"
	"github.com/yanghanqing/homebase/internal/tmux"
)

// controlTimeout bounds one short-lived tmux command.
const controlTimeout = 15 * time.Second

// errUnknownProject covers both a malformed id and one this server does not
// currently track. Either way the answer is the same 404 — the format
// details are not the browser's business.
var errUnknownProject = errors.New("unknown project")

// tmuxClient builds the control-channel client, honoring the test hook.
func (s *Server) tmuxClient() tmux.Client {
	if s.NewTmux != nil {
		return s.NewTmux()
	}
	return tmux.NewClient()
}

// tmuxClientForRequest resolves the optional "?project=<id>" query param
// shared by every /api/windows and /api/scroll route. An absent param keeps
// today's behavior exactly: the legacy singleton session, via tmuxClient.
func (s *Server) tmuxClientForRequest(r *http.Request) (tmux.Client, error) {
	id := r.URL.Query().Get("project")
	if id == "" {
		return s.tmuxClient(), nil
	}
	if err := ident.ValidateProjectID(id); err != nil {
		return tmux.Client{}, errUnknownProject
	}
	if s.Projects == nil {
		return tmux.Client{}, errUnknownProject
	}
	if _, err := s.Projects.Find(id); err != nil {
		return tmux.Client{}, errUnknownProject
	}
	client := s.tmuxClient()
	client.Session = tmux.ProjectSession(id)
	return client, nil
}

// projectPathForRequest resolves the optional "?project=<id>" query param to
// that project's path, for NewWindow's fallback directory. Any failure —
// no param, unknown id — yields "", the legacy singleton's "no fallback"
// value; tmuxClientForRequest has already turned a real error into 404 by
// the time this runs, so this lookup only needs to be best-effort.
func (s *Server) projectPathForRequest(r *http.Request) string {
	id := r.URL.Query().Get("project")
	if id == "" || s.Projects == nil {
		return ""
	}
	p, err := s.Projects.Find(id)
	if err != nil {
		return ""
	}
	return p.Path
}

// dialerForProject resolves a "?project=<id>" query param on the WebSocket
// upgrade to a per-project dialer, the same validation tmuxClientForRequest
// applies to the control channel.
func (s *Server) dialerForProject(id string) (session.Dialer, error) {
	if err := ident.ValidateProjectID(id); err != nil {
		return nil, errUnknownProject
	}
	if s.Projects == nil {
		return nil, errUnknownProject
	}
	if _, err := s.Projects.Find(id); err != nil {
		return nil, errUnknownProject
	}
	return session.LocalDialer{Session: tmux.ProjectSession(id)}, nil
}

func (s *Server) handleTmuxWindows(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), controlTimeout)
	defer cancel()
	client, err := s.tmuxClientForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	switch r.Method {
	case http.MethodGet:
		windows, err := client.ListWindows(ctx)
		if err != nil {
			s.writeTmuxError(w, err)
			return
		}
		// Best-effort: a client-count failure should not hide the window list.
		clients, err := client.ClientCount(ctx)
		if err != nil {
			clients = 0
		}
		// "now" is this machine's clock, read at the same moment as the
		// activity timestamps beside it. The browser's clock may be minutes
		// off — a phone that has been asleep, a machine with no NTP — so the
		// frontend must never subtract a window's activity from Date.now().
		// Sending both readings of the *server's* clock makes the age
		// skew-free without inventing a clock-sync protocol, and keeps
		// Window a faithful mirror of what tmux reported.
		writeJSON(w, http.StatusOK, map[string]any{
			"windows": windows,
			"clients": clients,
			"now":     time.Now().Unix(),
		})
	case http.MethodPost:
		var body struct {
			Dir string `json:"dir"`
		}
		if err := decodeJSON(r, &body); err != nil && err != io.EOF {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		index, err := client.NewWindow(ctx, body.Dir, s.projectPathForRequest(r))
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
	client, err := s.tmuxClientForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

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
