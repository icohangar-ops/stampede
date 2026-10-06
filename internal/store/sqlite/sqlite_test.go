package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/icohangar-ops/stampede/internal/store"
)

func TestSQLiteQuotaAndRoundTrip(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "stampede.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := s.CreateChallenge(ctx, store.Challenge{
		ID: "c1", URL: "https://example.com/", Host: "example.com", Token: "st_abc",
		ClientIP: "9.9.9.9", CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	n, err := s.CountChallenges(ctx, "9.9.9.9", now.Add(-time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("count %d %v", n, err)
	}
	if err := s.MarkVerified(ctx, "c1", "dns", now); err != nil {
		t.Fatal(err)
	}
	run := store.Run{
		ID: "r1", ChallengeID: "c1", URL: "https://example.com/", Host: "example.com",
		Preset: "top5", ClientIP: "9.9.9.9", Status: "running", StartedAt: now,
		MaxRPS: 40, Duration: 45 * time.Second, TaskCount: 1,
	}
	if err := s.CreateRun(ctx, run, 1, 5); err != nil {
		t.Fatal(err)
	}
	again := run
	again.ID = "r2"
	if err := s.CreateRun(ctx, again, 1, 5); err != store.ErrQuota {
		t.Fatalf("quota: %v", err)
	}
	if err := s.MergeSample(ctx, store.Sample{RunID: "r1", Sec: 0, Workers: 10, Requests: 8, Errors: 0, LatencyMS: []float64{20, 30, 40}}); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeSample(ctx, store.Sample{RunID: "r1", Sec: 0, Workers: 2, Requests: 2, Errors: 1, LatencyMS: []float64{100}}); err != nil {
		t.Fatal(err)
	}
	samples, err := s.ListSamples(ctx, "r1")
	if err != nil || len(samples) != 1 || samples[0].Requests != 10 || samples[0].Workers != 12 {
		t.Fatalf("%v %+v", err, samples)
	}
	run.Status = "completed"
	run.EndedAt = now.Add(time.Minute)
	run.Verdict = "launch_ready"
	if err := s.UpdateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRun(ctx, "r1")
	if err != nil || got.Status != "completed" || got.Verdict != "launch_ready" {
		t.Fatalf("%+v %v", got, err)
	}
	if err := s.SetKill(ctx, true); err != nil {
		t.Fatal(err)
	}
	on, err := s.KillEnabled(ctx)
	if err != nil || !on {
		t.Fatal(on, err)
	}
}
