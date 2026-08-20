package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStaticAssets(t *testing.T) {
	h := NewMuxHealth("127.0.0.1:7681")
	for _, path := range []string{"/", "/settings.html", "/css/app.css", "/css/settings.css", "/js/app.js", "/js/session.js", "/js/backoff.js", "/js/terminal.js", "/js/i18n.js", "/js/settings.js", "/vendor/xterm/xterm.js", "/vendor/xterm/xterm.css"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != 200 {
			t.Errorf("%s -> %d", path, rec.Code)
		}
	}
}
