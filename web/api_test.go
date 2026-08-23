package web

import (
	"regexp"
	"strings"
	"testing"
)

// Only the REST handlers answer in JSON. The auth gate serves the pairing
// instructions as HTML, and net/http's own rejections (bad host, method not
// allowed) are plain text. Parsing every response as JSON meant a revoked
// device read "Unexpected token '<'" in the sidebar instead of being told it
// was no longer paired.
func TestAPIHelperDoesNotAssumeJSON(t *testing.T) {
	s := readWeb(t, "js/app.js")
	body := apiHelper(t, s)

	if strings.Contains(body, "res.json()") {
		t.Error("api() must read the body as text and try JSON, not assume it")
	}
	if !strings.Contains(body, "res.text()") {
		t.Error("api() should read the body as text first")
	}
	if !strings.Contains(body, "JSON.parse") || !strings.Contains(body, "catch") {
		t.Error("api() should parse JSON defensively")
	}
}

// A 401 is not an ordinary error: the device was revoked or never paired, and
// the fix is not something the sidebar can express. Reload, and let the gate
// render the pairing page it already serves.
func TestAPIHelperTurns401IntoAReload(t *testing.T) {
	s := readWeb(t, "js/app.js")
	body := apiHelper(t, s)

	if !strings.Contains(body, "res.status === 401") {
		t.Fatal("api() must special-case 401")
	}
	if !strings.Contains(s, "location.reload()") {
		t.Error("an unpaired device should reload onto the gate's pairing page")
	}
	// Reloading from inside a 3s poll would otherwise fire on every tick.
	if !strings.Contains(s, "reloading") {
		t.Error("the reload must be guarded so polling cannot re-trigger it")
	}
}

// apiHelper returns the source of app.js's api() function.
func apiHelper(t *testing.T, s string) string {
	t.Helper()
	start := strings.Index(s, "function api(path, opts)")
	if start < 0 {
		t.Fatal("api() not found in app.js")
	}
	end := strings.Index(s[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("could not find the end of api()")
	}
	return s[start : start+end]
}

// Every string the UI can ask for has to exist in both dictionaries. t()
// falls back to English for a missing key, so a gap shows up as a stray
// English sentence in a Chinese UI rather than as any kind of failure.
func TestI18nDictionariesHaveTheSameKeys(t *testing.T) {
	s := readWeb(t, "js/i18n.js")
	en := dictKeys(t, s, "en")
	zh := dictKeys(t, s, "zh")
	if len(en) == 0 || len(zh) == 0 {
		t.Fatalf("parsed %d en and %d zh keys", len(en), len(zh))
	}
	for k := range en {
		if !zh[k] {
			t.Errorf("key %q is missing from the zh dictionary", k)
		}
	}
	for k := range zh {
		if !en[k] {
			t.Errorf("key %q is missing from the en dictionary", k)
		}
	}
}

var keyLine = regexp.MustCompile(`(?m)^\s*"([^"]+)":`)

// dictKeys pulls the keys out of one language block of i18n.js's DICT.
func dictKeys(t *testing.T, s, lang string) map[string]bool {
	t.Helper()
	start := strings.Index(s, "\n    "+lang+": {")
	if start < 0 {
		t.Fatalf("dictionary %q not found", lang)
	}
	rest := s[start:]
	end := strings.Index(rest, "\n    }")
	if end < 0 {
		t.Fatalf("could not find the end of dictionary %q", lang)
	}
	out := map[string]bool{}
	for _, m := range keyLine.FindAllStringSubmatch(rest[:end], -1) {
		out[m[1]] = true
	}
	return out
}

// The activity dot is an age, and an age needs two readings of the *same*
// clock. #{window_activity} comes off the server's clock, so the server sends
// its own "now" with the list and the browser subtracts one from the other. A
// phone that has been asleep, or any machine without NTP, can be minutes out;
// deriving the age from Date.now() would paint every window either frantic or
// dead, and would do it silently.
func TestActivityAgeUsesTheServerClock(t *testing.T) {
	s := readWeb(t, "js/app.js")
	start := strings.Index(s, "function activityState(w)")
	if start < 0 {
		t.Fatal("activityState() not found in app.js")
	}
	end := strings.Index(s[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("could not find the end of activityState()")
	}
	body := s[start : start+end]

	if strings.Contains(body, "Date.now()") || strings.Contains(body, "new Date") {
		t.Error("the activity age must not come from the browser's clock")
	}
	if !strings.Contains(body, "serverNow - w.activity") {
		t.Error("the age should be the server's now minus the window's activity")
	}
	if !strings.Contains(s, "body.now") {
		t.Error("refreshWindows must read the server's clock out of the response")
	}
}
