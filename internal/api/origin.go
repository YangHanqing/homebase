package api

import (
	"net/http"
	"net/url"
	"strings"
)

func mutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// originOK implements the CSRF belt-and-suspenders check:
// if Origin is present, its host must match the request host.
// Missing Origin (curl) is allowed.
func originOK(r *http.Request) bool {
	raw := r.Header.Get("Origin")
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

func requireOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mutating(r.Method) && !originOK(r) {
			http.Error(w, `{"error":"origin mismatch"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
