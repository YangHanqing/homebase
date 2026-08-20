package listen

import (
	"context"
	"testing"

	"github.com/yanghanqing/homebase/internal/config"
)

var defaultRanges = []string{"100.64.0.0/10"}

func noLookup(t *testing.T) LookupIPv4 {
	return func(context.Context) (string, error) {
		t.Helper()
		t.Fatal("lookup should not run")
		return "", nil
	}
}

func TestLocalTierBindsLoopback(t *testing.T) {
	r, err := Resolve(config.AccessLocal, "", "", 7681, defaultRanges, noLookup(t))
	if err != nil {
		t.Fatal(err)
	}
	if r.Addr != "127.0.0.1:7681" {
		t.Fatalf("got %q", r.Addr)
	}
	if !r.Loopback || r.NeedsAuth() {
		t.Fatalf("local tier should be loopback and unauthenticated: %+v", r)
	}
	if !r.NeedsHostCheck() {
		t.Fatal("loopback must pin the Host header")
	}
}

// The tier is the gate: -listen picks an address but cannot turn a loopback-only
// install into a routable one.
func TestListenFlagCannotEscapeLocalTier(t *testing.T) {
	for _, in := range []string{"0.0.0.0", "0.0.0.0:7681", "192.168.1.8", "[::]:7681"} {
		if _, err := Resolve(config.AccessLocal, in, "", 7681, defaultRanges, nil); err == nil {
			t.Errorf("Resolve(local, %q) succeeded, want refuse", in)
		}
	}
}

func TestLANTierBindsUnspecified(t *testing.T) {
	r, err := Resolve(config.AccessLAN, "", "", 7681, defaultRanges, noLookup(t))
	if err != nil {
		t.Fatal(err)
	}
	if r.Addr != "0.0.0.0:7681" || !r.Unspecified {
		t.Fatalf("got %+v", r)
	}
	if !r.NeedsAuth() {
		t.Fatalf("lan tier must require pairing: %+v", r)
	}
	if r.NeedsHostCheck() {
		t.Fatal("host pinning is only for the unauthenticated tier")
	}
}

func TestPrivateTierUsesTrustedRange(t *testing.T) {
	r, err := Resolve(config.AccessPrivate, "", "", 7681, defaultRanges, func(context.Context) (string, error) {
		return "100.64.1.20\n", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// An interface scan runs first and may legitimately find a real Tailscale
	// address on the machine running the tests; either way the posture is what
	// matters.
	if !r.Trusted {
		t.Skip("no trusted-range address available in this environment")
	}
	if r.TierFallback {
		t.Fatal("should not fall back when a trusted address exists")
	}
	if !r.NeedsAuth() {
		t.Fatal("private tier still requires pairing")
	}
	loop := "127.0.0.1:" + "7681"
	found := false
	for _, a := range r.BindAddrs() {
		if a == loop {
			found = true
		}
	}
	if !found {
		t.Fatalf("private tier must also bind loopback, got %v", r.BindAddrs())
	}
}

func TestPrivateTierUsesCustomRange(t *testing.T) {
	custom := []string{"203.0.113.0/24"}
	r, err := Resolve(config.AccessPrivate, "", "", 7681, custom, func(context.Context) (string, error) {
		return "203.0.113.5", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.TierFallback || !r.Trusted || r.Host != "203.0.113.5" {
		t.Fatalf("expected custom-range address to be picked up: %+v", r)
	}
	if got := r.BindAddrs(); len(got) < 2 || got[1] != "127.0.0.1:7681" {
		t.Fatalf("private must also bind loopback, got %v", got)
	}
}

func TestPrivateTierFallsBackToLoopback(t *testing.T) {
	if scanInterfaces(ParseTrustedRanges(defaultRanges)) != "" {
		t.Skip("machine has a real Tailscale address")
	}
	r, err := Resolve(config.AccessPrivate, "", "", 7681, defaultRanges, func(context.Context) (string, error) {
		return "", context.DeadlineExceeded
	})
	if err != nil {
		t.Fatal(err)
	}
	if !r.TierFallback || r.Addr != "127.0.0.1:7681" {
		t.Fatalf("got %+v", r)
	}
	// Falling back must also drop the auth requirement, otherwise the operator
	// is locked out of a loopback listener they cannot pair against.
	if r.NeedsAuth() {
		t.Fatal("loopback fallback should not require pairing")
	}
}

func TestClassifyPosture(t *testing.T) {
	nets := ParseTrustedRanges(defaultRanges)
	cases := []struct {
		host          string
		loop, trusted bool
	}{
		{"127.0.0.1", true, false},
		{"localhost", true, false},
		{"100.64.1.20", false, true},
		{"100.127.255.254", false, true},
		{"192.168.1.8", false, false},
		{"100.128.0.1", false, false}, // just outside 100.64.0.0/10
		{"0.0.0.0", false, false},
	}
	for _, c := range cases {
		r := classify(c.host, 7681, nets)
		if r.Loopback != c.loop || r.Trusted != c.trusted {
			t.Errorf("%s: loopback=%v trusted=%v, want %v/%v",
				c.host, r.Loopback, r.Trusted, c.loop, c.trusted)
		}
	}
}

func TestParseTailscaleIPv4(t *testing.T) {
	ip, err := parseTailscaleIPv4("100.64.0.5\nextra\n")
	if err != nil {
		t.Fatal(err)
	}
	if ip != "100.64.0.5" {
		t.Fatalf("got %q", ip)
	}
	if _, err := parseTailscaleIPv4("not-an-ip\n"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := parseTailscaleIPv4("0.0.0.0\n"); err == nil {
		t.Fatal("unspecified is not unicast")
	}
}

func TestFlagHostOverridesConfig(t *testing.T) {
	r, err := Resolve(config.AccessLAN, "192.168.1.8", "10.0.0.1:9", 7681, defaultRanges, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Addr != "192.168.1.8:7681" {
		t.Fatalf("got %q", r.Addr)
	}
}

func TestListenWithEmbeddedPort(t *testing.T) {
	r, err := Resolve(config.AccessLAN, "192.168.1.8:9000", "", 7681, defaultRanges, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Addr != "192.168.1.8:9000" || r.Port != 9000 {
		t.Fatalf("got %+v", r)
	}
}

func TestUnknownTierRejected(t *testing.T) {
	if _, err := Resolve(config.Access("public"), "", "", 7681, defaultRanges, nil); err == nil {
		t.Fatal("expected error for unknown tier")
	}
}
