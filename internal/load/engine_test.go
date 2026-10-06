package load

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/icohangar-ops/stampede/internal/preset"
	"github.com/icohangar-ops/stampede/internal/safety"
)

func TestEngineGETOnlyAndCaps(t *testing.T) {
	var gets, others atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.ContentLength == 0 && r.Body != nil {
			gets.Add(1)
		} else {
			others.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := safety.DefaultPolicy()
	p.MaxRPS = 8
	p.MaxDuration = 5 * time.Second
	p.MaxWorkers = 50
	p.AllowHosts = safety.ParseAllow("127.0.0.1")
	eng := &Engine{Policy: p, Client: p.HTTPClient()}
	plan := Plan{
		URL: srv.URL + "/", Preset: preset.Top5(), MaxRPS: 10_000, Duration: 3 * time.Second,
		TaskIndex: 0, TaskCount: 1,
	}
	res, err := eng.Run(context.Background(), plan, func() bool { return false }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if others.Load() != 0 {
		t.Fatalf("non-GET requests: %d", others.Load())
	}
	if gets.Load() == 0 {
		t.Fatal("expected some GETs")
	}
	// 3 seconds at 8 rps is the hard cap. Allow a small scheduling overshoot of zero
	// because the engine rounds per second and clamps to the policy cap.
	if gets.Load() > int64(p.MaxRPS)*3+3 {
		t.Fatalf("exceeded cap: %d", gets.Load())
	}
	if len(res.Samples) < 3 {
		t.Fatalf("samples %d", len(res.Samples))
	}
	if res.Samples[0].Workers < 9 || res.Samples[0].Workers > 11 {
		t.Fatalf("start workers %d", res.Samples[0].Workers)
	}
	last := res.Samples[len(res.Samples)-1]
	if last.Workers < 48 || last.Workers > 50 {
		t.Fatalf("end workers %d", last.Workers)
	}
}

func TestEngineStopsWhenKilled(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()
	p := safety.DefaultPolicy()
	p.AllowHosts = safety.ParseAllow("127.0.0.1")
	eng := &Engine{Policy: p, Client: p.HTTPClient()}
	res, err := eng.Run(context.Background(), Plan{
		URL: srv.URL, Preset: preset.NumberOne(), MaxRPS: 20, Duration: 8 * time.Second,
		TaskCount: 1,
	}, func() bool { return true }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Cancelled {
		t.Fatal("expected cancel")
	}
	if hits.Load() != 0 {
		t.Fatalf("kill switch still sent %d requests", hits.Load())
	}
}

func TestRejectsNonGET(t *testing.T) {
	if err := ValidateMethod(http.MethodPost); err != ErrMethod {
		t.Fatal(err)
	}
	if err := ValidateMethod(http.MethodGet); err != nil {
		t.Fatal(err)
	}
}

func TestSplitAcrossTasks(t *testing.T) {
	if split(50, 0, 10) != 5 || split(50, 9, 10) != 5 {
		t.Fatalf("even split")
	}
	if split(11, 0, 10) != 2 || split(11, 1, 10) != 1 {
		t.Fatalf("remainder")
	}
}

func TestPrivateTargetNeverContacted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("private target was contacted")
	}))
	defer srv.Close()
	eng := &Engine{Policy: safety.DefaultPolicy()}
	_, err := eng.Run(context.Background(), Plan{
		URL: srv.URL, Preset: preset.Top5(), MaxRPS: 5, Duration: time.Second, TaskCount: 1,
	}, nil, nil)
	if err == nil {
		t.Fatal("expected SSRF rejection")
	}
}
