package api

import (
	"encoding/json"
	"net/http"
)

// HealthHandler serves GET /api/health.
func HealthHandler(listen string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"listen": listen,
		})
	}
}
