package memory

import (
	"context"
	"testing"
	"time"

	"github.com/icohangar-ops/stampede/internal/store"
)

func TestQuotaReservedOnce(t *testing.T) {
	s := New()
	ctx := context.Background()
	now := time.Now().UTC()
	mk := func(id string) store.Run {
		return store.Run{ID: id, Host: "example.com", ClientIP: "1.2.3.4", StartedAt: now, Status: "running"}
	}
	if err := s.CreateRun(ctx, mk("a"), 1, 5); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRun(ctx, mk("b"), 1, 5); err != store.ErrQuota {
		t.Fatalf("domain quota: %v", err)
	}
	other := mk("c")
	other.Host = "other.example"
	if err := s.CreateRun(ctx, other, 3, 5); err != nil {
		t.Fatal(err)
	}
	// Burn the IP quota (5). a and c already used 2.
	for i, id := range []string{"d", "e", "f"} {
		r := mk(id)
		r.Host = "h" + string(rune('a'+i))
		if err := s.CreateRun(ctx, r, 3, 5); err != nil {
			t.Fatal(err)
		}
	}
	r := mk("g")
	r.Host = "fresh.example"
	if err := s.CreateRun(ctx, r, 3, 5); err != store.ErrQuota {
		t.Fatalf("ip quota: %v", err)
	}
}

func TestMergeSamples(t *testing.T) {
	s := New()
	ctx := context.Background()
	_ = s.MergeSample(ctx, store.Sample{RunID: "r", Sec: 1, Workers: 5, Requests: 4, Errors: 1, LatencyMS: []float64{10, 20, 30, 40}})
	_ = s.MergeSample(ctx, store.Sample{RunID: "r", Sec: 1, Workers: 5, Requests: 4, Errors: 0, LatencyMS: []float64{15, 25, 35, 45}})
	got, err := s.ListSamples(ctx, "r")
	if err != nil || len(got) != 1 {
		t.Fatal(err, len(got))
	}
	if got[0].Workers != 10 || got[0].Requests != 8 || got[0].Errors != 1 {
		t.Fatalf("%+v", got[0])
	}
	if got[0].RPS != 8 || got[0].P95 <= got[0].P50 {
		t.Fatalf("stats %+v", got[0])
	}
}

func TestKillAndVerify(t *testing.T) {
	s := New()
	ctx := context.Background()
	on, err := s.KillEnabled(ctx)
	if err != nil || on {
		t.Fatal(on, err)
	}
	if err := s.SetKill(ctx, true); err != nil {
		t.Fatal(err)
	}
	on, _ = s.KillEnabled(ctx)
	if !on {
		t.Fatal("expected kill")
	}
	now := time.Now().UTC()
	if err := s.CreateChallenge(ctx, store.Challenge{ID: "c", Host: "example.com", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkVerified(ctx, "c", "file", now); err != nil {
		t.Fatal(err)
	}
	ch, err := s.GetChallenge(ctx, "c")
	if err != nil || !ch.Verified || ch.Method != "file" {
		t.Fatalf("%+v %v", ch, err)
	}
	if !ch.ExpiresAt.After(now.Add(23 * time.Hour)) {
		t.Fatalf("expiry not extended: %s", ch.ExpiresAt)
	}
}
