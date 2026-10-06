package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/icohangar-ops/stampede/internal/badge"
	"github.com/icohangar-ops/stampede/internal/load"
	"github.com/icohangar-ops/stampede/internal/metrics"
	"github.com/icohangar-ops/stampede/internal/preset"
	"github.com/icohangar-ops/stampede/internal/report"
	"github.com/icohangar-ops/stampede/internal/store"
)

func killFilePresent(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func (s *Server) execute(runID string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.Policy.MaxDuration+45*time.Second)
	defer cancel()
	run, err := s.Store.GetRun(ctx, runID)
	if err != nil {
		return
	}
	target := s.rewrite(run.URL)
	probe := load.Probe(ctx, s.client, target)
	if raw, err := json.Marshal(probe); err == nil {
		_ = s.Store.SaveProbe(ctx, runID, string(raw))
		run.ProbeJSON = string(raw)
	}
	p, ok := preset.ByID(run.Preset)
	if !ok {
		s.finish(runID, fmt.Errorf("unknown preset"), false)
		return
	}
	eng := &load.Engine{Policy: s.Policy, Client: s.client}
	res, err := eng.Run(ctx, load.Plan{
		URL: target, Preset: p, MaxRPS: run.MaxRPS, Duration: run.Duration, TaskIndex: 0, TaskCount: 1,
	}, func() bool { return s.killed(context.Background()) }, func(sm store.Sample) {
		sm.RunID = runID
		_ = s.Store.MergeSample(context.Background(), sm)
	})
	s.finish(runID, err, res.Cancelled || s.killed(context.Background()))
}

func (s *Server) finish(runID string, runErr error, cancelled bool) {
	s.finishMu.Lock()
	defer s.finishMu.Unlock()
	ctx := context.Background()
	run, err := s.Store.GetRun(ctx, runID)
	if err != nil {
		return
	}
	if run.Status != "running" {
		return
	}
	samples, _ := s.Store.ListSamples(ctx, runID)
	sum := metrics.Summarize(samples)
	var probe load.ProbeResult
	if run.ProbeJSON != "" {
		_ = json.Unmarshal([]byte(run.ProbeJSON), &probe)
	}
	p, _ := preset.ByID(run.Preset)
	rep := s.buildReport(ctx, report.Input{
		URL: run.URL, PresetID: run.Preset, PresetName: p.Name, MaxRPS: run.MaxRPS, Summary: sum, Probe: probe,
	})
	raw, _ := json.Marshal(rep)
	run.ReportJSON = string(raw)
	run.HasBreaking = sum.HasBreaking
	run.BreakingRPS = sum.BreakingRPS
	run.PeakRPS = sum.PeakRPS
	run.PeakP95 = sum.PeakP95
	run.PeakError = sum.PeakError
	run.Verdict = rep.Verdict
	run.EndedAt = time.Now().UTC()
	switch {
	case cancelled:
		run.Status = "cancelled"
	case runErr != nil && sum.Requests == 0:
		run.Status = "failed"
		run.Error = "load generator failed"
		s.log.Printf("run %s failed: %v", runID, runErr)
	default:
		run.Status = "completed"
	}
	if err := s.Store.UpdateRun(ctx, run); err != nil {
		s.log.Printf("update run %s: %v", runID, err)
		return
	}
	svg := badge.SVG(run.Status, run.Verdict, run.PeakP95, run.HasBreaking)
	_ = s.Blobs.Put(ctx, "runs/"+runID+"/badge.svg", []byte(svg), "image/svg+xml")
	_ = s.Blobs.Put(ctx, "runs/"+runID+"/report.json", raw, "application/json")
}

func (s *Server) buildReport(ctx context.Context, in report.Input) report.Report {
	if s.ReportURL == "" {
		return s.Reports.Build(ctx, in)
	}
	base := report.Template(in)
	body, _ := json.Marshal(in)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.ReportURL+"/v1/summarize", bytesReader(body))
	if err != nil {
		base.Notice = "The report service was unavailable, so this is the deterministic report."
		return base
	}
	req.Header.Set("Content-Type", "application/json")
	if s.ReportToken != "" {
		req.Header.Set("X-Stampede-Token", s.ReportToken)
	}
	if s.ReportAudience != "" {
		if tok, err := identity(ctx, s.ReportAudience); err == nil && tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		base.Notice = "The report service was unavailable, so this is the deterministic report."
		return base
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		base.Notice = "The report service was unavailable, so this is the deterministic report."
		return base
	}
	var got report.Report
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil || got.Headline == "" {
		base.Notice = "The report service reply could not be used, so this is the deterministic report."
		return base
	}
	base.Headline = got.Headline
	if got.Summary != "" {
		base.Summary = got.Summary
	}
	if len(got.Fixes) > 0 {
		base.Fixes = got.Fixes
	}
	if got.Model != "" {
		base.Model = got.Model
	}
	return base
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "stream unsupported")
		return
	}
	id := r.PathValue("id")
	if _, err := s.Store.GetRun(r.Context(), id); err != nil {
		writeErr(w, http.StatusNotFound, "run not found")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	sent := 0
	deadline := time.NewTimer(s.Policy.MaxDuration + 90*time.Second)
	defer deadline.Stop()
	for {
		samples, err := s.Store.ListSamples(r.Context(), id)
		if err == nil {
			for _, sm := range samples[sent:] {
				payload, _ := json.Marshal(map[string]any{
					"sec": sm.Sec, "workers": sm.Workers, "rps": sm.RPS,
					"p50_ms": sm.P50, "p95_ms": sm.P95, "error_rate": sm.ErrorRate,
					"requests": sm.Requests, "errors": sm.Errors,
				})
				fmt.Fprintf(w, "event: sample\ndata: %s\n\n", payload)
				sent++
			}
		}
		run, err := s.Store.GetRun(r.Context(), id)
		if err != nil {
			return
		}
		if run.Status != "running" {
			payload, _ := json.Marshal(map[string]string{"status": run.Status})
			fmt.Fprintf(w, "event: done\ndata: %s\n\n", payload)
			flusher.Flush()
			return
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-deadline.C:
			fmt.Fprintf(w, "event: done\ndata: {\"status\":\"timeout\"}\n\n")
			flusher.Flush()
			return
		case <-time.After(400 * time.Millisecond):
		}
	}
}
