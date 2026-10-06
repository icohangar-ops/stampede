// Package demotarget is the first-party site the local demo is allowed to hit.
// It cooperates with ownership checks because we own it, and it falls over
// on purpose once the ramp gets serious so the chart has a breaking point.
package demotarget

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Handler is the demo site.
func Handler() http.Handler {
	hero := makeHero()
	var gate hitGate
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	})
	mux.HandleFunc("GET /.well-known/", func(w http.ResponseWriter, r *http.Request) {
		const prefix = "/.well-known/stampede-"
		if !strings.HasPrefix(r.URL.Path, prefix) || !strings.HasSuffix(r.URL.Path, ".txt") {
			http.NotFound(w, r)
			return
		}
		token := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), ".txt")
		if len(token) < 6 || len(token) > 80 || strings.ContainsAny(token, "/\\") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, token)
	})
	mux.HandleFunc("GET /hero.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(hero)
	})
	mux.HandleFunc("GET /api/pricing", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(900 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"plan":"launch","amount":49}`)
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		delay, fail := gate.hit()
		time.Sleep(delay)
		if fail {
			http.Error(w, "overloaded", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Northwind Kits</title></head>
<body>
  <h1>Northwind Kits</h1>
  <p>A small shop, about to meet the front page.</p>
  <img src="/hero.png" alt="Kit on a bench" width="480" height="270">
  <p><a href="/api/pricing">Pricing</a></p>
</body></html>`)
	})
	return mux
}

type hitGate struct {
	mu   sync.Mutex
	hits []time.Time
}

func (g *hitGate) hit() (time.Duration, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	cut := now.Add(-time.Second)
	kept := g.hits[:0]
	for _, t := range g.hits {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	g.hits = append(kept, now)
	n := len(g.hits)
	switch {
	case n >= 34:
		return 30 * time.Millisecond, true
	case n >= 24:
		return 1600 * time.Millisecond, false
	case n >= 14:
		return 220 * time.Millisecond, false
	default:
		return 20 * time.Millisecond, false
	}
}

func makeHero() []byte {
	const w, h = 480, 270
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rng := rand.New(rand.NewSource(7))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(180 + rng.Intn(70)),
				G: uint8(40 + rng.Intn(50)),
				B: uint8(rng.Intn(40)),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
