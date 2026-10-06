package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/icohangar-ops/stampede/internal/gcpauth"
	"github.com/icohangar-ops/stampede/internal/load"
	"github.com/icohangar-ops/stampede/internal/preset"
	"github.com/icohangar-ops/stampede/internal/safety"
	"github.com/icohangar-ops/stampede/internal/store"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("loadgen ")
	base := os.Getenv("ORCHESTRATOR_URL")
	runID := os.Getenv("RUN_ID")
	token := os.Getenv("INTERNAL_TOKEN")
	if base == "" || runID == "" || token == "" {
		log.Fatal("ORCHESTRATOR_URL, RUN_ID, and INTERNAL_TOKEN are required")
	}
	index := atoi(os.Getenv("CLOUD_RUN_TASK_INDEX"))
	count := atoi(os.Getenv("CLOUD_RUN_TASK_COUNT"))
	if count < 1 {
		count = 1
	}
	ctx := context.Background()
	plan := fetchPlan(ctx, base, runID, token)
	if plan.Cancel {
		log.Printf("run %s already cancelled", runID)
		return
	}
	p, ok := preset.ByID(plan.Preset)
	if !ok {
		log.Fatalf("unknown preset %s", plan.Preset)
	}
	policy := safety.DefaultPolicy()
	if plan.MaxRPS > 0 && plan.MaxRPS < policy.MaxRPS {
		policy.MaxRPS = plan.MaxRPS
	}
	policy.AllowHosts = safety.ParseAllow(os.Getenv("ALLOW_HOSTS"))
	client := policy.HTTPClient()
	var probe *load.ProbeResult
	if index == 0 {
		got := load.Probe(ctx, client, plan.URL)
		probe = &got
	}
	eng := &load.Engine{Policy: policy, Client: client}
	dur := time.Duration(plan.DurationSeconds) * time.Second
	var gate struct {
		mu      sync.Mutex
		checked time.Time
		cancel  bool
	}
	_, err := eng.Run(ctx, load.Plan{
		URL: plan.URL, Preset: p, MaxRPS: plan.MaxRPS, Duration: dur, TaskIndex: index, TaskCount: count,
	}, func() bool {
		gate.mu.Lock()
		defer gate.mu.Unlock()
		if time.Since(gate.checked) < time.Second {
			return gate.cancel
		}
		gate.checked = time.Now()
		gate.cancel = fetchPlan(ctx, base, runID, token).Cancel
		return gate.cancel
	}, func(sm store.Sample) {
		post(ctx, base+"/v1/internal/runs/"+runID+"/samples", token, map[string]any{
			"samples": []any{map[string]any{
				"sec": sm.Sec, "workers": sm.Workers, "requests": sm.Requests,
				"errors": sm.Errors, "latency_ms": sm.LatencyMS,
			}},
		})
	})
	body := map[string]any{}
	if err != nil {
		body["error"] = err.Error()
	}
	if probe != nil {
		body["probe"] = probe
	}
	post(ctx, base+"/v1/internal/runs/"+runID+"/complete", token, body)
	if err != nil {
		log.Fatal(err)
	}
}

type planDoc struct {
	URL             string  `json:"url"`
	Preset          string  `json:"preset"`
	DurationSeconds int     `json:"duration_seconds"`
	MaxRPS          float64 `json:"max_rps"`
	Cancel          bool    `json:"cancel"`
}

func fetchPlan(ctx context.Context, base, runID, token string) planDoc {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/internal/runs/"+runID+"/plan", nil)
	if err != nil {
		log.Fatal(err)
	}
	authorize(ctx, req, token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Fatalf("plan status %d", resp.StatusCode)
	}
	var doc planDoc
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		log.Fatal(err)
	}
	return doc
}

func post(ctx context.Context, url, token string, body any) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		log.Printf("post %s: %v", url, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	authorize(ctx, req, token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("post %s: %v", url, err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("post %s status %d", url, resp.StatusCode)
	}
}

func authorize(ctx context.Context, req *http.Request, token string) {
	req.Header.Set("X-Stampede-Token", token)
	if aud := os.Getenv("ORCHESTRATOR_AUDIENCE"); aud != "" {
		if id, err := gcpauth.Identity(ctx, aud); err == nil {
			req.Header.Set("Authorization", "Bearer "+id)
		}
	}
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
