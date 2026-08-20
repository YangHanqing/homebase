package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yanghanqing/homebase/internal/devices"
)

func testDevices(t *testing.T) *devices.Store {
	t.Helper()
	s, err := devices.Open(filepath.Join(t.TempDir(), "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func enroll(t *testing.T, s *devices.Store, name string) devices.Device {
	t.Helper()
	now := time.Now()
	tok, _, err := s.Mint(now)
	if err != nil {
		t.Fatal(err)
	}
	_, dev, err := s.Redeem(tok, name, now)
	if err != nil {
		t.Fatal(err)
	}
	return dev
}

func devicesMux(t *testing.T, dev *devices.Store) http.Handler {
	t.Helper()
	return NewMuxServer(&Server{Store: testStore(t), Devices: dev})
}

func TestListDevicesOmitsSecrets(t *testing.T) {
	dev := testDevices(t)
	enroll(t, dev, "Phone")
	h := devicesMux(t, dev)
	rec := doFrom(t, h, http.MethodGet, "/api/devices", "127.0.0.1:9", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	if strings.Contains(raw, `"hash"`) {
		t.Fatalf("hash leaked: %s", raw)
	}
	if strings.Contains(raw, "pending") {
		t.Fatalf("pending leaked: %s", raw)
	}
	body := decode[devicesResponse](t, rec)
	if len(body.Devices) != 1 {
		t.Fatalf("got %+v", body.Devices)
	}
	if body.Devices[0].Name != "Phone" || body.Devices[0].ID == "" || body.Devices[0].CreatedAt == "" {
		t.Fatalf("got %+v", body.Devices[0])
	}
}

func TestListDevicesRequiresLoopback(t *testing.T) {
	h := devicesMux(t, testDevices(t))
	rec := doFrom(t, h, http.MethodGet, "/api/devices", "192.0.2.1:9", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListDevicesEmpty(t *testing.T) {
	h := devicesMux(t, testDevices(t))
	rec := doFrom(t, h, http.MethodGet, "/api/devices", "127.0.0.1:9", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Devices json.RawMessage `json:"devices"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if string(body.Devices) != "[]" {
		t.Fatalf("want empty array, got %s", body.Devices)
	}
}

func TestDeleteDevice(t *testing.T) {
	dev := testDevices(t)
	d := enroll(t, dev, "Laptop")
	h := devicesMux(t, dev)

	rec := doFrom(t, h, http.MethodDelete, "/api/devices/"+d.ID, "127.0.0.1:9", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	list, err := dev.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("still have %d devices", len(list))
	}
}

func TestDeleteDeviceMissing(t *testing.T) {
	h := devicesMux(t, testDevices(t))
	rec := doFrom(t, h, http.MethodDelete, "/api/devices/nope", "127.0.0.1:9", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteDeviceRequiresLoopback(t *testing.T) {
	dev := testDevices(t)
	d := enroll(t, dev, "Phone")
	h := devicesMux(t, dev)
	rec := doFrom(t, h, http.MethodDelete, "/api/devices/"+d.ID, "192.0.2.1:9", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	list, _ := dev.List()
	if len(list) != 1 {
		t.Fatal("non-loopback revoke must not succeed")
	}
}

func TestSettingsPutSignalsRestart(t *testing.T) {
	h := NewMuxServer(&Server{Store: testStore(t)})
	rec := doFrom(t, h, http.MethodPut, "/api/settings", "127.0.0.1:9",
		map[string]any{"access": "local", "trusted_ranges": []string{"100.64.0.0/10"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := decode[map[string]any](t, rec)
	if body["restart_required"] != true {
		t.Fatalf("restart_required=%v", body["restart_required"])
	}
}
