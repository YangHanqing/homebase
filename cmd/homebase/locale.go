package main

import (
	"os"
	"os/exec"
	"strings"
)

func uiChinese() bool {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if zhLocale(os.Getenv(k)) {
			return true
		}
	}
	out, err := exec.Command("defaults", "read", "-g", "AppleLocale").Output()
	if err != nil {
		return false
	}
	return zhLocale(string(out))
}

func zhLocale(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(s, "zh")
}
