// Package listen resolves the bind address from the access tier and reports
// what that address implies for auth.
//
// There is no TLS in this build: Homebase always speaks plain HTTP. The
// TrustedRanges list (see internal/config) is not a security boundary — it is
// only used to decide which warning to show in Settings/README, on the
// user's own assertion that the range in question is already encrypted by
// something else (Tailscale, WireGuard, Headscale, ...). Homebase has no way
// to verify that assertion.
package listen

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/yanghanqing/homebase/internal/config"
)

const lookupTimeout = 2 * time.Second

// LookupIPv4, if non-nil, replaces the default Tailscale probe used as a
// fallback when no local interface address falls inside the trusted ranges.
// Return ("", nil) when unavailable.
type LookupIPv4 func(ctx context.Context) (string, error)

// Result is the resolved listener and what it implies.
type Result struct {
	Addr string // host:port, ready for http.Server
	Host string
	Port int

	Loopback    bool // 127.0.0.1 / ::1
	Trusted     bool // inside one of the configured trusted ranges
	Unspecified bool // 0.0.0.0 / ::

	// ExtraAddrs are additional host:port listeners. private and lan always
	// bind 127.0.0.1 as well as the chosen private addresses, so Settings
	// stays reachable on this machine without binding 0.0.0.0.
	ExtraAddrs []string

	// TierFallback is set when the private tier asked for a trusted-range
	// address and none was found, so the listener dropped to loopback.
	TierFallback bool
}

// BindAddrs is every address serve must listen on, primary first.
func (r Result) BindAddrs() []string {
	out := []string{r.Addr}
	seen := map[string]bool{r.Addr: true}
	for _, a := range r.ExtraAddrs {
		if a != "" && !seen[a] {
			out = append(out, a)
			seen[a] = true
		}
	}
	return out
}

// NeedsAuth reports whether pairing is required. Everything that leaves the
// machine does. Loopback does not: reaching it already implies a local shell,
// so a credential there guards nothing.
func (r Result) NeedsAuth() bool { return !r.Loopback }

// NeedsHostCheck reports whether Host headers must be pinned to loopback names.
// Only the unauthenticated tier needs it, and there it is load-bearing: without
// it any web page could DNS-rebind to 127.0.0.1 and reach the shell. Once a
// session cookie is required, a rebound origin cannot present one.
func (r Result) NeedsHostCheck() bool { return r.Loopback }

