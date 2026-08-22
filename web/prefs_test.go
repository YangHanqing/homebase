package web

import (
	"os"
	"strings"
	"testing"
)

// Appearance preferences are per-device browser state. If they ever start
// talking to the server they would inherit the loopback gate that guards the
// listen address, and stop working from a paired phone.
func TestPrefsNeverTalkToTheServer(t *testing.T) {
	src, err := os.ReadFile("js/prefs.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	if !strings.Contains(s, "localStorage") {
		t.Error("prefs.js should persist to localStorage")
	}
	for _, banned := range []string{"fetch(", "XMLHttpRequest", "/api/"} {
		if strings.Contains(s, banned) {
			t.Errorf("prefs.js must not reach the server, found %q", banned)
		}
	}
}

// A denial from /api/settings (401 unpaired, 403 paired-but-not-loopback)
// must blank only the server-config half of the page, and the controls must
// stay hidden by default until a successful load proves this browser is
// actually on 127.0.0.1 -- no flash of editable controls while the request
// is in flight. Hiding every .settings-card, as an earlier version did, also
// hid the appearance tab for anyone not on 127.0.0.1.
func TestLoopbackDenialOnlyHidesSecurityPanel(t *testing.T) {
	src, err := os.ReadFile("js/settings.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	if !strings.Contains(s, `securityControls.hidden = true`) {
		t.Error("security controls must start (and stay, on denial) hidden")
	}
	if !strings.Contains(s, `securityControls.hidden = false`) {
		t.Error("security controls must only unhide on a successful /api/settings load")
	}
	if !strings.Contains(s, `err.status === 401 || err.status === 403`) {
		t.Error("both unpaired (401) and paired-but-not-loopback (403) must hide the controls")
	}
	if strings.Contains(s, `document.querySelectorAll(".settings-card, .settings-actions")`) {
		t.Error("403 handler still hides every settings card, including appearance")
	}

	html, err := os.ReadFile("settings.html")
	if err != nil {
		t.Fatal(err)
	}
	hs := string(html)
	secStart := strings.Index(hs, `id="panel-security"`)
	ctrlIdx := strings.Index(hs, `id="security-controls"`)
	if secStart < 0 || ctrlIdx < secStart {
		t.Fatal("security-controls must live inside panel-security")
	}
	if !strings.Contains(hs, `id="security-controls" hidden`) {
		t.Error("security-controls must be hidden by default in markup, not just via JS")
	}
}

// Both tabs must exist, and the security one must keep its loopback notice.
func TestSettingsPageHasBothPanels(t *testing.T) {
	src, err := os.ReadFile("settings.html")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, needle := range []string{
		`id="panel-appearance"`,
		`id="panel-security"`,
		`id="loopback-notice"`,
		`id="lang-radios"`,
		`value="auto"`,
		`id="pref-copy-on-select"`,
		`id="ranges-card"`,
		`id="btn-revoke-all"`,
	} {
		if !strings.Contains(s, needle) {
			t.Errorf("settings.html missing %q", needle)
		}
	}
	for _, banned := range []string{
		`id="theme-radios"`,
		`id="btn-lang"`,
		`s.httpTitle`,
	} {
		if strings.Contains(s, banned) {
			t.Errorf("settings.html should not contain %q", banned)
		}
	}
	// prefs.js must load in <head>, before first paint, or the pinned theme
	// flashes the wrong palette on every navigation.
	head := s
	if i := strings.Index(s, "</head>"); i >= 0 {
		head = s[:i]
	}
	if !strings.Contains(head, `src="/js/prefs.js"`) {
		t.Error("prefs.js must be loaded in <head>")
	}
}

func TestThemeControlLivesOnTheTerminalPage(t *testing.T) {
	idx, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	s := string(idx)
	if !strings.Contains(s, `data-theme-cycle`) {
		t.Error("index.html must expose a theme cycle control next to Settings")
	}
	if strings.Contains(s, `id="btn-lang"`) {
		t.Error("language must not be a corner toggle on the terminal page")
	}
	prefs, err := os.ReadFile("js/prefs.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prefs), "function cycleTheme") {
		t.Error("prefs.js must expose cycleTheme")
	}
}

func TestLanguageDefaultsToAuto(t *testing.T) {
	src, err := os.ReadFile("js/i18n.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	if !strings.Contains(s, `return "auto"`) {
		t.Error("i18n.js must default the language preference to auto")
	}
	if !strings.Contains(s, `saved === "auto"`) {
		t.Error("i18n.js must persist auto as a first-class preference")
	}
}

func TestRenameFieldSurvivesI18nApply(t *testing.T) {
	src, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	if !strings.Contains(s, `id="r-name"`) {
		t.Fatal("rename input missing")
	}
	// apply() sets textContent, which destroys nested controls. The input
	// must not live inside the data-i18n node.
	if strings.Contains(s, `data-i18n="rename.name">Name <input`) {
		t.Error("rename input is nested inside a data-i18n element")
	}
	i18n, err := os.ReadFile("js/i18n.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(i18n), `el.querySelector("input, textarea, select, button")`) {
		t.Error("i18n apply must refuse to wipe nested form controls")
	}
}

func TestLanAccessHidesTrustedRanges(t *testing.T) {
	src, err := os.ReadFile("js/settings.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `rangesCard.hidden = currentAccess() === "lan"`) {
		t.Error("settings.js must hide trusted ranges while Access is lan")
	}
}

// Copy-on-select is a preference now, but it must still default to on and
// still be decided at copy time so the toggle works without a reload.
func TestCopyOnSelectIsAPreferenceDefaultingToOn(t *testing.T) {
	prefs, err := os.ReadFile("js/prefs.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prefs), "copyOnSelect: true") {
		t.Error("copyOnSelect must default to true")
	}
	term, err := os.ReadFile("js/terminal.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(term)
	if !strings.Contains(s, `homebasePref("copyOnSelect", true)`) {
		t.Error("terminal.js must consult the copyOnSelect preference")
	}
	// The check belongs inside copySelection, not around the bind call.
	i := strings.Index(s, "function copySelection()")
	j := strings.Index(s, `homebasePref("copyOnSelect", true)`)
	if i < 0 || j < i {
		t.Error("the copyOnSelect check must be inside copySelection()")
	}
}
