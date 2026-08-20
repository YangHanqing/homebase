package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yanghanqing/homebase/internal/config"
)

// testStore returns a fresh config.Store backed by a temp file.
func testStore(t *testing.T) *config.Store {
	t.Helper()
	s, err := config.Load(t.TempDir() + "/config.json")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// do sends one request through h and returns the recorded response.
func do(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return doFrom(t, h, method, path, "192.0.2.1:1234", body)
}

// doFrom is do with an explicit RemoteAddr (settings/devices are loopback-gated).
func doFrom(t *testing.T, h http.Handler, method, path, remote string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.RemoteAddr = remote
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// decode JSON-decodes a recorded response body into T.
func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}
