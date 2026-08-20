// Package ident validates tmux window fields before they are stored or
// turned into argv.
package ident

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxNameLen   = 64
	maxWindowIdx = 9999
)

// ValidateName checks a tmux window name: trimmed 1–64 runes, no control
// characters. Spaces are fine; tmux allows them in window names, and every
// use site quotes the value.
func ValidateName(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("name is empty")
	}
	if utf8.RuneCountInString(s) > maxNameLen {
		return fmt.Errorf("name is longer than %d characters", maxNameLen)
	}
	// A bare ";" is tmux's own command separator, so it never survives as a
	// window name -- reject it here instead of letting tmux fail confusingly.
	if s == ";" {
		return fmt.Errorf("name is reserved")
	}
	for _, r := range s {
		if r == utf8.RuneError || unicode.IsControl(r) {
			return fmt.Errorf("name contains a control character")
		}
	}
	return nil
}

// ValidateWindowIndex checks a tmux window index: 0–9999.
func ValidateWindowIndex(i int) error {
	if i < 0 || i > maxWindowIdx {
		return fmt.Errorf("window index out of range")
	}
	return nil
}
