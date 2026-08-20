package session

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/yanghanqing/homebase/internal/tmux"
)

// Status codes surfaced on the WS status frame. Never include secrets.
const (
	CodeSSHAuth    = "ssh_auth"
	CodeSSHHostKey = "ssh_hostkey"
	CodeSSHTimeout = "ssh_timeout"
	CodeSSHRefused = "ssh_refused"
	CodeENOTMUX    = "enotmux"
	CodePTYSpawn   = "pty_spawn"
	CodeWSClosed   = "ws_closed"
	CodeUnknown    = "unknown"
)

const (
	enotmuxMarker = tmux.ENOTMUXMarker
	maxMessage    = 200
)

// Classify maps ssh stderr / spawn errors / the ENOTMUX marker to a code.
// stderr is ssh's own stream, not PTY contents.
func Classify(stderr []byte, sawENOTMUX bool, spawnErr error) (code, message string) {
	if spawnErr != nil {
		return CodePTYSpawn, trimMessage(spawnErr.Error())
	}
	if sawENOTMUX || bytes.Contains(stderr, []byte(enotmuxMarker)) {
		return CodeENOTMUX, enotmuxMarker
	}
	s := string(stderr)
	switch {
	case strings.Contains(s, "Permission denied"):
		return CodeSSHAuth, trimMessage(firstLine(s))
	case strings.Contains(s, "Host key verification failed"):
		return CodeSSHHostKey, trimMessage(firstLine(s))
	case strings.Contains(s, "Connection timed out") || strings.Contains(s, "Connection timed out during banner exchange") || strings.Contains(s, "ConnectTimeout"):
		return CodeSSHTimeout, trimMessage(firstLine(s))
	case strings.Contains(s, "Connection refused"):
		return CodeSSHRefused, trimMessage(firstLine(s))
	case strings.TrimSpace(s) == "":
		return "", ""
	default:
		return CodeUnknown, trimMessage(firstLine(s))
	}
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

// SawENOTMUX reports whether b contains the remote sentinel.
func SawENOTMUX(b []byte) bool {
	return bytes.Contains(b, []byte(enotmuxMarker))
}
