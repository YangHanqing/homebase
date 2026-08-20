package web

import (
	"os"
	"strings"
	"testing"
)

func TestCopyOnSelectStaysInTheBrowser(t *testing.T) {
	src, err := os.ReadFile("js/terminal.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, needle := range []string{
		"homebaseBindCopyOnSelect",
		`document.execCommand("copy")`,
		"navigator.clipboard",
		"term.getSelection()",
	} {
		if !strings.Contains(s, needle) {
			t.Errorf("terminal.js missing %q", needle)
		}
	}
	if strings.Contains(s, "sendInput") {
		t.Fatal("selection must not go down the PTY")
	}
}
