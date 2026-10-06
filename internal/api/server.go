// Package api is the orchestrator HTTP surface.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/icohangar-ops/stampede/internal/badge"
	"github.com/icohangar-ops/stampede/internal/blob"
	"github.com/icohangar-ops/stampede/internal/id"
	"github.com/icohangar-ops/stampede/internal/launch"
	"github.com/icohangar-ops/stampede/internal/load"
	"github.com/icohangar-ops/stampede/internal/preset"
	"github.com/icohangar-ops/stampede/internal/report"
	"github.com/icohangar-ops/stampede/internal/safety"
	"github.com/icohangar-ops/stampede/internal/store"
	"github.com/icohangar-ops/stampede/internal/verify"
)

// Server is the orchestrator.
type Server struct {
	Store          store.Store
	Policy         safety.Policy
	Reports        report.Builder
	Blobs          blob.Store
	AdminToken     string
	InternalToken  string
	DemoPublic     string
	DemoInternal   string
	KillFile       string
	EnvKill        bool
	DomainQuota    int
	IPQuota        int
	Mode           string
	Launcher       launch.Launcher
	TrustProxy     bool
	ModelName      string
	ReportURL      string
	ReportToken    string
	ReportAudience string

	mux      *http.ServeMux
	client   *http.Client
	finishMu sync.Mutex
	log      *log.Logger
}

// New wires routes. Quotas default to 3 per domain and 5 per IP per UTC day.
func New(s *Server) *Server {
	if s.DomainQuota <= 0 {
		s.DomainQuota = 3
	}
	if s.IPQuota <= 0 {
		s.IPQuota = 5
	}
	if s.Blobs == nil {
		s.Blobs = blob.Local{Dir: "data/artifacts"}
	}
	if s.log == nil {
		s.log = log.Default()
	}
	s.client = s.Policy.HTTPClient()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /v1/config", s.config)
	mux.HandleFunc("GET /v1/presets", s.presets)
	mux.HandleFunc("POST /v1/challenges", s.createChallenge)
	mux.HandleFunc("GET /v1/challenges/{id}", s.getChallenge)
	mux.HandleFunc("POST /v1/challenges/{id}/verify", s.verifyChallenge)
	mux.HandleFunc("POST /v1/runs", s.createRun)
	mux.HandleFunc("GET /v1/runs/{id}", s.getRun)
	mux.HandleFunc("GET /v1/runs/{id}/events", s.events)
	mux.HandleFunc("GET /v1/runs/{id}/badge.svg", s.badge)
	mux.HandleFunc("POST /v1/admin/kill", s.adminKill)
	mux.HandleFunc("POST /v1/admin/resume", s.adminResume)
	mux.HandleFunc("GET /v1/internal/runs/{id}/plan", s.plan)
	mux.HandleFunc("POST /v1/internal/runs/{id}/samples", s.ingest)
	mux.HandleFunc("POST /v1/internal/runs/{id}/complete", s.complete)
	mux.HandleFunc("POST /v1/internal/cron/retest", s.retest)
	s.mux = mux
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) Listen(addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return srv.ListenAndServe()
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	model := s.ModelName
	if model == "" {
		model = "template"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"demo_url":             strings.TrimRight(s.DemoPublic, "/"),
		"max_rps":              s.Policy.MaxRPS,
		"max_duration_seconds": int(s.Policy.MaxDuration.Seconds()),
		"max_workers":          s.Policy.MaxWorkers,
		"kill":                 s.killed(r.Context()),
		"model":                model,
		"domain_quota":         s.DomainQuota,
		"ip_quota":             s.IPQuota,
	})
}

