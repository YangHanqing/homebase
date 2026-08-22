package api

import (
	"context"
	"io"
	"net/http"
)

// handleScroll moves the tmux view through its own scrollback.
//
// The browser cannot do this by itself: xterm is attached to a full-screen
// tmux client, so it sits on the alternate screen with no scrollback of its
// own. A swipe on the phone therefore has to become a tmux copy-mode scroll,
// which is what this route is for. Positive lines go back into history,
// negative come forward, 0 leaves copy mode.
func (s *Server) handleScroll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Lines int `json:"lines"`
	}
	if err := decodeJSON(r, &body); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), controlTimeout)
	defer cancel()
	inMode, err := s.tmuxClient().Scroll(ctx, body.Lines)
	if err != nil {
		s.writeTmuxError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"in_mode": inMode})
}
