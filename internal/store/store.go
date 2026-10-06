// Package store is the persistence boundary for challenges, runs, and samples.
// SQLite backs local and CI. Firestore backs Cloud Run. Tests use memory.
package store

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
	ErrQuota    = errors.New("daily quota exceeded")
)

// Challenge is an ownership proof waiting on a file or a TXT record.
type Challenge struct {
	ID         string
	URL        string
	Host       string
	Token      string
	Verified   bool
	Method     string
	ClientIP   string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	VerifiedAt time.Time
}

// Run is one load test.
type Run struct {
	ID          string
	ChallengeID string
	URL         string
	Host        string
	Preset      string
	ClientIP    string
	Status      string
	StartedAt   time.Time
	EndedAt     time.Time
	MaxRPS      float64
	Duration    time.Duration
	HasBreaking bool
	BreakingRPS float64
	PeakRPS     float64
	PeakP95     float64
	PeakError   float64
	Verdict     string
	ReportJSON  string
	ProbeJSON   string
	OptIn       bool
	Error       string
	TaskCount   int
	TasksDone   int
}

// Sample is one second of aggregated load.
type Sample struct {
	RunID     string
	Sec       int
	Workers   int
	RPS       float64
	P50       float64
	P95       float64
	ErrorRate float64
	Requests  int
	Errors    int
	LatencyMS []float64
}

// Host is an opt-in retest registration.
type Host struct {
	Host        string
	URL         string
	OptIn       bool
	VerifiedAt  time.Time
	ChallengeID string
}

// Store is the product's memory.
type Store interface {
	CreateChallenge(ctx context.Context, c Challenge) error
	GetChallenge(ctx context.Context, id string) (Challenge, error)
	MarkVerified(ctx context.Context, id, method string, at time.Time) error
	CountChallenges(ctx context.Context, ip string, since time.Time) (int, error)
	CreateRun(ctx context.Context, r Run, domainQuota, ipQuota int) error
	GetRun(ctx context.Context, id string) (Run, error)
	UpdateRun(ctx context.Context, r Run) error
	SaveProbe(ctx context.Context, runID, probeJSON string) error
	MergeSample(ctx context.Context, s Sample) error
	ListSamples(ctx context.Context, runID string) ([]Sample, error)
	// MarkTaskDone increments the completed-task counter and reports whether every task has checked in.
	MarkTaskDone(ctx context.Context, runID string) (done, total int, finished bool, err error)
	SetKill(ctx context.Context, on bool) error
	KillEnabled(ctx context.Context) (bool, error)
	UpsertHost(ctx context.Context, h Host) error
	ListOptIn(ctx context.Context, verifiedAfter time.Time) ([]Host, error)
}

// ApplyLatencies recomputes percentiles, error rate, and RPS from raw counts.
func ApplyLatencies(s *Sample) {
	if len(s.LatencyMS) > 1 {
		sort.Float64s(s.LatencyMS)
	}
	s.P50 = percentile(s.LatencyMS, 0.50)
	s.P95 = percentile(s.LatencyMS, 0.95)
	if s.Requests > 0 {
		s.ErrorRate = float64(s.Errors) / float64(s.Requests)
		s.RPS = float64(s.Requests)
	} else {
		s.ErrorRate = 0
		s.RPS = 0
	}
}

func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	idx := int(math.Ceil(p*float64(n))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}

// DayKey is the UTC day used for quotas.
func DayKey(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return t.UTC().Format("2006-01-02")
}