func (s *Server) presets(w http.ResponseWriter, r *http.Request) {
	out := make([]map[string]any, 0, 2)
	for _, p := range preset.All() {
		out = append(out, map[string]any{
			"id":               p.ID,
			"name":             p.Name,
			"blurb":            p.Blurb,
			"assumptions":      p.Assumptions,
			"duration_seconds": int(p.Duration.Seconds()),
			"start_workers":    p.StartWorkers,
			"end_workers":      p.EndWorkers,
			"peak_fraction":    p.PeakFraction(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"presets": out})
}

func (s *Server) createChallenge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if !decode(w, r, &body) {
		return
	}
	target, err := s.Policy.Normalize(body.URL)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ip := clientIP(r, s.TrustProxy)
	n, err := s.Store.CountChallenges(r.Context(), ip, time.Now().Add(-time.Hour))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not check challenge rate")
		return
	}
	if n >= 30 {
		writeErr(w, http.StatusTooManyRequests, "too many verification challenges from this network")
		return
	}
	now := time.Now().UTC()
	ch := store.Challenge{
		ID: id.New(), URL: target.URL.String(), Host: target.Host, Token: id.Token(),
		ClientIP: ip, CreatedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	}
	if err := s.Store.CreateChallenge(r.Context(), ch); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not store challenge")
		return
	}
	writeJSON(w, http.StatusCreated, challengeView(ch))
}

func (s *Server) getChallenge(w http.ResponseWriter, r *http.Request) {
	ch, err := s.Store.GetChallenge(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "challenge not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load challenge")
		return
	}
	writeJSON(w, http.StatusOK, challengeView(ch))
}

func (s *Server) verifyChallenge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Method string `json:"method"`
	}
	if !decode(w, r, &body) {
		return
	}
	ch, err := s.Store.GetChallenge(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "challenge not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load challenge")
		return
	}
	if ch.Verified && time.Now().Before(ch.ExpiresAt) {
		writeJSON(w, http.StatusOK, map[string]any{"verified": true, "method": ch.Method})
		return
	}
	if time.Now().After(ch.ExpiresAt) {
		writeErr(w, http.StatusGone, "verification challenge expired")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	switch body.Method {
	case "file":
		page, err := url.Parse(s.rewrite(ch.URL))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "challenge URL is not usable")
			return
		}
		if _, err := s.Policy.Normalize(page.String()); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		file := verify.FileURL(page, ch.Token)
		if err := verify.CheckFile(ctx, s.client, file.String(), ch.Token); err != nil {
			writeErr(w, http.StatusUnprocessableEntity, "ownership file did not match the token")
			return
		}
	case "dns":
		if err := verify.CheckDNS(ctx, nil, ch.Host, ch.Token); err != nil {
			writeErr(w, http.StatusUnprocessableEntity, "DNS TXT record did not match the token")
			return
		}
	default:
		writeErr(w, http.StatusBadRequest, "method must be file or dns")
		return
	}
	now := time.Now().UTC()
	if err := s.Store.MarkVerified(r.Context(), ch.ID, body.Method, now); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not store verification")
		return
	}
	_ = s.Store.UpsertHost(r.Context(), store.Host{
		Host: ch.Host, URL: ch.URL, OptIn: false, VerifiedAt: now, ChallengeID: ch.ID,
	})
	writeJSON(w, http.StatusOK, map[string]any{"verified": true, "method": body.Method})
}

