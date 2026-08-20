// Package ws upgrades /ws and bridges a session.Proc to the browser.
package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yanghanqing/homebase/internal/session"
)

const (
	writeWait  = 10 * time.Second
	readLimit  = 1 << 20
	readIdle   = 90 * time.Second
	ptyBufSize = 32 * 1024
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return originOK(r)
	},
}

// Hub serves one WebSocket per request (Model A).
type Hub struct {
	Dialer session.Dialer
	Log    *slog.Logger
}

func (h *Hub) log() *slog.Logger {
	if h.Log != nil {
		return h.Log
	}
	return slog.Default()
}

// ServeHTTP upgrades and pipes Proc ↔ browser.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log().Debug("ws upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	h.log().Info("ws open")
	h.bridge(r.Context(), conn)
	h.log().Info("ws close")
}

func (h *Hub) bridge(parent context.Context, raw *websocket.Conn) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	c := &safeConn{c: raw}
	_ = c.status("connecting", "", "")

	if h.Dialer == nil {
		_ = c.status("error", session.CodePTYSpawn, "no dialer")
		return
	}

	proc, err := h.Dialer.Start(ctx, session.Size{Cols: 80, Rows: 24})
	if err != nil {
		code, msg := session.Classify(nil, false, err)
		h.log().Info("pty spawn failed", "code", code, "err", err)
		_ = c.status("error", code, msg)
		return
	}
	defer func() { _ = proc.Kill() }()

	_ = c.status("connected", "", "")

	var sawENOTMUX bool
	var sawMu sync.Mutex

	go func() {
		buf := make([]byte, ptyBufSize)
		peek := make([]byte, 0, 64)
		for {
			n, err := proc.Read(buf)
			if n > 0 {
				chunk := buf[:n]
				sawMu.Lock()
				if !sawENOTMUX && len(peek) < 64 {
					need := 64 - len(peek)
					if len(chunk) < need {
						need = len(chunk)
					}
					peek = append(peek, chunk[:need]...)
					if session.SawENOTMUX(peek) {
						sawENOTMUX = true
					}
				}
				sawMu.Unlock()
				if werr := c.binary(chunk); werr != nil {
					cancel()
					return
				}
			}
			if err != nil {
				cancel()
				return
			}
		}
	}()

	raw.SetReadLimit(readLimit)
	_ = raw.SetReadDeadline(time.Now().Add(readIdle))

	go func() {
		<-ctx.Done()
		_ = raw.SetReadDeadline(time.Now())
	}()

	for {
		mt, payload, err := raw.ReadMessage()
		if err != nil {
			break
		}
		_ = raw.SetReadDeadline(time.Now().Add(readIdle))
		switch mt {
		case websocket.BinaryMessage:
			if _, werr := proc.Write(payload); werr != nil {
				cancel()
			}
		case websocket.TextMessage:
			h.handleText(proc, c, payload)
		}
	}

	cancel()
	_ = proc.Kill()
	waitErr := proc.Wait()

	sawMu.Lock()
	enot := sawENOTMUX
	sawMu.Unlock()
	var stderr []byte
	if s, ok := proc.(session.Stderrer); ok {
		stderr = s.Stderr()
	}
	code, msg := session.Classify(stderr, enot, nil)
	if code != "" {
		h.log().Info("pty exit", "code", code, "wait_err", waitErr != nil)
		_ = c.status("error", code, msg)
		return
	}
	_ = c.status("disconnected", session.CodeWSClosed, "")
}

func (h *Hub) handleText(proc session.Proc, c *safeConn, payload []byte) {
	var msg struct {
		Type string `json:"type"`
		Cols int    `json:"cols"`
		Rows int    `json:"rows"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil {
		h.log().Debug("ws ignore malformed json")
		return
	}
	switch msg.Type {
	case "resize":
		if msg.Cols < 1 || msg.Cols > 512 || msg.Rows < 1 || msg.Rows > 512 {
			return
		}
		sz := session.Size{Cols: uint16(msg.Cols), Rows: uint16(msg.Rows)}
		if err := proc.SetSize(sz); err != nil {
			h.log().Debug("resize failed", "err", err)
		} else {
			h.log().Debug("resize", "cols", msg.Cols, "rows", msg.Rows)
		}
	case "ping":
		_ = c.pong()
	default:
		// unknown type: ignore
	}
}

type safeConn struct {
	mu sync.Mutex
	c  *websocket.Conn
}

func (s *safeConn) binary(p []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.c.SetWriteDeadline(time.Now().Add(writeWait))
	return s.c.WriteMessage(websocket.BinaryMessage, p)
}

func (s *safeConn) text(v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.c.SetWriteDeadline(time.Now().Add(writeWait))
	return s.c.WriteJSON(v)
}

func (s *safeConn) status(state, code, message string) error {
	return s.text(map[string]string{
		"type":    "status",
		"state":   state,
		"code":    code,
		"message": message,
	})
}

func (s *safeConn) pong() error {
	return s.text(map[string]string{"type": "pong"})
}

func originOK(r *http.Request) bool {
	raw := r.Header.Get("Origin")
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}
