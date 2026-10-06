package main

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/icohangar-ops/stampede/internal/report"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("report ")
	token := os.Getenv("REPORT_TOKEN")
	var llm report.LLM
	model := "template"
	if project := os.Getenv("VERTEX_PROJECT"); project != "" {
		llm = report.Vertex{
			Project:  project,
			Location: env("VERTEX_LOCATION", "us-central1"),
			Model:    env("VERTEX_MODEL", "gemini-2.5-flash"),
		}
		model = env("VERTEX_MODEL", "gemini-2.5-flash")
	}
	builder := report.Builder{LLM: llm}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("POST /v1/summarize", func(w http.ResponseWriter, r *http.Request) {
		if !match(r.Header.Get("X-Stampede-Token"), token) {
			http.Error(w, `{"error":"report token required"}`, http.StatusUnauthorized)
			return
		}
		var in report.Input
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		rep := builder.Build(r.Context(), in)
		if rep.Model == "" {
			rep.Model = model
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rep)
	})
	addr := ":" + env("PORT", "8080")
	log.Printf("listening on %s model=%s", addr, model)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func match(got, want string) bool {
	if want == "" || len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// keep strings import honest if a future trim is added
var _ = strings.TrimSpace
