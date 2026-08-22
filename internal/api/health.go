package api

import (
	"encoding/json"
	"net/http"
)

// HealthHandler serves GET /api/health. It is the one route that is never
// gated on a paired device, because `homebase start`/`status` probe it before
// any browser exists — so it must not report anything an unpaired caller
// should not learn. The bind address is echoed to loopback peers only: that
// is the probe's actual caller, while an off-machine prober has no business
// discovering which other addresses this listener answers on.
func HealthHandler(listen string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{"ok": true}
		if loopbackPeer(r) {
			body["listen"] = listen
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}
}
