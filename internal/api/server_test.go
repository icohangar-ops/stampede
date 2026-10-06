package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/icohangar-ops/stampede/internal/blob"
	"github.com/icohangar-ops/stampede/internal/demotarget"
	"github.com/icohangar-ops/stampede/internal/safety"
	"github.com/icohangar-ops/stampede/internal/store/memory"
)

func newTest(t *testing.T, quota int) (*Server, *httptest.Server, *httptest.Server) {
	t.Helper()
	target := httptest.NewServer(demotarget.Handler())
	t.Cleanup(target.Close)
	policy := safety.DefaultPolicy()
	policy.AllowHosts = safety.ParseAllow("127.0.0.1")
	policy.MaxRPS = 8
	policy.MaxDuration = 20 * time.Second
	policy.MaxWorkers = 50
	s := New(&Server{
		Store: memory.New(), Policy: policy, Blobs: blob.Local{Dir: t.TempDir()},
		AdminToken: "admin-token-test", InternalToken: "internal-token-test",
		DomainQuota: quota, IPQuota: 10, DemoPublic: target.URL,
	})
	apiSrv := httptest.NewServer(s.Handler())
	t.Cleanup(apiSrv.Close)
	return s, apiSrv, target
}

func TestOwnershipRequiredAndHappyPath(t *testing.T) {
	_, apiSrv, target := newTest(t, 5)
	var hits atomic.Int64
	// Wrap is unnecessary: demotarget counts via its own gate. Track via a front server instead.
	_ = hits
	ch := postJSON(t, apiSrv.URL+"/v1/challenges", map[string]string{"url": target.URL + "/"}, "")
	if ch["token"] == "" || ch["file_path"] == "" || ch["dns_name"] == "" {
		t.Fatalf("challenge %#v", ch)
	}
	denied := postJSONStatus(t, apiSrv.URL+"/v1/runs", map[string]string{
		"challenge_id": ch["id"].(string), "preset": "top5",
	}, "")
	if denied != http.StatusForbidden {
		t.Fatalf("unverified run status %d", denied)
	}

	verified := postJSON(t, apiSrv.URL+"/v1/challenges/"+ch["id"].(string)+"/verify", map[string]string{"method": "file"}, "")
	if verified["verified"] != true {
		t.Fatalf("verify %#v", verified)
	}
	run := postJSON(t, apiSrv.URL+"/v1/runs", map[string]any{
		"challenge_id": ch["id"], "preset": "top5", "duration_seconds": 3,
	}, "")
	id := run["id"].(string)
	deadline := time.Now().Add(20 * time.Second)
	var final map[string]any
	for time.Now().Before(deadline) {
		final = getJSON(t, apiSrv.URL+"/v1/runs/"+id)
		if final["status"] != "running" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if final["status"] != "completed" {
		t.Fatalf("status %#v", final["status"])
	}
	samples, _ := final["samples"].([]any)
	if len(samples) < 3 {
		t.Fatalf("samples %#v", final["samples"])
	}
	first := samples[0].(map[string]any)
	last := samples[len(samples)-1].(map[string]any)
	if int(first["workers"].(float64)) < 9 || int(last["workers"].(float64)) < 45 {
		t.Fatalf("worker ramp %v -> %v", first["workers"], last["workers"])
	}
	rep, _ := final["report"].(map[string]any)
	if rep == nil || rep["headline"] == "" || rep["cost"] == nil {
		t.Fatalf("report %#v", final["report"])
	}
	resp, err := http.Get(apiSrv.URL + "/v1/runs/" + id + "/badge.svg")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	svg, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(svg, []byte("Launch-ready")) && !bytes.Contains(svg, []byte("Needs work")) {
		t.Fatalf("badge %s", svg)
	}
	if bytes.Contains(svg, []byte("Launch-ready")) && final["has_breaking"] == true {
		t.Fatal("launch-ready badge on a broken run")
	}
}

func TestSSRFChallengeBlocked(t *testing.T) {
	_, apiSrv, _ := newTest(t, 5)
	for _, raw := range []string{
		"http://169.254.169.254/",
		"http://10.1.2.3/",
		"http://metadata.google.internal/",
		"http://example.com:22/",
		"file:///etc/passwd",
	} {
		status := postJSONStatus(t, apiSrv.URL+"/v1/challenges", map[string]string{"url": raw}, "")
		if status != http.StatusBadRequest {
			t.Fatalf("%s status %d", raw, status)
		}
	}
}

func TestKillSwitchAndQuota(t *testing.T) {
	s, apiSrv, target := newTest(t, 1)
	_ = s
	ch := postJSON(t, apiSrv.URL+"/v1/challenges", map[string]string{"url": target.URL}, "")
	postJSON(t, apiSrv.URL+"/v1/challenges/"+ch["id"].(string)+"/verify", map[string]string{"method": "file"}, "")
	if postJSONStatus(t, apiSrv.URL+"/v1/admin/kill", map[string]string{}, "admin-token-test") != http.StatusOK {
		t.Fatal("kill")
	}
	if postJSONStatus(t, apiSrv.URL+"/v1/runs", map[string]string{"challenge_id": ch["id"].(string), "preset": "top5"}, "") != http.StatusConflict {
		t.Fatal("expected kill to block the run")
	}
	if postJSONStatus(t, apiSrv.URL+"/v1/admin/resume", nil, "nope") != http.StatusUnauthorized {
		t.Fatal("bad admin token")
	}
	postJSON(t, apiSrv.URL+"/v1/admin/resume", map[string]string{}, "admin-token-test")
	first := postJSONStatus(t, apiSrv.URL+"/v1/runs", map[string]any{"challenge_id": ch["id"], "preset": "numberone", "duration_seconds": 3}, "")
	if first != http.StatusCreated {
		t.Fatalf("first run %d", first)
	}
	second := postJSONStatus(t, apiSrv.URL+"/v1/runs", map[string]any{"challenge_id": ch["id"], "preset": "top5", "duration_seconds": 3}, "")
	if second != http.StatusTooManyRequests {
		t.Fatalf("quota %d", second)
	}
}

func TestBreakingBadgeAndInternalMerge(t *testing.T) {
	var hits atomic.Int64
	policy := safety.DefaultPolicy()
	policy.AllowHosts = safety.ParseAllow("127.0.0.1")
	policy.MaxRPS = 6
	policy.MaxDuration = 15 * time.Second
	policy.MaxWorkers = 50
	s := New(&Server{
		Store: memory.New(), Policy: policy, Blobs: blob.Local{Dir: t.TempDir()},
		InternalToken: "internal-token-test", DomainQuota: 5, IPQuota: 5,
		Mode: "cloudrun", Launcher: fakeLaunch{n: 2},
	})
	apiSrv := httptest.NewServer(s.Handler())
	defer apiSrv.Close()

	combined := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "/.well-known/stampede-"
		if strings.HasPrefix(r.URL.Path, prefix) && strings.HasSuffix(r.URL.Path, ".txt") {
			w.Header().Set("Content-Type", "text/plain")
			token := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), ".txt")
			_, _ = io.WriteString(w, token)
			return
		}
		hits.Add(1)
		http.Error(w, "nope", http.StatusServiceUnavailable)
	}))
	defer combined.Close()

	ch := postJSON(t, apiSrv.URL+"/v1/challenges", map[string]string{"url": combined.URL + "/"}, "")
	postJSON(t, apiSrv.URL+"/v1/challenges/"+ch["id"].(string)+"/verify", map[string]string{"method": "file"}, "")
	before := hits.Load()
	run := postJSON(t, apiSrv.URL+"/v1/runs", map[string]any{
		"challenge_id": ch["id"], "preset": "top5", "duration_seconds": 4,
	}, "")
	if hits.Load() != before {
		t.Fatal("cloud mode should not send load in-process")
	}
	id := run["id"].(string)
	postInternal(t, apiSrv.URL+"/v1/internal/runs/"+id+"/samples", map[string]any{
		"samples": []map[string]any{{
			"sec": 1, "workers": 25, "requests": 10, "errors": 0, "latency_ms": []float64{30, 40},
		}},
	})
	postInternal(t, apiSrv.URL+"/v1/internal/runs/"+id+"/samples", map[string]any{
		"samples": []map[string]any{{
			"sec": 1, "workers": 25, "requests": 10, "errors": 8, "latency_ms": []float64{30, 40},
		}},
	})
	postInternal(t, apiSrv.URL+"/v1/internal/runs/"+id+"/complete", map[string]any{})
	postInternal(t, apiSrv.URL+"/v1/internal/runs/"+id+"/complete", map[string]any{})
	deadline := time.Now().Add(5 * time.Second)
	var final map[string]any
	for time.Now().Before(deadline) {
		final = getJSON(t, apiSrv.URL+"/v1/runs/"+id)
		if final["status"] != "running" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if final["status"] != "completed" || final["has_breaking"] != true {
		t.Fatalf("final %#v", final)
	}
	if final["breaking_rps"].(float64) != 20 {
		t.Fatalf("breaking %v", final["breaking_rps"])
	}
	resp, err := http.Get(apiSrv.URL + "/v1/runs/" + id + "/badge.svg")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	svg, _ := io.ReadAll(resp.Body)
	if bytes.Contains(svg, []byte("Launch-ready")) || !bytes.Contains(svg, []byte("Needs work")) {
		t.Fatalf("badge %s", svg)
	}
	req, err := http.NewRequest(http.MethodGet, apiSrv.URL+"/v1/internal/runs/"+id+"/plan", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("plan should require the internal token, got %d", resp.StatusCode)
	}
}

type fakeLaunch struct{ n int }

func (f fakeLaunch) Launch(context.Context, string) error { return nil }
func (f fakeLaunch) Tasks() int                           { return f.n }

func postJSON(t *testing.T, url string, body any, admin string) map[string]any {
	t.Helper()
	status, m := doJSON(t, url, body, admin, "")
	if status >= 300 {
		t.Fatalf("%s status %d %#v", url, status, m)
	}
	return m
}

func postJSONStatus(t *testing.T, url string, body any, admin string) int {
	t.Helper()
	status, _ := doJSON(t, url, body, admin, "")
	return status
}

func postInternal(t *testing.T, url string, body any) {
	t.Helper()
	status, m := doJSON(t, url, body, "", "internal-token-test")
	if status >= 300 {
		t.Fatalf("internal %s %d %#v", url, status, m)
	}
}

func doJSON(t *testing.T, url string, body any, admin, internal string) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body == nil {
		buf.WriteString(`{}`)
	} else if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if admin != "" {
		req.Header.Set("Authorization", "Bearer "+admin)
	}
	if internal != "" {
		req.Header.Set("X-Stampede-Token", internal)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return resp.StatusCode, m
}

func getJSON(t *testing.T, url string) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}
