// Package load sends the capped GET ramp and records one sample per second.
package load

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/icohangar-ops/stampede/internal/preset"
	"github.com/icohangar-ops/stampede/internal/safety"
	"github.com/icohangar-ops/stampede/internal/store"
)

// ErrMethod is returned if anything other than GET is requested.
var ErrMethod = errors.New("only GET is allowed")

// Plan is one task's view of a run. TaskCount 1 runs the whole curve.
type Plan struct {
	URL       string
	Preset    preset.Preset
	MaxRPS    float64
	Duration  time.Duration
	TaskIndex int
	TaskCount int
}

// Result is the set of samples this task emitted.
type Result struct {
	Samples   []store.Sample
	Cancelled bool
}

// Engine is the GET-only load generator.
type Engine struct {
	Policy safety.Policy
	Client *http.Client
}

func (e *Engine) client() *http.Client {
	if e.Client != nil {
		return e.Client
	}
	return e.Policy.HTTPClient()
}

// ValidateMethod rejects every method except GET.
func ValidateMethod(method string) error {
	if method != http.MethodGet {
		return ErrMethod
	}
	return nil
}

// Run executes the curve. killed is polled so a kill switch stops further requests.
// emit may be nil. Samples are also returned.
func (e *Engine) Run(ctx context.Context, plan Plan, killed func() bool, emit func(store.Sample)) (Result, error) {
	if err := ValidateMethod(http.MethodGet); err != nil {
		return Result{}, err
	}
	if _, err := e.Policy.Normalize(plan.URL); err != nil {
		return Result{}, err
	}
	maxRPS := plan.MaxRPS
	if maxRPS > e.Policy.MaxRPS {
		maxRPS = e.Policy.MaxRPS
	}
	if maxRPS <= 0 {
		maxRPS = e.Policy.MaxRPS
	}
	dur := plan.Duration
	if dur > e.Policy.MaxDuration || dur > 3*time.Minute {
		dur = e.Policy.MaxDuration
		if dur > 3*time.Minute {
			dur = 3 * time.Minute
		}
	}
	if dur < time.Second {
		dur = time.Second
	}
	workersCap := e.Policy.MaxWorkers
	if workersCap <= 0 {
		workersCap = safety.DefaultMaxWorkers
	}
	secs := int(dur.Seconds())
	if secs < 1 {
		secs = 1
	}
	if plan.TaskCount < 1 {
		plan.TaskCount = 1
	}
	if plan.TaskIndex < 0 || plan.TaskIndex >= plan.TaskCount {
		return Result{}, errors.New("task index out of range")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		tick := time.NewTicker(200 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-tick.C:
				if killed != nil && killed() {
					cancel()
					return
				}
			}
		}
	}()

	var out Result
	var sent int
	ceiling := int(math.Ceil(e.Policy.MaxRPS*e.Policy.MaxDuration.Seconds())) + 1
	client := e.client()
	for sec := 0; sec < secs; sec++ {
		if ctx.Err() != nil || (killed != nil && killed()) {
			out.Cancelled = true
			break
		}
		frac := preset.Fraction(sec, secs)
		workers := plan.Preset.Workers(frac)
		if workers > workersCap {
			workers = workersCap
		}
		rps := plan.Preset.RPS(frac, maxRPS)
		if rps > e.Policy.MaxRPS {
			rps = e.Policy.MaxRPS
		}
		totalN := int(math.Round(rps))
		myN := split(totalN, plan.TaskIndex, plan.TaskCount)
		myWorkers := split(workers, plan.TaskIndex, plan.TaskCount)
		if sent+myN > ceiling {
			myN = ceiling - sent
			if myN < 0 {
				myN = 0
			}
		}
		sample := e.fire(ctx, client, plan.URL, myN, myWorkers)
		sample.Sec = sec
		sample.Workers = myWorkers
		sent += sample.Requests
		out.Samples = append(out.Samples, sample)
		if emit != nil {
			emit(sample)
		}
		if sent >= ceiling {
			break
		}
	}
	return out, nil
}

func (e *Engine) fire(ctx context.Context, client *http.Client, rawURL string, n, workers int) store.Sample {
	sample := store.Sample{Workers: workers}
	if n <= 0 || ctx.Err() != nil {
		store.ApplyLatencies(&sample)
		return sample
	}
	if workers < 1 {
		workers = 1
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	pace := time.Second / time.Duration(n)
	if pace <= 0 {
		pace = time.Millisecond
	}
	for i := 0; i < n; i++ {
		if i > 0 {
			timer := time.NewTimer(pace)
			select {
			case <-ctx.Done():
				timer.Stop()
				goto wait
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
			if err != nil {
				mu.Lock()
				sample.Errors++
				sample.Requests++
				sample.LatencyMS = append(sample.LatencyMS, 0)
				mu.Unlock()
				return
			}
			if req.Method != http.MethodGet || req.Body != nil {
				cancel()
				mu.Lock()
				sample.Errors++
				sample.Requests++
				mu.Unlock()
				return
			}
			req.Header.Set("User-Agent", safety.UserAgent)
			req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")
			start := time.Now()
			resp, err := client.Do(req)
			ms := float64(time.Since(start).Microseconds()) / 1000
			mu.Lock()
			defer mu.Unlock()
			sample.Requests++
			sample.LatencyMS = append(sample.LatencyMS, ms)
			if err != nil || resp == nil || resp.StatusCode >= 400 {
				sample.Errors++
			}
			if resp != nil {
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
				resp.Body.Close()
			}
		}()
	}
wait:
	wg.Wait()
	store.ApplyLatencies(&sample)
	return sample
}

func split(total, index, count int) int {
	if count < 1 {
		count = 1
	}
	if index < 0 || index >= count || total <= 0 {
		return 0
	}
	base := total / count
	rem := total % count
	if index < rem {
		return base + 1
	}
	return base
}