func (s *Server) createRun(w http.ResponseWriter, r *http.Request) {
	if s.killed(r.Context()) {
		writeErr(w, http.StatusConflict, "stampede is paused")
		return
	}
	var body struct {
		ChallengeID     string `json:"challenge_id"`
		Preset          string `json:"preset"`
		OptIn           bool   `json:"opt_in"`
		DurationSeconds int    `json:"duration_seconds"`
	}
	if !decode(w, r, &body) {
		return
	}
	ch, err := s.Store.GetChallenge(r.Context(), body.ChallengeID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "challenge not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load challenge")
		return
	}
	if !ch.Verified || time.Now().After(ch.ExpiresAt) {
		writeErr(w, http.StatusForbidden, "ownership verification required")
		return
	}
	p, ok := preset.ByID(body.Preset)
	if !ok {
		writeErr(w, http.StatusBadRequest, "unknown preset")
		return
	}
	dur := p.Duration
	if body.DurationSeconds > 0 {
		dur = time.Duration(body.DurationSeconds) * time.Second
	}
	if err := s.Policy.ValidatePlan(s.Policy.MaxRPS, dur, p.EndWorkers); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	fetch := s.rewrite(ch.URL)
	if _, err := s.Policy.Normalize(fetch); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now().UTC()
	tasks := 1
	if s.Mode == "cloudrun" && s.Launcher != nil {
		tasks = s.Launcher.Tasks()
	}
	run := store.Run{
		ID: id.New(), ChallengeID: ch.ID, URL: fetch, Host: ch.Host, Preset: p.ID,
		ClientIP: clientIP(r, s.TrustProxy), Status: "running", StartedAt: now,
		MaxRPS: s.Policy.MaxRPS, Duration: dur, OptIn: body.OptIn, TaskCount: tasks,
	}
	run.URL = ch.URL
	if err := s.Store.CreateRun(r.Context(), run, s.DomainQuota, s.IPQuota); err != nil {
		if errors.Is(err, store.ErrQuota) {
			writeErr(w, http.StatusTooManyRequests, "daily quota exceeded for this domain or network")
			return
		}
		writeErr(w, http.StatusInternalServerError, "could not start run")
		return
	}
	if body.OptIn {
		_ = s.Store.UpsertHost(r.Context(), store.Host{
			Host: ch.Host, URL: ch.URL, OptIn: true, VerifiedAt: ch.VerifiedAt, ChallengeID: ch.ID,
		})
	}
	if s.Mode == "cloudrun" && s.Launcher != nil {
		if err := s.Launcher.Launch(r.Context(), run.ID); err != nil {
			run.Status = "failed"
			run.Error = "could not start load generator"
			run.EndedAt = time.Now().UTC()
			_ = s.Store.UpdateRun(r.Context(), run)
			s.log.Printf("launch run %s: %v", run.ID, err)
			writeErr(w, http.StatusBadGateway, "could not start load generator")
			return
		}
	} else {
		go s.execute(run.ID)
	}
	writeJSON(w, http.StatusCreated, s.runView(r.Context(), run, true))
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.Store.GetRun(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "run not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load run")
		return
	}
	writeJSON(w, http.StatusOK, s.runView(r.Context(), run, true))
}

func (s *Server) badge(w http.ResponseWriter, r *http.Request) {
	run, err := s.Store.GetRun(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "unavailable", http.StatusInternalServerError)
		return
	}
	body := badge.SVG(run.Status, run.Verdict, run.PeakP95, run.HasBreaking)
	if run.Status == "completed" {
		if b, _, err := s.Blobs.Get(r.Context(), "runs/"+run.ID+"/badge.svg"); err == nil && len(b) > 0 {
			body = string(b)
		}
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, body)
}

func (s *Server) adminKill(w http.ResponseWriter, r *http.Request) {
	if !tokenMatch(adminCredential(r), s.AdminToken) {
		writeErr(w, http.StatusUnauthorized, "admin token required")
		return
	}
	if err := s.Store.SetKill(r.Context(), true); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not engage kill switch")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"kill": true})
}

func (s *Server) adminResume(w http.ResponseWriter, r *http.Request) {
	if !tokenMatch(adminCredential(r), s.AdminToken) {
		writeErr(w, http.StatusUnauthorized, "admin token required")
		return
	}
	if s.EnvKill {
		writeErr(w, http.StatusConflict, "kill switch is pinned by the environment")
		return
	}
	if err := s.Store.SetKill(r.Context(), false); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not resume")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"kill": false})
}

func (s *Server) plan(w http.ResponseWriter, r *http.Request) {
	if !s.internalOK(r) {
		writeErr(w, http.StatusUnauthorized, "internal token required")
		return
	}
	run, err := s.Store.GetRun(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "run not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load run")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":           run.ID,
		"url":              run.URL,
		"preset":           run.Preset,
		"duration_seconds": int(run.Duration.Seconds()),
		"max_rps":          run.MaxRPS,
		"task_count":       run.TaskCount,
		"cancel":           s.killed(r.Context()) || run.Status != "running",
	})
}

