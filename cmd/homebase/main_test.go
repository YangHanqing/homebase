package main

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStartHelpDoesNotStart(t *testing.T) {
	if code := runStart([]string{"-h"}); code != 0 {
		t.Fatalf("start -h exit %d", code)
	}
}

func TestHelpListsPublicCommandsOnly(t *testing.T) {
	for _, want := range []string{"start", "stop", "restart", "status", "pair", "version"} {
		if !strings.Contains(helpText, want) {
			t.Errorf("help missing %q", want)
		}
	}
	for _, hide := range []string{"access", "devices", "serve"} {
		// "access" appears in the Settings sentence — check command column.
		if strings.Contains(helpText, "  "+hide+" ") || strings.Contains(helpText, "  "+hide+"\n") {
			t.Errorf("help should not list %q", hide)
		}
	}
	if strings.Contains(helpText, "homebase access") || strings.Contains(helpText, "homebase devices") {
		t.Fatal("help still documents hidden subcommands")
	}
}

func TestVersionStringNotEmpty(t *testing.T) {
	if s := versionString(); s == "" {
		t.Fatal("version must never be empty")
	}
}

func TestLaunchdPlistUsesServe(t *testing.T) {
	xml := renderLaunchdPlist("/Users/me/.local/bin/homebase", "/Users/me/Library/Logs/homebase.log", "")
	if !strings.Contains(xml, "<string>serve</string>") {
		t.Fatal(xml)
	}
	if strings.Contains(xml, "-port") || strings.Contains(xml, "-listen") {
		t.Fatal("plist must not pin listen flags")
	}
	if !strings.Contains(xml, launchdLabel) {
		t.Fatal("missing label")
	}
}

func TestLaunchdPlistOptionalConfig(t *testing.T) {
	xml := renderLaunchdPlist("/opt/homebase", "/tmp/homebase.log", "/tmp/cfg.json")
	if !strings.Contains(xml, "<string>-config</string>") || !strings.Contains(xml, "<string>/tmp/cfg.json</string>") {
		t.Fatal(xml)
	}
}

func TestSystemdUnitUsesServe(t *testing.T) {
	unit := renderSystemdUnit("/home/me/.local/bin/homebase", "")
	if !strings.Contains(unit, "ExecStart=/home/me/.local/bin/homebase serve") {
		t.Fatal(unit)
	}
	if !strings.Contains(unit, "WantedBy=default.target") {
		t.Fatal(unit)
	}
	if strings.Contains(unit, "-port") {
		t.Fatal("unit must not pin -port")
	}
}

func TestProbeHealthOursAndOther(t *testing.T) {
	ours := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"listen":"127.0.0.1:1990"}`))
	}))
	defer ours.Close()
	p := probeHealth(ours.URL)
	if p.kind != probeOurs || p.listen != "127.0.0.1:1990" {
		t.Fatalf("ours: %+v", p)
	}

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("nginx"))
	}))
	defer other.Close()
	p = probeHealth(other.URL)
	if p.kind != probeOther {
		t.Fatalf("other: %+v", p)
	}

	p = probeHealth("http://127.0.0.1:1/api/health")
	if p.kind != probeNotRunning {
		t.Fatalf("closed port: %+v", p)
	}
}

func TestInstallScriptSyntax(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	script := filepath.Join(filepath.Dir(file), "..", "..", "install.sh")
	cmd := exec.Command("sh", "-n", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sh -n install.sh: %v\n%s", err, out)
	}
}

func TestNoTmuxHintMentionsInstall(t *testing.T) {
	var b strings.Builder
	printNoTmux(&b)
	got := b.String()
	if !strings.Contains(got, "tmux frontend") {
		t.Fatal(got)
	}
	if !strings.Contains(got, "homebase start") {
		t.Fatal(got)
	}
}
