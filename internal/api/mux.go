package api

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/yanghanqing/homebase/internal/config"
	"github.com/yanghanqing/homebase/internal/devices"
	"github.com/yanghanqing/homebase/internal/session"
	"github.com/yanghanqing/homebase/internal/tmux"
	"github.com/yanghanqing/homebase/internal/ws"
	"github.com/yanghanqing/homebase/web"
)

// Server holds HTTP dependencies.
type Server struct {
	Store   *config.Store
	Devices *devices.Store
	Listen  string
	Dialer  session.Dialer
	Log     *slog.Logger
	// NewTmux overrides control-channel construction in tests only.
	NewTmux func() tmux.Client
}

// NewMux is the HTTP surface.
func NewMux(listen string, store *config.Store, dialer session.Dialer, log *slog.Logger) http.Handler {
	return NewMuxServer(&Server{Store: store, Listen: listen, Dialer: dialer, Log: log})
}

// NewMuxServer wires routes onto an already-built Server, so tests can inject
// a fake control channel.
func NewMuxServer(s *Server) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", HealthHandler(s.Listen))

	if s.Store != nil {
		mux.HandleFunc("GET /api/windows", s.handleTmuxWindows)
		mux.HandleFunc("POST /api/windows", s.handleTmuxWindows)
		mux.HandleFunc("PUT /api/windows/{index}", s.handleTmuxWindow)
		mux.HandleFunc("DELETE /api/windows/{index}", s.handleTmuxWindow)

		mux.HandleFunc("GET /api/settings", s.handleSettings)
		mux.HandleFunc("PUT /api/settings", s.handleSettings)
	}
	if s.Devices != nil {
		mux.HandleFunc("GET /api/devices", s.handleDevices)
		mux.HandleFunc("DELETE /api/devices/{id}", s.handleDevice)
	}
	if s.Store != nil && s.Dialer != nil {
		mux.Handle("GET /ws", &ws.Hub{Dialer: s.Dialer, Log: s.Log})
	}

	sub, err := fs.Sub(web.FS, ".")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))
	return requireOrigin(mux)
}

// NewMuxHealth is kept for tests that only need health + static.
func NewMuxHealth(listen string) http.Handler {
	return NewMux(listen, nil, nil, nil)
}