// ParseTrustedRanges parses a config.File's TrustedRanges into net.IPNet,
// skipping (not erroring on) anything malformed — SetTrustedRanges already
// validates on write, so this only guards against a hand-edited file.
func ParseTrustedRanges(ranges []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(ranges))
	for _, r := range ranges {
		_, n, err := net.ParseCIDR(strings.TrimSpace(r))
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

// Resolve picks the bind host:port for a tier.
//
//	private → first local address inside trustedRanges, else loopback with TierFallback set
//	lan     → every non-public IPv4 on this machine, plus loopback
//
// Public and unspecified addresses are never bound. An explicit -listen /
// listen_addr may pick a single non-public address but cannot open the
// public internet.
func Resolve(tier config.Access, flagListen, configListen string, port int, trustedRanges []string, lookup LookupIPv4) (Result, error) {
	if !config.ValidAccess(tier) {
		return Result{}, fmt.Errorf("%w: %q", config.ErrAccess, tier)
	}
	if port <= 0 {
		port = config.DefaultPort
	}
	raw := strings.TrimSpace(flagListen)
	if raw == "" {
		raw = strings.TrimSpace(configListen)
	}
	trustedNets := ParseTrustedRanges(trustedRanges)

	var (
		host     string
		extras   []string
		fallback bool
	)
	bindPort := port
	if raw != "" {
		h, p, err := splitListen(raw, port)
		if err != nil {
			return Result{}, err
		}
		host, bindPort = h, p
	} else {
		switch tier {
		case config.AccessLAN:
			ips := listPrivateIPv4s()
			if len(ips) == 0 {
				return Result{}, fmt.Errorf("access is lan but this machine has no private IPv4 address")
			}
			host = ips[0]
			extras = ips[1:]
		case config.AccessPrivate:
			ips := trustedAddrs(trustedNets, lookup)
			if len(ips) == 0 {
				host = "127.0.0.1"
				fallback = true
			} else {
				host = ips[0]
				extras = ips[1:]
			}
		}
	}

	if err := rejectPublicHost(host); err != nil {
		return Result{}, err
	}

	res := classify(host, bindPort, trustedNets)
	res.TierFallback = fallback
	if !res.Loopback {
		loop := joinHostPort("127.0.0.1", bindPort)
		seen := map[string]bool{res.Addr: true, loop: true}
		out := []string{loop}
		for _, ip := range extras {
			if err := rejectPublicHost(ip); err != nil {
				continue
			}
			a := joinHostPort(ip, bindPort)
			if seen[a] {
				continue
			}
			seen[a] = true
			out = append(out, a)
		}
		res.ExtraAddrs = out
	}
	return res, nil
}

func rejectPublicHost(host string) error {
	if host == "" {
		return fmt.Errorf("refusing to bind an unspecified address")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("refusing to bind %q: not an IP address", host)
	}
	if ip.IsLoopback() {
		return nil
	}
	if ip.IsUnspecified() || !config.IPIsNonPublic(ip) {
		return fmt.Errorf("refusing to bind %q: public and unspecified addresses are not allowed", host)
	}
	return nil
}

func classify(host string, port int, trustedNets []*net.IPNet) Result {
	r := Result{Host: host, Port: port, Addr: joinHostPort(host, port)}
	if host == "" {
		r.Unspecified = true
		return r
	}
	if strings.EqualFold(host, "localhost") {
		r.Loopback = true
		return r
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return r
	}
	r.Loopback = ip.IsLoopback()
	r.Unspecified = ip.IsUnspecified()
	r.Trusted = inAny(ip, trustedNets)
	return r
}

func inAny(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// trustedAddrs finds every local IPv4 inside trustedNets. The interface scan
// comes first because it needs no subprocess and works for any overlay
// network (Tailscale, WireGuard, Headscale, ZeroTier, plain interfaces...);
// the Tailscale CLI is a fallback only when no matching interface address
// exists (e.g. userspace networking).
func trustedAddrs(trustedNets []*net.IPNet, lookup LookupIPv4) []string {
	if len(trustedNets) == 0 {
		return nil
	}
	ips := listTrustedIPv4s(trustedNets)
	if len(ips) > 0 {
		return ips
	}
	if lookup == nil {
		lookup = TailscaleIPv4
	}
	ctx, cancel := context.WithTimeout(context.Background(), lookupTimeout)
	defer cancel()
	ip, err := lookup(ctx)
	if err != nil {
		return nil
	}
	ip = strings.TrimSpace(ip)
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() == nil || !unicastIPv4(parsed) || !inAny(parsed, trustedNets) {
		return nil
	}
	return []string{parsed.To4().String()}
}

// listTrustedIPv4s is the interface scanner used by access=private. Tests replace it.
var listTrustedIPv4s = scanTrustedIPv4s

func scanTrustedIPv4s(trustedNets []*net.IPNet) []string {
	var out []string
	seen := map[string]bool{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			v4 := ipnet.IP.To4()
			if v4 == nil || !unicastIPv4(v4) || !inAny(v4, trustedNets) {
				continue
			}
			s := v4.String()
			if seen[s] {
				continue
			}
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// listPrivateIPv4s is the interface scanner used by access=lan. Tests replace it.
var listPrivateIPv4s = scanPrivateIPv4s

// PrivateIPv4s lists this machine's non-public IPv4 addresses (RFC1918, CGNAT,
// link-local). Public addresses are omitted — Homebase will not bind them.
func PrivateIPv4s() []string {
	return scanPrivateIPv4s()
}

func scanPrivateIPv4s() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			v4 := ipnet.IP.To4()
			if v4 == nil || !unicastIPv4(v4) || !config.IPIsNonPublic(v4) {
				continue
			}
			out = append(out, v4.String())
		}
	}
	return out
}

func unicastIPv4(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	if ip4.IsUnspecified() || ip4.IsMulticast() || ip4.IsLoopback() {
		return false
	}
	// 255.255.255.255
	if ip4[0] == 255 && ip4[1] == 255 && ip4[2] == 255 && ip4[3] == 255 {
		return false
	}
	return true
}

func splitListen(s string, defaultPort int) (host string, port int, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0, fmt.Errorf("empty listen address")
	}
	// Bare IPv6 without brackets (has multiple colons, no ']').
	if strings.Count(s, ":") >= 2 && !strings.Contains(s, "]") {
		if net.ParseIP(s) != nil {
			return s, defaultPort, nil
		}
	}
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		// No port: treat the whole string as host.
		if net.ParseIP(s) != nil || !strings.Contains(s, ":") {
			return s, defaultPort, nil
		}
		return "", 0, fmt.Errorf("invalid listen address %q: %w", s, err)
	}
	if portStr == "" {
		return host, defaultPort, nil
	}
	p, err := net.LookupPort("tcp", portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid listen port in %q: %w", s, err)
	}
	return host, p, nil
}

func joinHostPort(host string, port int) string {
	return net.JoinHostPort(host, fmt.Sprintf("%d", port))
}

// TailscaleIPv4 runs `tailscale ip -4`.
func TailscaleIPv4(ctx context.Context) (string, error) {
	bin, err := findTailscale()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, bin, "ip", "-4")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return parseTailscaleIPv4(string(out))
}

// tailscaleBins covers Homebrew, the standalone app, and the Mac App Store
// build, which ships its CLI inside the bundle and never on PATH.
var tailscaleBins = []string{
	"/opt/homebrew/bin/tailscale",
	"/usr/local/bin/tailscale",
	"/Applications/Tailscale.app/Contents/MacOS/Tailscale",
}

func findTailscale() (string, error) {
	if p, err := exec.LookPath("tailscale"); err == nil {
		return p, nil
	}
	for _, p := range tailscaleBins {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("tailscale not found")
}

func parseTailscaleIPv4(out string) (string, error) {
	line := strings.TrimSpace(strings.SplitN(out, "\n", 2)[0])
	ip := net.ParseIP(line)
	if ip == nil || ip.To4() == nil {
		return "", fmt.Errorf("tailscale ip -4 did not print an IPv4 address")
	}
	if !unicastIPv4(ip) {
		return "", fmt.Errorf("tailscale ip -4 is not a unicast IPv4 address")
	}
	return ip.To4().String(), nil
}
