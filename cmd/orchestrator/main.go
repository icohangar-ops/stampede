package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/icohangar-ops/stampede/internal/api"
	"github.com/icohangar-ops/stampede/internal/blob"
	"github.com/icohangar-ops/stampede/internal/gcpauth"
	"github.com/icohangar-ops/stampede/internal/launch"
	"github.com/icohangar-ops/stampede/internal/report"
	"github.com/icohangar-ops/stampede/internal/safety"
	"github.com/icohangar-ops/stampede/internal/store"
	"github.com/icohangar-ops/stampede/internal/store/firestore"
	"github.com/icohangar-ops/stampede/internal/store/memory"
	"github.com/icohangar-ops/stampede/internal/store/sqlite"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("orchestrator ")
	policy := safety.DefaultPolicy()
	if v := os.Getenv("MAX_RPS"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			policy.MaxRPS = n
		}
	}
	if v := os.Getenv("MAX_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			policy.MaxDuration = d
		}
	}
	if policy.MaxDuration > 3*time.Minute {
		policy.MaxDuration = 3 * time.Minute
	}
	if v := os.Getenv("MAX_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			policy.MaxWorkers = n
		}
	}
	policy.AllowHosts = safety.ParseAllow(os.Getenv("ALLOW_HOSTS"))

	ctx := context.Background()
	var st store.Store
	switch os.Getenv("STORE") {
	case "memory":
		st = memory.New()
	case "firestore":
		fs, err := firestore.Open(ctx, env("FIRESTORE_PROJECT", os.Getenv("GOOGLE_CLOUD_PROJECT")))
		if err != nil {
			log.Fatal(err)
		}
		defer fs.Close()
		st = fs
	default:
		path := env("DATABASE_PATH", "data/stampede.db")
		sq, err := sqlite.Open(path)
		if err != nil {
			log.Fatal(err)
		}
		defer sq.Close()
		st = sq
	}

	var blobs blob.Store = blob.Local{Dir: env("ARTIFACT_DIR", "data/artifacts")}
	if bucket := os.Getenv("GCS_BUCKET"); bucket != "" {
		g, err := blob.OpenGCS(ctx, bucket)
		if err != nil {
			log.Fatal(err)
		}
		defer g.Close()
		blobs = g
	}

	var llm report.LLM
	modelName := "template"
	if project := os.Getenv("VERTEX_PROJECT"); project != "" && os.Getenv("REPORT_URL") == "" {
		llm = report.Vertex{
			Project:  project,
			Location: env("VERTEX_LOCATION", "us-central1"),
			Model:    env("VERTEX_MODEL", "gemini-2.5-flash"),
		}
		modelName = env("VERTEX_MODEL", "gemini-2.5-flash")
	}

	mode := env("LOADGEN_MODE", "inprocess")
	var launcher launch.Launcher
	if mode == "cloudrun" {
		// jobs.run wants a cloud-platform access token, not an identity token.
		launcher = launch.CloudRun{
			Project: env("CLOUD_RUN_PROJECT", os.Getenv("GOOGLE_CLOUD_PROJECT")),
			Region:  os.Getenv("CLOUD_RUN_REGION"),
			Job:     env("CLOUD_RUN_JOB", "stampede-loadgen"),
			TasksN:  10,
			Token:   gcpauth.AccessToken,
		}
	}

	domainQuota := intEnv("DOMAIN_DAILY_QUOTA", 3)
	ipQuota := intEnv("IP_DAILY_QUOTA", 5)
	srv := api.New(&api.Server{
		Store:          st,
		Policy:         policy,
		Reports:        report.Builder{LLM: llm},
		Blobs:          blobs,
		AdminToken:     os.Getenv("ADMIN_TOKEN"),
		InternalToken:  os.Getenv("INTERNAL_TOKEN"),
		DemoPublic:     os.Getenv("DEMO_URL_PUBLIC"),
		DemoInternal:   os.Getenv("DEMO_URL_INTERNAL"),
		KillFile:       os.Getenv("KILL_FILE"),
		EnvKill:        os.Getenv("KILL_SWITCH") == "1" || os.Getenv("KILL_SWITCH") == "true",
		DomainQuota:    domainQuota,
		IPQuota:        ipQuota,
		Mode:           mode,
		Launcher:       launcher,
		TrustProxy:     os.Getenv("TRUST_PROXY") == "1",
		ModelName:      modelName,
		ReportURL:      os.Getenv("REPORT_URL"),
		ReportToken:    os.Getenv("REPORT_TOKEN"),
		ReportAudience: os.Getenv("REPORT_AUDIENCE"),
	})
	addr := ":" + env("PORT", "8080")
	log.Printf("listening on %s store=%s mode=%s", addr, env("STORE", "sqlite"), mode)
	log.Fatal(srv.Listen(addr))
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
