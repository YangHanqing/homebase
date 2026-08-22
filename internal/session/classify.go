package session

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/yanghanqing/homebase/internal/tmux"
)

// Status codes surfaced on the WS status frame. Never include secrets.
const (
	CodeENOTMUX  = "enotmux"
	CodePTYSpawn = "pty_spawn"
	CodeWSClosed = "ws_closed"
	CodeUnknown  = "unknown"
)

const maxMessage = 200

// Classify turns a spawn failure or the process's stderr into a status code.
// stderr is the child's own stream, never PTY contents.
//
// The two failures worth telling apart are "tmux is not installed" and
// everything else: the first has an action the user can take, and the UI
// carries a hint for it.
func Classify(stderr []byte, spawnErr error) (code, message string) {
	if spawnErr != nil {
		if errors.Is(spawnErr, tmux.ErrNoTmux) {
			return CodeENOTMUX, trimMessage(spawnErr.Error())
		}
		return CodePTYSpawn, trimMessage(spawnErr.Error())
	}
	if strings.TrimSpace(string(stderr)) == "" {
		return "", ""
	}
	return CodeUnknown, trimMessage(firstLine(string(stderr)))
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return s
}

func trimMessage(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) <= maxMessage {
		if utf8.ValidString(s) {
			return s
		}
		return string([]rune(s))
	}
	// byte cap, then back up to a rune boundary
	cut := s[:maxMessage]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}
