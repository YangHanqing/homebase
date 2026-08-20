// Package auth gates the HTTP surface on an enrolled device cookie, and pins
// Host headers when the listener is unauthenticated loopback.
//
// There is no password. Enrollment is `homebase pair` on the machine itself
// (see internal/devices), so the credential that gets you in is the one you
// already needed to have.
package auth

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/yanghanqing/homebase/internal/devices"
)

// CookieName holds the session secret minted at pairing time.
const CookieName = "homebase_session"

// PairPath redeems a one-time token from `homebase pair`.
const PairPath = "/pair"

// sessionMaxAge is deliberately long: re-pairing means walking back to a shell
// on the host, so expiring sessions on a timer would be pure friction. Devices
// are revoked explicitly instead (`homebase devices revoke`).
const sessionMaxAge = 365 * 24 * time.Hour

// Gate authenticates requests against enrolled devices.
type Gate struct {
	Devices *devices.Store
	Log     *slog.Logger
}

// Wrap requires a valid session cookie on every request, including the
// WebSocket upgrade, and serves the pairing redemption endpoint.
func (g *Gate) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PairPath {
			g.handlePair(w, r)
			return
		}
		c, err := r.Cookie(CookieName)
		if err != nil || c.Value == "" {
			g.deny(w, r)
			return
		}
		if _, ok := g.Devices.Verify(c.Value, time.Now()); !ok {
			g.deny(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (g *Gate) handlePair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := r.URL.Query().Get("t")
	secret, dev, err := g.Devices.Redeem(token, deviceName(r), time.Now())
	if err != nil {
		if g.Log != nil {
			// Never log the token itself.
			g.Log.Warn("pairing rejected", "remote", clientIP(r), "reason", err)
		}
		g.deny(w, r)
		return
	}
	// Secure is deliberately false: this build never serves TLS, and marking
	// the cookie HTTPS-only on a plain-HTTP listener would make the browser
	// withhold it and lock everyone out.
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    secret,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionMaxAge.Seconds()),
	})
	if g.Log != nil {
		g.Log.Info("device paired", "id", dev.ID, "name", dev.Name, "remote", clientIP(r))
	}
	// Redirect immediately so the token stops existing in the address bar,
	// browser history, and any Referer this page would otherwise send.
	http.Redirect(w, r, "/", http.StatusFound)
}

func (g *Gate) deny(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if wantsHTML(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(pairPage))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"not paired","hint":"run 'homebase pair' on the Homebase machine"}`))
}

func wantsHTML(r *http.Request) bool {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
		return false
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

const pairPage = `<!doctype html>
<meta charset="utf-8">
<title>Homebase — not paired</title>
<style>
  body{font:16px/1.6 system-ui,sans-serif;margin:0;display:grid;place-items:center;
       min-height:100vh;background:#111;color:#eee}
  main{max-width:34rem;padding:2rem}
  h1{font-size:1.25rem;margin:0 0 1rem}
  code{font:14px ui-monospace,monospace;background:#222;padding:.15rem .4rem;border-radius:4px}
  pre{background:#222;padding:1rem;border-radius:8px;overflow-x:auto}
  p{color:#bbb}
</style>
<main>
  <h1>This device is not paired yet</h1>
  <p>On the machine running Homebase (local terminal, or ssh in), run:</p>
  <pre><code>homebase pair</code></pre>
  <p>It prints a link valid for 10 minutes, usable once. Open that link on this device.</p>
</main>
`

// deviceName labels the enrolled device from its User-Agent. It is cosmetic —
// shown in `homebase devices` so the operator can tell which row to revoke.
func deviceName(r *http.Request) string {
	ua := strings.TrimSpace(r.Header.Get("User-Agent"))
	if ua == "" {
		return "unknown browser"
	}
	if len(ua) > 120 {
		ua = ua[:120]
	}
	return ua
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RequireLoopbackHost rejects requests whose Host header is not a loopback
// name. This is what makes the unauthenticated local tier safe: without it, any
// web page could point a hostname it controls at 127.0.0.1 (DNS rebinding), and
// the browser would treat the resulting requests as same-origin — handing an
// arbitrary site a shell on this machine.
func RequireLoopbackHost(port string, next http.Handler) http.Handler {
	allowed := map[string]bool{
		"127.0.0.1:" + port: true,
		"localhost:" + port: true,
		"[::1]:" + port:     true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowed[strings.ToLower(r.Host)] {
			http.Error(w, "bad host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// AllowedLoopbackHost reports whether host would pass RequireLoopbackHost.
func AllowedLoopbackHost(host, port string) bool {
	switch strings.ToLower(host) {
	case "127.0.0.1:" + port, "localhost:" + port, "[::1]:" + port:
		return true
	}
	return false
}
