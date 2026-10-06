// Package gcpauth fetches Cloud Run identity tokens from the metadata server.
// Local processes get an empty token and should rely on the shared secret instead.
package gcpauth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/oauth2/google"
)

// AccessToken returns an application-default cloud-platform token.
func AccessToken(ctx context.Context) (string, error) {
	ts, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", err
	}
	tok, err := ts.Token()
	if err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

// Identity returns a cached ID token for audience, or an error off Cloud Run.
func Identity(ctx context.Context, audience string) (string, error) {
	if audience == "" {
		return "", fmt.Errorf("audience is required")
	}
	if tok, ok := cache.get(audience); ok {
		return tok, nil
	}
	u := "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity?audience=" + url.QueryEscape(audience) + "&format=full"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata status %d", resp.StatusCode)
	}
	tok := string(body)
	cache.set(audience, tok, 30*time.Minute)
	return tok, nil
}

type tokenCache struct {
	mu    sync.Mutex
	items map[string]item
}

type item struct {
	tok string
	exp time.Time
}

func (c *tokenCache) get(aud string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.items[aud]
	if !ok || time.Now().After(it.exp) {
		return "", false
	}
	return it.tok, true
}

func (c *tokenCache) set(aud, tok string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil {
		c.items = map[string]item{}
	}
	c.items[aud] = item{tok: tok, exp: time.Now().Add(ttl)}
}

var cache tokenCache
