package verify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/icohangar-ops/stampede/internal/safety"
)

func TestFileMatchAndMismatch(t *testing.T) {
	token := "st_demo_token_value"
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/stampede-"+token+".txt", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method %s", r.Method)
		}
		_, _ = w.Write([]byte("  " + token + "\n"))
	})
	mux.HandleFunc("/.well-known/stampede-wrong.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("nope"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := safety.DefaultPolicy()
	p.AllowHosts = safety.ParseAllow("127.0.0.1")
	client := p.HTTPClient()
	page, _ := url.Parse(srv.URL + "/pricing")
	file := FileURL(page, token)
	if file.Path != "/.well-known/stampede-"+token+".txt" {
		t.Fatal(file)
	}
	ctx := context.Background()
	if err := CheckFile(ctx, client, file.String(), token); err != nil {
		t.Fatal(err)
	}
	bad := FileURL(page, "wrong")
	if err := CheckFile(ctx, client, bad.String(), "wrong"); err == nil {
		t.Fatal("expected mismatch")
	}

	// A private host that is not allowlisted never gets a connection.
	blocked := safety.DefaultPolicy().HTTPClient()
	if err := CheckFile(ctx, blocked, file.String(), token); err == nil {
		t.Fatal("expected SSRF block")
	}
}

func TestDNS(t *testing.T) {
	token := "st_dns"
	ctx := context.Background()
	lookup := func(_ context.Context, name string) ([]string, error) {
		if name != "_stampede.example.com" {
			t.Errorf("name %s", name)
		}
		return []string{"v=spf1", DNSValue(token)}, nil
	}
	if err := CheckDNS(ctx, lookup, "example.com", token); err != nil {
		t.Fatal(err)
	}
	if err := CheckDNS(ctx, func(context.Context, string) ([]string, error) {
		return []string{"other"}, nil
	}, "example.com", token); err == nil {
		t.Fatal("expected mismatch")
	}
}

func TestExpiryConstant(t *testing.T) {
	if ErrExpired == nil {
		t.Fatal()
	}
	_ = time.Minute
}
