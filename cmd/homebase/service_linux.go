//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

func systemdUnitPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "systemd", "user", systemdUnit)
	}
	return filepath.Join(home, ".config", "systemd", "user", systemdUnit)
}

func applyService(exe, configPath string) error {
	path := systemdUnitPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := renderSystemdUnit(exe, configPath)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}
	if out, err := systemctl("daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %s", strings.TrimSpace(string(out)))
	}
	if out, err := systemctl("enable", "--now", systemdUnit); err != nil {
		return fmt.Errorf("systemctl enable --now: %s", strings.TrimSpace(string(out)))
	}
	if out, err := systemctl("restart", systemdUnit); err != nil {
		return fmt.Errorf("systemctl restart: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func stopService() error {
	out, err := systemctl("stop", systemdUnit)
	if err == nil {
		return nil
	}
	msg := strings.ToLower(string(out) + err.Error())
	if strings.Contains(msg, "not loaded") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "inactive") {
		return nil
	}
	return fmt.Errorf("systemctl stop: %s", strings.TrimSpace(string(out)))
}

func serviceActive() bool {
	return exec.Command("systemctl", "--user", "is-active", "--quiet", systemdUnit).Run() == nil
}

func logLocation() string {
	return "journalctl --user -u homebase"
}

func logHint() string {
	return "journalctl --user -u homebase -n 50"
}

func checkUserBus() error {
	if os.Getenv("XDG_RUNTIME_DIR") != "" {
		return nil
	}
	uid := os.Getuid()
	name := os.Getenv("USER")
	if name == "" {
		if u, err := user.Current(); err == nil {
			name = u.Username
		}
	}
	return fmt.Errorf("XDG_RUNTIME_DIR is not set; systemd --user cannot connect to the bus.\nTry:\n  export XDG_RUNTIME_DIR=/run/user/%d\nor re-login / `machinectl shell %s@`.", uid, name)
}

func warnSessionLinger() {
	userName := os.Getenv("USER")
	if userName == "" {
		if u, err := user.Current(); err == nil {
			userName = u.Username
		} else {
			return
		}
	}
	out, err := exec.Command("loginctl", "show-user", userName, "-p", "Linger", "--value").Output()
	if err != nil {
		return
	}
	if strings.TrimSpace(string(out)) == "yes" {
		return
	}
	fmt.Fprintf(os.Stderr, "\nThis user session will end on logout, taking Homebase with it.\nTo keep it running after SSH disconnect:\n\n  sudo loginctl enable-linger %s\n\n", userName)
}

func recoverAfterFailedStart(exe string) bool { return false }

func systemctl(args ...string) ([]byte, error) {
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	return cmd.CombinedOutput()
}
