package metrics

import (
	"testing"

	"github.com/icohangar-ops/stampede/internal/store"
)

func TestBreakingPoint(t *testing.T) {
	clean := []store.Sample{
		{Sec: 0, Requests: 10, RPS: 10, P95: 40, ErrorRate: 0},
		{Sec: 1, Requests: 20, RPS: 20, P95: 80, ErrorRate: 0.01},
	}
	if got := Summarize(clean); got.HasBreaking {
		t.Fatalf("clean run broke: %+v", got)
	}
	byErr := append([]store.Sample{}, clean...)
	byErr = append(byErr, store.Sample{Sec: 2, Requests: 30, RPS: 30, P95: 100, ErrorRate: 0.2})
	got := Summarize(byErr)
	if !got.HasBreaking || got.BreakingRPS != 30 {
		t.Fatalf("error break: %+v", got)
	}
	byLat := []store.Sample{
		{Sec: 0, Requests: 10, RPS: 12, P95: 100, ErrorRate: 0},
		{Sec: 1, Requests: 10, RPS: 26, P95: 1600, ErrorRate: 0.01},
	}
	got = Summarize(byLat)
	if !got.HasBreaking || got.BreakingRPS != 26 {
		t.Fatalf("latency break: %+v", got)
	}
	empty := Summarize(nil)
	if empty.HasBreaking {
		t.Fatal(empty)
	}
}
