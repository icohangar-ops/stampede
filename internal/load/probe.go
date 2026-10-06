package load

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/icohangar-ops/stampede/internal/safety"
)

// Asset is one same-origin resource discovered on the page.
type Asset struct {
	URL         string  `json:"url"`
	MS          float64 `json:"ms"`
	Bytes       int     `json:"bytes"`
	ContentType string  `json:"content_type"`
	Status      int     `json:"status"`
}

// ProbeResult is a single quiet pass over the page and a few assets.
// It is not part of the requests-per-second series.
type ProbeResult struct {
	PageMS     float64 `json:"page_ms"`
	PageBytes  int     `json:"page_bytes"`
	PageStatus int     `json:"page_status"`
	Assets     []Asset `json:"assets"`
	Error      string  `json:"error,omitempty"`
}

var attrRe = regexp.MustCompile(`(?i)\b(?:src|href)\s*=\s*["']([^"']+)["']`)

// Probe fetches the page once and up to four same-origin assets.
func Probe(ctx context.Context, client *http.Client, pageURL string) ProbeResult {
	var out ProbeResult
	if client == nil {
		out.Error = "no client"
		return out
	}
	body, status, ms, err := get(ctx, client, pageURL)
	out.PageMS = ms
	out.PageStatus = status
	out.PageBytes = len(body)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	seen := map[string]struct{}{base.Path: {}}
	var urls []string
	for _, m := range attrRe.FindAllSubmatch(body, 20) {
		ref := strings.TrimSpace(string(m[1]))
		if ref == "" || strings.HasPrefix(ref, "data:") || strings.HasPrefix(ref, "mailto:") || strings.HasPrefix(ref, "#") {
			continue
		}
		u, err := base.Parse(ref)
		if err != nil || u.Hostname() != base.Hostname() {
			continue
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			continue
		}
		if _, ok := seen[u.Path]; ok {
			continue
		}
		seen[u.Path] = struct{}{}
		u.Fragment = ""
		urls = append(urls, u.String())
		if len(urls) == 4 {
			break
		}
	}
	for _, u := range urls {
		b, st, ms, err := get(ctx, client, u)
		asset := Asset{URL: u, MS: ms, Bytes: len(b), Status: st}
		if err == nil {
			// Content type is not returned by get. Infer from the path.
			asset.ContentType = contentGuess(u)
		}
		out.Assets = append(out.Assets, asset)
	}
	return out
}

func contentGuess(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	p := strings.ToLower(u.Path)
	switch {
	case strings.HasSuffix(p, ".png"):
		return "image/png"
	case strings.HasSuffix(p, ".jpg"), strings.HasSuffix(p, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(p, ".webp"):
		return "image/webp"
	case strings.HasSuffix(p, ".gif"):
		return "image/gif"
	case strings.HasSuffix(p, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(p, ".css"):
		return "text/css"
	case strings.HasSuffix(p, ".js"):
		return "text/javascript"
	default:
		return ""
	}
}

func get(ctx context.Context, client *http.Client, rawURL string) ([]byte, int, float64, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, 0, err
	}
	req.Header.Set("User-Agent", safety.UserAgent)
	start := time.Now()
	resp, err := client.Do(req)
	ms := float64(time.Since(start).Microseconds()) / 1000
	if err != nil {
		return nil, 0, ms, err
	}
	defer resp.Body.Close()
	buf, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return buf, resp.StatusCode, ms, err
}
