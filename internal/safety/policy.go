// Package safety is the gate in front of every outbound connection.
// Caps, URL shape, and address class are enforced here so a caller
// cannot turn Stampede into a flood or an SSRF client.
package safety

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultMaxRPS      = 40
	DefaultMaxWorkers  = 50
	DefaultMaxDuration = 180 * time.Second
	UserAgent          = "StampedeBot/1.0 (launch check; ownership-verified GET only)"
)

var (
	ErrScheme     = errors.New("only http and https URLs are allowed")
	ErrUserinfo   = errors.New("URLs with userinfo are blocked")
	ErrPrivate    = errors.New("private or internal address blocked")
	ErrMetadata   = errors.New("cloud metadata address blocked")
	ErrPort       = errors.New("port is not allowed")
	ErrHost       = errors.New("host is not allowed")
	ErrObfuscated = errors.New("obfuscated IP host is blocked")
	ErrCap        = errors.New("requested load exceeds the safety cap")
	ErrRedirect   = errors.New("redirect blocked")
)

// Resolver looks up a hostname. Tests inject a fake.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// Policy is the operator-controlled safety envelope.
type Policy struct {
	AllowHosts   map[string]struct{}
	MaxRPS       float64
	MaxDuration  time.Duration
	MaxWorkers   int
	MaxRedirects int
	Resolver     Resolver
}

func DefaultPolicy() Policy {
	return Policy{
		AllowHosts:   map[string]struct{}{},
		MaxRPS:       DefaultMaxRPS,
		MaxDuration:  DefaultMaxDuration,
		MaxWorkers:   DefaultMaxWorkers,
		MaxRedirects: 2,
	}
}

// ParseAllow splits a comma-separated hostname allowlist.
func ParseAllow(list string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, p := range strings.Split(list, ",") {
		p = normalizeHost(p)
		if p != "" {
			m[p] = struct{}{}
		}
	}
	return m
}

func (p Policy) allowed(host string) bool {
	_, ok := p.AllowHosts[normalizeHost(host)]
	return ok
}

// Target is a normalized URL that passed the static checks.
// DNS and the dial still happen later, on every connection.
type Target struct {
	URL  *url.URL
	Host string
	Port int
}

// Normalize checks scheme, userinfo, port, and literal addresses.
// It does not resolve DNS. Call Resolve or use the HTTP client for that.
func (p Policy) Normalize(raw string) (Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return Target{}, ErrHost
	}
	if strings.ContainsAny(raw, "\\\n\r") {
		return Target{}, ErrHost
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Target{}, fmt.Errorf("%w: %s", ErrHost, err.Error())
	}
	if u.User != nil {
		return Target{}, ErrUserinfo
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return Target{}, ErrScheme
	}
	host := normalizeHost(u.Hostname())
	if host == "" {
		return Target{}, ErrHost
	}
	if isMetadataHost(host) {
		return Target{}, ErrMetadata
	}
	if isObfuscatedHost(host) {
		return Target{}, ErrObfuscated
	}
	port := 80
	if scheme == "https" {
		port = 443
	}
	if ps := u.Port(); ps != "" {
		n, err := strconv.Atoi(ps)
		if err != nil || n < 1 || n > 65535 {
			return Target{}, ErrPort
		}
		port = n
	}
	allow := p.allowed(host)
	if ip := net.ParseIP(host); ip != nil {
		if err := p.checkIP(ip, allow); err != nil {
			return Target{}, err
		}
	} else if !strings.Contains(host, ".") && !allow {
		return Target{}, ErrHost
	}
	if !allowedPort(port, allow) {
		return Target{}, ErrPort
	}
	u.Scheme = scheme
	u.Host = host
	if !((scheme == "http" && port == 80) || (scheme == "https" && port == 443)) {
		u.Host = net.JoinHostPort(host, strconv.Itoa(port))
	}
	u.Fragment = ""
	u.RawFragment = ""
	return Target{URL: u, Host: host, Port: port}, nil
}

