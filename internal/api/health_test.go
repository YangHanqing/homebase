package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// health must stay usable by `homebase start`/`status`, which probe it over
// loopback before any device is paired — and must not hand the bind address to
// anyone else, since it is the one route that is never gated on pairing.
func TestHealth(t *testing.T) {
	for _, tc := range []struct {
		name       string
		remoteAddr string
		wantListen string
	}{
		{"loopback peer sees the bind address", "127.0.0.1:54321", "127.0.0.1:7681"},
		{"off-machine peer does not", "192.0.2.1:54321", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewMuxHealth("127.0.0.1:7681")
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
			req.RemoteAddr = tc.remoteAddr
			h.ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatalf("status %d", rec.Code)
			}
			var body struct {
				OK     bool   `json:"ok"`
				Listen string `json:"listen"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if !body.OK || body.Listen != tc.wantListen {
				t.Fatalf("got %+v, want listen %q", body, tc.wantListen)
			}
		})
	}
}

func TestPlaceholderIndex(t *testing.T) {
	h := NewMuxHealth("127.0.0.1:7681")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if !contains(rec.Body.String(), "Homebase") {
		t.Fatalf("missing placeholder: %s", rec.Body.String())
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && search(s, sub)))
}

func search(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
