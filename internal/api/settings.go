package api

import (
	"net"
	"net/http"

	"github.com/yanghanqing/homebase/internal/config"
)

// requireLoopbackPeer gates the settings endpoints to the machine's own
// browser, regardless of which access tier is currently running. This is
// deliberately independent of auth.RequireLoopbackHost (which pins the Host
// header for an unauthenticated loopback listener): here we check the actual
// TCP peer address, so that even on the private/lan tier — where the main UI
// is reachable from elsewhere and gated by a paired-device cookie instead —
// changing access/trusted-ranges still requires being on the machine itself.
func requireLoopbackPeer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackPeer(r) {
			writeError(w, http.StatusForbidden, "settings can only be changed from this machine (open it in a browser running on the Homebase host itself)")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loopbackPeer reports whether the TCP client is on this machine. It reads the
// peer address, never a header: X-Forwarded-For and friends are attacker-set.
func loopbackPeer(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type settingsBody struct {
	Access        config.Access `json:"access"`
	TrustedRanges []string      `json:"trusted_ranges"`
}

func settingsResponse(f config.File) settingsBody {
	return settingsBody{Access: f.Access, TrustedRanges: f.TrustedRanges}
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	requireLoopbackPeer(http.HandlerFunc(s.handleSettingsInner)).ServeHTTP(w, r)
}

func (s *Server) handleSettingsInner(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		resp := settingsResponse(s.Store.Snapshot())
		writeJSON(w, http.StatusOK, map[string]any{
			"access":         resp.Access,
			"trusted_ranges": resp.TrustedRanges,
			"version":        s.Version,
		})
	case http.MethodPut:
		var body settingsBody
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if body.Access != "" {
			if !config.ValidAccess(body.Access) {
				writeError(w, http.StatusBadRequest, "unknown access tier")
				return
			}
			if err := s.Store.SetAccess(body.Access); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		if body.TrustedRanges != nil {
			if err := s.Store.SetTrustedRanges(body.TrustedRanges); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		resp := settingsResponse(s.Store.Snapshot())
		restarting := s.Restart != nil
		writeJSON(w, http.StatusOK, map[string]any{
			"access":           resp.Access,
			"trusted_ranges":   resp.TrustedRanges,
			"restart_required": !restarting,
			"restarting":       restarting,
		})
		if s.Restart != nil {
			s.Restart()
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
