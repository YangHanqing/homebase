package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yanghanqing/homebase/internal/devices"
)

func newGate(t *testing.T) (*Gate, *devices.Store) {
	t.Helper()
	st, err := devices.Open(filepath.Join(t.TempDir(), "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	return &Gate{Devices: st}, st
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestUnpairedIsRejected(t *testing.T) {
	g, _ := newGate(t)
	h := g.Wrap(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no cookie: %d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "not-a-real-secret"})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("forged cookie: %d", rec.Code)
	}
}

func TestPairThenAccess(t *testing.T) {
	g, st := newGate(t)
	h := g.Wrap(okHandler())

	token, _, err := st.Mint(time.Now())
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, PairPath+"?t="+token, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("redeem: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "location.replace") {
		t.Fatalf("pair success must replace history, got %q", rec.Body.String())
	}

	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName {
			session = c
		}
	}
	if session == nil || session.Value == "" {
		t.Fatal("no session cookie set")
	}
	// Secure is deliberately false: this build never serves TLS.
	// Lax, not Strict: Strict withholds the cookie on a cross-app open.
	if !session.HttpOnly || session.Secure || session.SameSite != http.SameSiteLaxMode {
		t.Fatalf("weak cookie flags: %+v", session)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(session)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("paired request: %d", rec.Code)
	}
}

func TestTokenIsSingleUse(t *testing.T) {
	g, st := newGate(t)
	h := g.Wrap(okHandler())
	token, _, err := st.Mint(time.Now())
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, PairPath+"?t="+token, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("first redeem: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, PairPath+"?t="+token, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("replay should fail, got %d", rec.Code)
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	g, st := newGate(t)
	h := g.Wrap(okHandler())
	token, _, err := st.Mint(time.Now().Add(-devices.TokenTTL - time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, PairPath+"?t="+token, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired token: %d", rec.Code)
	}
}

func TestRevokedDeviceLosesAccess(t *testing.T) {
	g, st := newGate(t)
	h := g.Wrap(okHandler())
	token, _, err := st.Mint(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	secret, dev, err := st.Redeem(token, "test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: secret})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("before revoke: %d", rec.Code)
	}

	if err := st.Revoke(dev.ID); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: secret})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("after revoke: %d", rec.Code)
	}

	// A new pairing URL on the same browser must mint a fresh cookie that works.
	token2, _, err := st.Mint(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	pairReq := httptest.NewRequest(http.MethodGet, PairPath+"?t="+token2, nil)
	pairReq.AddCookie(&http.Cookie{Name: CookieName, Value: secret})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, pairReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("re-pair after revoke: %d", rec.Code)
	}
	var fresh *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName {
			fresh = c
		}
	}
	if fresh == nil || fresh.Value == "" || fresh.Value == secret {
		t.Fatal("re-pair must set a new session cookie")
	}
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(fresh)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("new cookie after re-pair: %d", rec.Code)
	}
}

// API and WebSocket paths must not get an HTML page; the frontend parses these.
func TestDenyContentType(t *testing.T) {
	g, _ := newGate(t)
	h := g.Wrap(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/hosts", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("api deny content-type = %q", ct)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("page deny content-type = %q", ct)
	}
}

// DNS rebinding: a page on evil.com resolves its own hostname to 127.0.0.1 and
// the browser sends Host: evil.com. Without pinning, the unauthenticated
// loopback tier would hand that page a shell.
func TestRequireLoopbackHost(t *testing.T) {
	h := RequireLoopbackHost("7681", okHandler())

	for _, host := range []string{"127.0.0.1:7681", "localhost:7681", "LocalHost:7681", "[::1]:7681"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("host %q rejected: %d", host, rec.Code)
		}
	}
	for _, host := range []string{"evil.com", "evil.com:7681", "127.0.0.1:9999", "attacker.local:7681"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("host %q allowed: %d", host, rec.Code)
		}
	}
}

// Cookies minted under SameSite=Strict must be rewritten to Lax without
// forcing the operator to re-pair; incoming Cookie headers do not carry flags.
func TestAuthenticatedRequestRefreshesCookie(t *testing.T) {
	g, st := newGate(t)
	h := g.Wrap(okHandler())
	token, _, err := st.Mint(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	secret, _, err := st.Redeem(token, "test", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/windows", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: secret})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated: %d", rec.Code)
	}
	var refreshed *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName {
			refreshed = c
		}
	}
	if refreshed == nil || refreshed.Value != secret {
		t.Fatal("authenticated request must rewrite the session cookie")
	}
	if refreshed.SameSite != http.SameSiteLaxMode || refreshed.Secure || !refreshed.HttpOnly {
		t.Fatalf("refreshed cookie flags: %+v", refreshed)
	}
}

func TestHealthSkipsPairing(t *testing.T) {
	g, _ := newGate(t)
	h := g.Wrap(okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("health without cookie: %d", rec.Code)
	}
}

func TestLoopbackPeerSkipsPairing(t *testing.T) {
	g, _ := newGate(t)
	g.Port = "1990"
	h := g.Wrap(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "127.0.0.1:1990"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback peer: %d", rec.Code)
	}
}

func TestLoopbackPeerPinsHost(t *testing.T) {
	g, _ := newGate(t)
	g.Port = "1990"
	h := g.Wrap(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "evil.com"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("rebinding host: %d", rec.Code)
	}
}
