package api

import (
	"encoding/json"
	"net/http"
	"time"
)

// HealthHandler serves GET /api/health. It is the one route that is never
// gated on a paired device, because `homebase start`/`status` probe it before
// any browser exists — so it must not report anything an unpaired caller
// should not learn. The bind address and process start time are echoed to
// loopback peers only: that is the probe's actual caller, while an
// off-machine prober has no business discovering which other addresses this
// listener answers on, or when it came up.
func HealthHandler(listen string, started time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{"ok": true}
		if loopbackPeer(r) {
			body["listen"] = listen
			if !started.IsZero() {
				body["started"] = started.UTC().Format(time.RFC3339)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}
}
