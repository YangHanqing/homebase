package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// health must stay usable by `homebase start`/`status`, which probe it over
// loopback before any device is paired — and must not hand the bind address or
// start time to anyone else, since it is the one route that is never gated on
// pairing.
func TestHealth(t *testing.T) {
	started := time.Date(2026, 9, 1, 6, 32, 5, 0, time.UTC)
	for _, tc := range []struct {
		name        string
		remoteAddr  string
		wantListen  string
		wantStarted string
	}{
		{"loopback peer sees the bind address", "127.0.0.1:54321", "127.0.0.1:7681", "2026-09-01T06:32:05Z"},
		{"off-machine peer does not", "192.0.2.1:54321", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewMuxServer(&Server{Listen: "127.0.0.1:7681", Started: started})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
			req.RemoteAddr = tc.remoteAddr
			h.ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatalf("status %d", rec.Code)
			}
			var body struct {
				OK      bool   `json:"ok"`
				Listen  string `json:"listen"`
				Started string `json:"started"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if !body.OK || body.Listen != tc.wantListen || body.Started != tc.wantStarted {
				t.Fatalf("got %+v, want listen %q started %q", body, tc.wantListen, tc.wantStarted)
			}
		})
	}
}

func TestHealthOmitsZeroStarted(t *testing.T) {
	h := NewMuxHealth("127.0.0.1:7681")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["started"]; ok {
		t.Fatalf("zero started must be omitted, got %+v", body)
	}
	if body["listen"] != "127.0.0.1:7681" {
		t.Fatalf("got %+v", body)
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