func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	if !s.internalOK(r) {
		writeErr(w, http.StatusUnauthorized, "internal token required")
		return
	}
	var body struct {
		Samples []struct {
			Sec       int       `json:"sec"`
			Workers   int       `json:"workers"`
			Requests  int       `json:"requests"`
			Errors    int       `json:"errors"`
			LatencyMS []float64 `json:"latency_ms"`
		} `json:"samples"`
	}
	if !decode(w, r, &body) {
		return
	}
	id := r.PathValue("id")
	if _, err := s.Store.GetRun(r.Context(), id); err != nil {
		writeErr(w, http.StatusNotFound, "run not found")
		return
	}
	for _, sm := range body.Samples {
		if sm.Sec < 0 || sm.Sec > 300 || sm.Requests < 0 || sm.Requests > 5000 || len(sm.LatencyMS) > 5000 {
			writeErr(w, http.StatusBadRequest, "sample out of range")
			return
		}
		err := s.Store.MergeSample(r.Context(), store.Sample{
			RunID: id, Sec: sm.Sec, Workers: sm.Workers, Requests: sm.Requests, Errors: sm.Errors, LatencyMS: sm.LatencyMS,
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "could not store sample")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cancel": s.killed(r.Context())})
}

func (s *Server) complete(w http.ResponseWriter, r *http.Request) {
	if !s.internalOK(r) {
		writeErr(w, http.StatusUnauthorized, "internal token required")
		return
	}
	var body struct {
		Error string            `json:"error"`
		Probe *load.ProbeResult `json:"probe"`
	}
	if !decode(w, r, &body) {
		return
	}
	runID := r.PathValue("id")
	if body.Probe != nil {
		if raw, err := json.Marshal(body.Probe); err == nil {
			_ = s.Store.SaveProbe(r.Context(), runID, string(raw))
		}
	}
	done, total, finished, err := s.Store.MarkTaskDone(r.Context(), runID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "run not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not record task")
		return
	}
	if finished {
		go s.finish(runID, nil, s.killed(context.Background()))
	}
	writeJSON(w, http.StatusOK, map[string]any{"done": done, "total": total, "finished": finished})
}

func (s *Server) retest(w http.ResponseWriter, r *http.Request) {
	if !s.internalOK(r) {
		writeErr(w, http.StatusUnauthorized, "internal token required")
		return
	}
	if s.killed(r.Context()) {
		writeErr(w, http.StatusConflict, "stampede is paused")
		return
	}
	hosts, err := s.Store.ListOptIn(r.Context(), time.Now().Add(-30*24*time.Hour))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list opt-in hosts")
		return
	}
	var started []string
	for _, h := range hosts {
		ch, err := s.Store.GetChallenge(r.Context(), h.ChallengeID)
		if err != nil || !ch.Verified {
			continue
		}
		if time.Since(h.VerifiedAt) > 30*24*time.Hour {
			continue
		}
		runID, err := s.startInternal(r.Context(), ch, "top5", false, "scheduler")
		if err != nil {
			continue
		}
		started = append(started, runID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"started": started})
}

func (s *Server) startInternal(ctx context.Context, ch store.Challenge, presetID string, optIn bool, ip string) (string, error) {
	p, ok := preset.ByID(presetID)
	if !ok {
		return "", errors.New("preset")
	}
	fetch := s.rewrite(ch.URL)
	if _, err := s.Policy.Normalize(fetch); err != nil {
		return "", err
	}
	tasks := 1
	if s.Mode == "cloudrun" && s.Launcher != nil {
		tasks = s.Launcher.Tasks()
	}
	run := store.Run{
		ID: id.New(), ChallengeID: ch.ID, URL: fetch, Host: ch.Host, Preset: p.ID,
		ClientIP: ip, Status: "running", StartedAt: time.Now().UTC(),
		MaxRPS: s.Policy.MaxRPS, Duration: p.Duration, OptIn: optIn, TaskCount: tasks,
	}
	run.URL = ch.URL
	if err := s.Store.CreateRun(ctx, run, s.DomainQuota, s.IPQuota); err != nil {
		return "", err
	}
	if s.Mode == "cloudrun" && s.Launcher != nil {
		if err := s.Launcher.Launch(ctx, run.ID); err != nil {
			run.Status = "failed"
			run.EndedAt = time.Now().UTC()
			run.Error = "could not start load generator"
			_ = s.Store.UpdateRun(ctx, run)
			return "", err
		}
	} else {
		go s.execute(run.ID)
	}
	return run.ID, nil
}

