package main

import (
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/icohangar-ops/stampede/internal/gcpauth"
)

func main() {
	orch := env("ORCHESTRATOR_URL", "http://127.0.0.1:8081")
	staticDir := env("STATIC_DIR", "web/dist")
	audience := os.Getenv("ORCHESTRATOR_AUDIENCE")
	trust := os.Getenv("TRUST_PROXY") == "1"
	target, err := url.Parse(orch)
	if err != nil {
		log.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = -1
	baseDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		ip := clientIP(r, trust)
		incoming := r.Header.Get("Authorization")
		baseDirector(r)
		r.Host = target.Host
		r.Header.Set("X-Forwarded-For", ip)
		if audience != "" {
			if tok, err := gcpauth.Identity(r.Context(), audience); err == nil && tok != "" {
				if strings.HasPrefix(strings.ToLower(incoming), "bearer ") {
					r.Header.Set("X-Stampede-Admin", strings.TrimSpace(incoming[7:]))
				}
				r.Header.Set("Authorization", "Bearer "+tok)
			}
		}
	}
	mux := http.NewServeMux()
	mux.Handle("/v1/", proxy)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/", spa(staticDir))
	addr := ":" + env("PORT", "8080")
	log.Printf("web listening on %s -> %s", addr, orch)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func spa(dir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rel := strings.TrimPrefix(filepath.Clean("/"+r.URL.Path), "/")
		full := filepath.Join(dir, rel)
		if rel != "" && !strings.HasPrefix(full, filepath.Clean(dir)+string(os.PathSeparator)) && full != filepath.Clean(dir) {
			http.NotFound(w, r)
			return
		}
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		http.ServeFile(w, r, full)
	})
}

func clientIP(r *http.Request, trust bool) string {
	if trust {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
