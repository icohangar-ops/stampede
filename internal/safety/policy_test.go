package safety

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNormalizeBlocksDangerousURLs(t *testing.T) {
	p := DefaultPolicy()
	cases := []struct {
		raw string
		err error
	}{
		{"https://example.com/pricing", nil},
		{"http://example.com", nil},
		{"file:///etc/passwd", ErrScheme},
		{"gopher://example.com", ErrScheme},
		{"http://user:pass@example.com/", ErrUserinfo},
		{"http://example.com@127.0.0.1/", ErrUserinfo},
		{"http://2130706433/", ErrObfuscated},
		{"http://127.0.0.1/", ErrPrivate},
		{"http://127.0.0.1:8090/", ErrPrivate},
		{"http://10.1.2.3/", ErrPrivate},
		{"http://192.168.1.20/admin", ErrPrivate},
		{"http://172.16.0.4/", ErrPrivate},
		{"http://100.64.0.5/", ErrPrivate},
		{"http://169.254.169.254/", ErrMetadata},
		{"http://169.254.169.253/computeMetadata/v1/", ErrMetadata},
		{"http://[::1]/", ErrPrivate},
		{"http://[fd00::1]/", ErrPrivate},
		{"http://[fe80::1]/", ErrPrivate},
		{"http://metadata.google.internal/", ErrMetadata},
		{"http://metadata.goog/", ErrMetadata},
		{"http://localhost/", ErrHost},
		{"http://example.com:22/", ErrPort},
		{"http://example.com:8080/", ErrPort},
		{"http://192.0.2.10/", ErrPrivate},
		{"http://198.51.100.20/", ErrPrivate},
		{"http://203.0.113.5/", ErrPrivate},
		{"ftp://example.com", ErrScheme},
		{"https://example.com/a\\b", ErrHost},
	}
	for _, tc := range cases {
		_, err := p.Normalize(tc.raw)
		if tc.err == nil {
			if err != nil {
				t.Errorf("%s: unexpected error %v", tc.raw, err)
			}
			continue
		}
		if err == nil || !errorsIs(err, tc.err) {
			t.Errorf("%s: got %v, want %v", tc.raw, err, tc.err)
		}
	}
}

func TestAllowlistPermitsDemoHostButNotMetadata(t *testing.T) {
	p := DefaultPolicy()
	p.AllowHosts = ParseAllow("localhost, target, metadata.google.internal")
	if _, err := p.Normalize("http://localhost:8090/"); err != nil {
		t.Fatalf("localhost demo: %v", err)
	}
	if _, err := p.Normalize("http://target:8090/healthz"); err != nil {
		t.Fatalf("target demo: %v", err)
	}
	if _, err := p.Normalize("http://metadata.google.internal/"); !errorsIs(err, ErrMetadata) {
		t.Fatalf("metadata host on allowlist: got %v", err)
	}
	if _, err := p.Normalize("http://169.254.169.254/"); !errorsIs(err, ErrMetadata) {
		t.Fatalf("metadata ip on allowlist: got %v", err)
	}
	// Public custom ports stay closed. Allowlisted hosts may use the demo ports.
	if _, err := p.Normalize("http://example.com:8090/"); !errorsIs(err, ErrPort) {
		t.Fatalf("public high port: got %v", err)
	}
}