func (s *Server) internalOK(r *http.Request) bool {
	return tokenMatch(r.Header.Get("X-Stampede-Token"), s.InternalToken)
}

func (s *Server) killed(ctx context.Context) bool {
	if s.EnvKill {
		return true
	}
	if s.KillFile != "" {
		// A present file is enough. Stat via os in a tiny helper to keep this file free of a broad os import use.
		if killFilePresent(s.KillFile) {
			return true
		}
	}
	on, err := s.Store.KillEnabled(ctx)
	return err == nil && on
}

func (s *Server) rewrite(raw string) string {
	if s.DemoPublic == "" || s.DemoInternal == "" {
		return raw
	}
	u, err := url.Parse(raw)
	pub, err2 := url.Parse(s.DemoPublic)
	in, err3 := url.Parse(s.DemoInternal)
	if err != nil || err2 != nil || err3 != nil {
		return raw
	}
	if !strings.EqualFold(u.Hostname(), pub.Hostname()) {
		return raw
	}
	if portOrDefault(u) != portOrDefault(pub) {
		return raw
	}
	u.Scheme = in.Scheme
	u.Host = in.Host
	return u.String()
}

func portOrDefault(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

func challengeView(ch store.Challenge) map[string]any {
	page, _ := url.Parse(ch.URL)
	file := ""
	if page != nil {
		file = verify.FileURL(page, ch.Token).String()
	}
	return map[string]any{
		"id":         ch.ID,
		"url":        ch.URL,
		"host":       ch.Host,
		"token":      ch.Token,
		"file_url":   file,
		"file_path":  "/.well-known/stampede-" + ch.Token + ".txt",
		"dns_name":   verify.DNSName(ch.Host),
		"dns_value":  verify.DNSValue(ch.Token),
		"expires_at": ch.ExpiresAt,
		"verified":   ch.Verified,
		"method":     ch.Method,
	}
}

func (s *Server) runView(ctx context.Context, run store.Run, withSamples bool) map[string]any {
	var rep any
	if run.ReportJSON != "" {
		var parsed report.Report
		if json.Unmarshal([]byte(run.ReportJSON), &parsed) == nil {
			rep = parsed
		}
	}
	view := map[string]any{
		"id":               run.ID,
		"url":              run.URL,
		"host":             run.Host,
		"preset":           run.Preset,
		"status":           run.Status,
		"started_at":       run.StartedAt,
		"max_rps":          run.MaxRPS,
		"duration_seconds": int(run.Duration.Seconds()),
		"has_breaking":     run.HasBreaking,
		"breaking_rps":     run.BreakingRPS,
		"peak_rps":         run.PeakRPS,
		"peak_p95_ms":      run.PeakP95,
		"error_rate":       run.PeakError,
		"verdict":          run.Verdict,
		"report":           rep,
		"error":            run.Error,
		"badge_path":       "/v1/runs/" + run.ID + "/badge.svg",
		"result_path":      "/r/" + run.ID,
	}
	if !run.EndedAt.IsZero() {
		view["ended_at"] = run.EndedAt
	}
	if withSamples {
		samples, err := s.Store.ListSamples(ctx, run.ID)
		if err == nil {
			out := make([]map[string]any, 0, len(samples))
			for _, sm := range samples {
				out = append(out, map[string]any{
					"sec": sm.Sec, "workers": sm.Workers, "rps": sm.RPS,
					"p50_ms": sm.P50, "p95_ms": sm.P95, "error_rate": sm.ErrorRate,
					"requests": sm.Requests, "errors": sm.Errors,
				})
			}
			view["samples"] = out
		}
	}
	return view
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
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

func adminCredential(r *http.Request) string {
	if t := strings.TrimSpace(r.Header.Get("X-Stampede-Admin")); t != "" {
		return t
	}
	return bearer(r)
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func tokenMatch(got, want string) bool {
	if want == "" || got == "" || len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// silence unused in case fmt is needed by logs later
var _ = fmt.Sprintf
