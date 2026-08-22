package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/yanghanqing/homebase/internal/devices"
)

// deviceJSON is the public view of an enrolled device. It must not grow
// fields from devices.Device automatically — Hash and pending tokens stay
// off the wire.
type deviceJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

type devicesResponse struct {
	Devices []deviceJSON `json:"devices"`
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	requireLoopbackPeer(http.HandlerFunc(s.listDevices)).ServeHTTP(w, r)
}

func (s *Server) handleDevice(w http.ResponseWriter, r *http.Request) {
	requireLoopbackPeer(http.HandlerFunc(s.deleteDevice)).ServeHTTP(w, r)
}

func (s *Server) handleDevicesRevokeAll(w http.ResponseWriter, r *http.Request) {
	requireLoopbackPeer(http.HandlerFunc(s.revokeAllDevices)).ServeHTTP(w, r)
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	list, err := s.Devices.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list devices")
		return
	}
	out := make([]deviceJSON, 0, len(list))
	for _, d := range list {
		out = append(out, toDeviceJSON(d))
	}
	writeJSON(w, http.StatusOK, devicesResponse{Devices: out})
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	if err := s.Devices.Revoke(id); err != nil {
		if errors.Is(err, devices.ErrNotFound) {
			writeError(w, http.StatusNotFound, "device not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not revoke device")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) revokeAllDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.Devices.RevokeAll(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not revoke devices")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toDeviceJSON(d devices.Device) deviceJSON {
	return deviceJSON{
		ID:        d.ID,
		Name:      d.Name,
		CreatedAt: d.CreatedAt.UTC().Format(time.RFC3339),
	}
}