func TestResolveRejectsPrivateAnswers(t *testing.T) {
	p := DefaultPolicy()
	p.Resolver = mapResolver{
		"evil.test.example":      []net.IP{net.ParseIP("10.0.0.5")},
		"mixed.test.example":     []net.IP{net.ParseIP("1.1.1.1"), net.ParseIP("192.168.0.9")},
		"meta.test.example":      []net.IP{net.ParseIP("169.254.169.254")},
		"linklocal.test.example": []net.IP{net.ParseIP("169.254.1.1")},
		"good.test.example":      []net.IP{net.ParseIP("1.1.1.1")},
		"mapped.test.example":    []net.IP{net.ParseIP("::ffff:127.0.0.1")},
		"cgnat.test.example":     []net.IP{net.ParseIP("100.100.1.1")},
	}
	ctx := context.Background()
	if _, err := p.Resolve(ctx, "evil.test.example"); !errorsIs(err, ErrPrivate) {
		t.Fatalf("evil: %v", err)
	}
	if _, err := p.Resolve(ctx, "mixed.test.example"); !errorsIs(err, ErrPrivate) {
		t.Fatalf("mixed: %v", err)
	}
	if _, err := p.Resolve(ctx, "meta.test.example"); !errorsIs(err, ErrMetadata) {
		t.Fatalf("meta: %v", err)
	}
	if _, err := p.Resolve(ctx, "linklocal.test.example"); !errorsIs(err, ErrPrivate) {
		t.Fatalf("link local: %v", err)
	}
	if _, err := p.Resolve(ctx, "mapped.test.example"); !errorsIs(err, ErrPrivate) {
		t.Fatalf("mapped loopback: %v", err)
	}
	if _, err := p.Resolve(ctx, "cgnat.test.example"); !errorsIs(err, ErrPrivate) {
		t.Fatalf("cgnat: %v", err)
	}
	ip, err := p.Resolve(ctx, "good.test.example")
	if err != nil || !ip.Equal(net.ParseIP("1.1.1.1")) {
		t.Fatalf("good: %v %v", ip, err)
	}

	p.AllowHosts = ParseAllow("meta.test.example, evil.test.example")
	if _, err := p.Resolve(ctx, "meta.test.example"); !errorsIs(err, ErrMetadata) {
		t.Fatalf("allowlisted metadata ip: %v", err)
	}
	if _, err := p.Resolve(ctx, "evil.test.example"); err != nil {
		t.Fatalf("allowlisted private demo host should pass: %v", err)
	}
}

func TestDialBlocksLoopbackUnlessAllowlisted(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan struct{}, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			accepted <- struct{}{}
			c.Close()
		}
	}()

	p := DefaultPolicy()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := p.DialContext(ctx, "tcp", ln.Addr().String()); !errorsIs(err, ErrPrivate) {
		t.Fatalf("dial without allowlist: %v", err)
	}
	select {
	case <-accepted:
		t.Fatal("connection was accepted despite the block")
	default:
	}

	p.AllowHosts = ParseAllow("127.0.0.1")
	conn, err := p.DialContext(ctx, "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("allowlisted dial: %v", err)
	}
	conn.Close()
}

func TestRedirectToMetadataIsBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/computeMetadata/v1/", http.StatusFound)
	}))
	defer srv.Close()

	p := DefaultPolicy()
	p.AllowHosts = ParseAllow("127.0.0.1")
	client := p.HTTPClient()
	resp, err := client.Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected redirect to metadata to fail")
	}
	if !errorsIs(err, ErrMetadata) && !errorsIs(err, ErrRedirect) && !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("redirect error: %v", err)
	}
}

func TestClientDoesNotFollowCrossHostRedirect(t *testing.T) {
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com/secret", http.StatusFound)
	}))
	defer src.Close()

	p := DefaultPolicy()
	p.AllowHosts = ParseAllow("127.0.0.1")
	resp, err := p.HTTPClient().Get(src.URL)
	if err == nil {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("followed a cross-host redirect: %s", body)
	}
	if resp != nil && resp.Body != nil {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(body), "secret") {
			t.Fatal("followed a cross-host redirect")
		}
	}
	if !errorsIs(err, ErrRedirect) && !strings.Contains(err.Error(), "cross-host") {
		t.Fatalf("redirect error: %v", err)
	}
}

func TestValidatePlanCaps(t *testing.T) {
	p := DefaultPolicy()
	if err := p.ValidatePlan(40, 3*time.Minute, 50); err != nil {
		t.Fatal(err)
	}
	if err := p.ValidatePlan(41, time.Minute, 10); !errorsIs(err, ErrCap) {
		t.Fatalf("rps: %v", err)
	}
	if err := p.ValidatePlan(10, 3*time.Minute+time.Second, 10); !errorsIs(err, ErrCap) {
		t.Fatalf("duration: %v", err)
	}
	if err := p.ValidatePlan(10, time.Minute, 51); !errorsIs(err, ErrCap) {
		t.Fatalf("workers: %v", err)
	}
	if err := p.ValidatePlan(0, time.Minute, 10); !errorsIs(err, ErrCap) {
		t.Fatalf("zero rps: %v", err)
	}
}

type mapResolver map[string][]net.IP

func (m mapResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	ips, ok := m[normalizeHost(host)]
	if !ok {
		return nil, ErrHost
	}
	out := make([]net.IPAddr, len(ips))
	for i, ip := range ips {
		out[i] = net.IPAddr{IP: ip}
	}
	return out, nil
}

func errorsIs(err, target error) bool {
	return err == target || (err != nil && target != nil && strings.Contains(err.Error(), target.Error()))
}
