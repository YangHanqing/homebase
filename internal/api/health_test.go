package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	h := NewMuxHealth("127.0.0.1:7681")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
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
	if !body.OK || body.Listen != "127.0.0.1:7681" {
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