// Resolve returns a single IP that passed the policy. Every address
// returned by DNS is checked. One private answer rejects the host.
func (p Policy) Resolve(ctx context.Context, host string) (net.IP, error) {
	host = normalizeHost(host)
	if isMetadataHost(host) {
		return nil, ErrMetadata
	}
	allow := p.allowed(host)
	if ip := net.ParseIP(host); ip != nil {
		if err := p.checkIP(ip, allow); err != nil {
			return nil, err
		}
		return ip, nil
	}
	r := p.Resolver
	var addrs []net.IPAddr
	var err error
	if r == nil {
		addrs, err = net.DefaultResolver.LookupIPAddr(ctx, host)
	} else {
		addrs, err = r.LookupIPAddr(ctx, host)
	}
	if err != nil {
		return nil, fmt.Errorf("dns: %w", err)
	}
	if len(addrs) == 0 {
		return nil, ErrHost
	}
	var picked net.IP
	for _, a := range addrs {
		if err := p.checkIP(a.IP, allow); err != nil {
			return nil, err
		}
		if a.IP.To4() != nil {
			picked = a.IP
		} else if picked == nil {
			picked = a.IP
		}
	}
	if picked == nil {
		return nil, ErrPrivate
	}
	return picked, nil
}

// DialContext pins the connection to an address that passed Resolve.
func (p Policy) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ip, err := p.Resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	d := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 15 * time.Second}
	return d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
}

// HTTPClient is the only client verification and load generation should use.
// It ignores HTTP_PROXY so an environment proxy cannot bypass the dialer.
func (p Policy) HTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           p.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       15 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Timeout:   8 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > p.MaxRedirects {
				return fmt.Errorf("%w: too many redirects", ErrRedirect)
			}
			if len(via) == 0 || !sameSite(req.URL, via[0].URL) {
				return fmt.Errorf("%w: cross-host redirect", ErrRedirect)
			}
			if _, err := p.Normalize(req.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
}

// ValidatePlan rejects a plan that would exceed the operator caps.
func (p Policy) ValidatePlan(rps float64, d time.Duration, workers int) error {
	if rps <= 0 || d < time.Second || workers <= 0 {
		return ErrCap
	}
	if rps > p.MaxRPS || d > p.MaxDuration || workers > p.MaxWorkers {
		return ErrCap
	}
	if d > 3*time.Minute {
		return ErrCap
	}
	return nil
}

func (p Policy) checkIP(ip net.IP, allow bool) error {
	if ip == nil {
		return ErrPrivate
	}
	if isMetadataIP(ip) {
		return ErrMetadata
	}
	if !allow && isBlockedIP(ip) {
		return ErrPrivate
	}
	return nil
}

func normalizeHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	h = strings.TrimSuffix(h, ".")
	return h
}

func sameSite(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return normalizeHost(a.Hostname()) == normalizeHost(b.Hostname())
}

func allowedPort(port int, allowlisted bool) bool {
	if port < 1 || port > 65535 {
		return false
	}
	// Operator-allowlisted demo hosts may use any port. Everyone else is 80/443.
	if allowlisted {
		return true
	}
	return port == 80 || port == 443
}

func isMetadataHost(h string) bool {
	switch normalizeHost(h) {
	case "metadata.google.internal", "metadata.goog", "metadata":
		return true
	default:
		return false
	}
}

func isMetadataIP(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	return v4[0] == 169 && v4[1] == 254 && v4[2] == 169 && (v4[3] == 254 || v4[3] == 253)
}

func isObfuscatedHost(h string) bool {
	if h == "" {
		return false
	}
	for _, c := range h {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	// Check IPv4-mapped IPv6 as the underlying v4 address.
	if ip.To4() != nil && len(ip) == net.IPv6len {
		return isBlockedIP(ip.To4())
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	for _, n := range extraBlocked {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

var extraBlocked = mustCIDRs([]string{
	"0.0.0.0/8",
	"100.64.0.0/10",   // CGNAT
	"192.0.0.0/24",    // IETF protocol assignments
	"192.0.2.0/24",    // TEST-NET-1
	"198.18.0.0/15",   // benchmarking
	"198.51.100.0/24", // TEST-NET-2
	"203.0.113.0/24",  // TEST-NET-3
	"224.0.0.0/4",     // multicast
	"240.0.0.0/4",     // reserved
	"2001:db8::/32",   // documentation
})

func mustCIDRs(in []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(in))
	for _, s := range in {
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			panic(err)
		}
		out = append(out, n)
	}
	return out
}
