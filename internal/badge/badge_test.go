package badge

import (
	"strings"
	"testing"
)

func TestBadgeLabels(t *testing.T) {
	ready := SVG("completed", "launch_ready", 80, false)
	if !strings.Contains(ready, "Launch-ready") || !strings.Contains(ready, "p95 80 ms") {
		t.Fatal(ready)
	}
	needs := SVG("completed", "fails", 1600, true)
	if strings.Contains(needs, "Launch-ready") || !strings.Contains(needs, "Needs work") {
		t.Fatal(needs)
	}
	running := SVG("running", "launch_ready", 10, false)
	if strings.Contains(running, "Launch-ready") || !strings.Contains(running, "In progress") {
		t.Fatal(running)
	}
	failed := SVG("failed", "", 0, false)
	if strings.Contains(failed, "Launch-ready") || !strings.Contains(failed, "Run failed") {
		t.Fatal(failed)
	}
}
