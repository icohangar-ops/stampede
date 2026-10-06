// Package verify checks that the caller controls a host before any load is sent.
package verify

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxBody = 256

var (
	ErrMismatch = errors.New("ownership token did not match")
	ErrExpired  = errors.New("verification challenge expired")
)

// FileURL is the well-known URL the owner must serve.
func FileURL(page *url.URL, token string) *url.URL {
	u := *page
	u.Path = "/.well-known/stampede-" + token + ".txt"
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return &u
}

// DNSName is the TXT owner name.
func DNSName(host string) string { return "_stampede." + host }

// DNSValue is the exact TXT value.
func DNSValue(token string) string { return "stampede-verify=" + token }

// CheckFile GETs the well-known URL and requires the body to equal the token.
func CheckFile(ctx context.Context, client *http.Client, fileURL, token string) error {
	if client == nil {
		return errors.New("http client is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("User-Agent", "StampedeBot/1.0 (ownership check)")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	buf, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return err
	}
	if len(buf) > maxBody {
		return ErrMismatch
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrMismatch, resp.StatusCode)
	}
	got := strings.TrimSpace(string(buf))
	if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
		return ErrMismatch
	}
	return nil
}

// TXTLookup resolves TXT records. Tests inject this.
type TXTLookup func(ctx context.Context, name string) ([]string, error)

// DefaultTXT uses the system resolver.
func DefaultTXT(ctx context.Context, name string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return (&net.Resolver{}).LookupTXT(ctx, name)
}

// CheckDNS requires a TXT record equal to the issued value.
func CheckDNS(ctx context.Context, lookup TXTLookup, host, token string) error {
	if lookup == nil {
		lookup = DefaultTXT
	}
	records, err := lookup(ctx, DNSName(host))
	if err != nil {
		return err
	}
	want := DNSValue(token)
	for _, rec := range records {
		if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(rec)), []byte(want)) == 1 {
			return nil
		}
	}
	return ErrMismatch
}
