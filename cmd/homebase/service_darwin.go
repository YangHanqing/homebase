//go:build darwin

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func launchdPlistPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("Library", "LaunchAgents", launchdLabel+".plist")
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
}

func launchdLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "homebase.log"
	}
	return filepath.Join(home, "Library", "Logs", "homebase.log")
}

func guiDomain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func applyService(exe, configPath string) error {
	plist := launchdPlistPath()
	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(launchdLogPath()), 0o755); err != nil {
		return err
	}
	body := renderLaunchdPlist(exe, launchdLogPath(), configPath)
	if err := os.WriteFile(plist, []byte(body), 0o644); err != nil {
		return err
	}
	_, _ = launchctl("bootout", guiDomain()+"/"+launchdLabel)
	out, err := launchctl("bootstrap", guiDomain(), plist)
	if err != nil {
		return fmt.Errorf("launchctl bootstrap: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func stopService() error {
	out, err := launchctl("bootout", guiDomain()+"/"+launchdLabel)
	if err == nil {
		return nil
	}
	msg := strings.ToLower(string(out) + err.Error())
	if strings.Contains(msg, "no such") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "could not find") ||
		strings.Contains(msg, "5:") ||
		strings.Contains(msg, "3:") {
		return nil
	}
	return fmt.Errorf("launchctl bootout: %s", strings.TrimSpace(string(out)))
}

func serviceActive() bool {
	out, err := launchctl("print", guiDomain()+"/"+launchdLabel)
	if err != nil {
		return false
	}
	s := string(out)
	return strings.Contains(s, "state = running") || strings.Contains(s, "pid = ")
}

func logLocation() string {
	return launchdLogPath()
}

func logHint() string {
	return "See " + launchdLogPath()
}

func checkUserBus() error { return nil }

func warnSessionLinger() {}

func recoverAfterFailedStart(exe string) bool {
	if err := exec.Command("codesign", "-v", exe).Run(); err == nil {
		return false
	}
	if _, err := exec.LookPath("codesign"); err != nil {
		fmt.Fprintf(os.Stderr, "the binary failed codesign verification and codesign is not installed.\nInstall Xcode Command Line Tools, then:\n  codesign -s - -f %s\n  homebase start\n", exe)
		return false
	}
	cmd := exec.Command("codesign", "-s", "-", "-f", exe)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "codesign failed: %s\n  codesign -s - -f %s\n", bytes.TrimSpace(out), exe)
		return false
	}
	return true
}

func launchctl(args ...string) ([]byte, error) {
	cmd := exec.Command("launchctl", args...)
	return cmd.CombinedOutput()
}
