package ws

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yanghanqing/homebase/internal/session"
)

type fakeProc struct {
	mu     sync.Mutex
	writes [][]byte
	sizes  []session.Size
	pr     *io.PipeReader
	pw     *io.PipeWriter
	closed chan struct{}
	once   sync.Once
}

func newFakeProc() *fakeProc {
	pr, pw := io.Pipe()
	return &fakeProc{pr: pr, pw: pw, closed: make(chan struct{})}
}

func (p *fakeProc) Read(b []byte) (int, error) {
	return p.pr.Read(b)
}

func (p *fakeProc) push(b []byte) error {
	_, err := p.pw.Write(b)
	return err
}

func (p *fakeProc) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]byte, len(b))
	copy(cp, b)
	p.writes = append(p.writes, cp)
	return len(b), nil
}

func (p *fakeProc) SetSize(sz session.Size) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sizes = append(p.sizes, sz)
	return nil
}

func (p *fakeProc) Wait() error {
	<-p.closed
	return nil
}

func (p *fakeProc) Kill() error {
	p.once.Do(func() {
		_ = p.pw.Close()
		_ = p.pr.Close()
		close(p.closed)
	})
	return nil
}

func (p *fakeProc) lastSize() (session.Size, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.sizes) == 0 {
		return session.Size{}, false
	}
	return p.sizes[len(p.sizes)-1], true
}

func (p *fakeProc) wrote() [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([][]byte, len(p.writes))
	copy(out, p.writes)
	return out
}

type fakeDialer struct {
	proc session.Proc
}

func (d *fakeDialer) Start(_ context.Context, _ session.Size) (session.Proc, error) {
	return d.proc, nil
}

func setupWS(t *testing.T, d session.Dialer) (*httptest.Server, func() (*websocket.Conn, *http.Response, error)) {
	t.Helper()
	h := &Hub{Dialer: d}
	mux := http.NewServeMux()
	mux.Handle("GET /ws", h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	dial := func() (*websocket.Conn, *http.Response, error) {
		u := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
		return websocket.DefaultDialer.Dial(u, nil)
	}
	return srv, dial
}

func readJSON(t *testing.T, c *websocket.Conn) map[string]string {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	mt, b, err := c.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if mt != websocket.TextMessage {
		t.Fatalf("want text, got %d", mt)
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestBinaryAndResizeAndUnknownType(t *testing.T) {
	fp := newFakeProc()
	_, dial := setupWS(t, &fakeDialer{proc: fp})
	c, _, err := dial()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if st := readJSON(t, c); st["state"] != "connecting" {
		t.Fatalf("got %+v", st)
	}
	if st := readJSON(t, c); st["state"] != "connected" {
		t.Fatalf("got %+v", st)
	}

	stdin := []byte{0xff, 0x00, 0x80, 'a'}
	if err := c.WriteMessage(websocket.BinaryMessage, stdin); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteJSON(map[string]any{"type": "resize", "cols": 120, "rows": 40}); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteJSON(map[string]any{"type": "nope"}); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteMessage(websocket.TextMessage, []byte("not-json")); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteJSON(map[string]any{"type": "resize", "cols": 0, "rows": 10}); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteJSON(map[string]any{"type": "ping"}); err != nil {
		t.Fatal(err)
	}
	if err := fp.push([]byte{0xff, 0xfe, 0x00}); err != nil {
		t.Fatal(err)
	}

	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	var sawPong, sawBinary bool
	for !sawPong || !sawBinary {
		mt, b, err := c.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if mt == websocket.TextMessage {
			var m map[string]string
			_ = json.Unmarshal(b, &m)
			if m["type"] == "pong" {
				sawPong = true
			}
			continue
		}
		if mt == websocket.BinaryMessage {
			if string(b) != string([]byte{0xff, 0xfe, 0x00}) {
				t.Fatalf("payload mutated: %v", b)
			}
			sawBinary = true
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sz, ok := fp.lastSize(); ok && sz.Cols == 120 && sz.Rows == 40 {
			for _, w := range fp.wrote() {
				if string(w) == string(stdin) {
					return
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("stdin/resize not applied; writes=%v size=%v", fp.wrote(), func() any { sz, _ := fp.lastSize(); return sz }())
}
