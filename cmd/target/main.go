package main

import (
	"log"
	"net/http"
	"os"

	"github.com/icohangar-ops/stampede/internal/demotarget"
)

func main() {
	addr := ":" + env("PORT", "8090")
	log.Printf("demo target listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, demotarget.Handler()))
}

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
