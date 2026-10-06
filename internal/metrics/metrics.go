// Package metrics turns per-second samples into the numbers the report quotes.
package metrics

import "github.com/icohangar-ops/stampede/internal/store"

const (
	// ErrorBreak is the error-rate threshold that marks a breaking point.
	ErrorBreak = 0.05
	// P95BreakMS is the latency threshold that marks a breaking point.
	P95BreakMS = 1500
)

// Summary is the measured shape of a run.
type Summary struct {
	HasBreaking bool    `json:"has_breaking"`
	BreakingRPS float64 `json:"breaking_rps"`
	PeakRPS     float64 `json:"peak_rps"`
	PeakP95     float64 `json:"peak_p95_ms"`
	PeakError   float64 `json:"peak_error_rate"`
	Requests    int     `json:"requests"`
	Errors      int     `json:"errors"`
	// BreakReason is "error" or "latency" when HasBreaking is set.
	BreakReason string `json:"break_reason,omitempty"`
}

// Summarize finds the breaking point and the peaks.
// Breaking point is the requests/second of the earliest sample that exceeds
// either threshold. Samples with no requests are ignored.
func Summarize(samples []store.Sample) Summary {
	var s Summary
	for _, sm := range samples {
		if sm.RPS > s.PeakRPS {
			s.PeakRPS = sm.RPS
		}
		if sm.P95 > s.PeakP95 {
			s.PeakP95 = sm.P95
		}
		if sm.ErrorRate > s.PeakError {
			s.PeakError = sm.ErrorRate
		}
		s.Requests += sm.Requests
		s.Errors += sm.Errors
		if s.HasBreaking || sm.Requests == 0 {
			continue
		}
		if sm.ErrorRate > ErrorBreak || sm.P95 > P95BreakMS {
			s.HasBreaking = true
			s.BreakingRPS = sm.RPS
			if sm.ErrorRate > ErrorBreak {
				s.BreakReason = "error"
			} else {
				s.BreakReason = "latency"
			}
		}
	}
	return s
}
