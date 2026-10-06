// Package firestore persists Stampede in Cloud Firestore (Native mode).
// Local runs and CI use SQLite instead. This implementation is selected when
// FIRESTORE_PROJECT is set.
package firestore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/icohangar-ops/stampede/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Store is a Firestore-backed store.
type Store struct {
	c *firestore.Client
}

// Open dials Firestore with application default credentials.
func Open(ctx context.Context, projectID string) (*Store, error) {
	if projectID == "" {
		return nil, errors.New("firestore project id is required")
	}
	c, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return &Store{c: c}, nil
}

func (s *Store) Close() error { return s.c.Close() }

type challengeDoc struct {
	URL        string    `firestore:"url"`
	Host       string    `firestore:"host"`
	Token      string    `firestore:"token"`
	Verified   bool      `firestore:"verified"`
	Method     string    `firestore:"method"`
	ClientIP   string    `firestore:"client_ip"`
	CreatedAt  time.Time `firestore:"created_at"`
	ExpiresAt  time.Time `firestore:"expires_at"`
	VerifiedAt time.Time `firestore:"verified_at"`
}

type runDoc struct {
	ChallengeID string    `firestore:"challenge_id"`
	URL         string    `firestore:"url"`
	Host        string    `firestore:"host"`
	Preset      string    `firestore:"preset"`
	ClientIP    string    `firestore:"client_ip"`
	Status      string    `firestore:"status"`
	StartedAt   time.Time `firestore:"started_at"`
	EndedAt     time.Time `firestore:"ended_at"`
	MaxRPS      float64   `firestore:"max_rps"`
	DurationMS  int64     `firestore:"duration_ms"`
	HasBreaking bool      `firestore:"has_breaking"`
	BreakingRPS float64   `firestore:"breaking_rps"`
	PeakRPS     float64   `firestore:"peak_rps"`
	PeakP95     float64   `firestore:"peak_p95"`
	PeakError   float64   `firestore:"peak_error"`
	Verdict     string    `firestore:"verdict"`
	ReportJSON  string    `firestore:"report_json"`
	ProbeJSON   string    `firestore:"probe_json"`
	OptIn       bool      `firestore:"opt_in"`
	Error       string    `firestore:"error"`
	TaskCount   int       `firestore:"task_count"`
	TasksDone   int       `firestore:"tasks_done"`
}

type sampleDoc struct {
	Workers   int       `firestore:"workers"`
	Requests  int       `firestore:"requests"`
	Errors    int       `firestore:"errors"`
	LatencyMS []float64 `firestore:"latency_ms"`
}

type hostDoc struct {
	URL         string    `firestore:"url"`
	OptIn       bool      `firestore:"opt_in"`
	VerifiedAt  time.Time `firestore:"verified_at"`
	ChallengeID string    `firestore:"challenge_id"`
}

func (s *Store) CreateChallenge(ctx context.Context, c store.Challenge) error {
	_, err := s.c.Collection("challenges").Doc(c.ID).Set(ctx, challengeDoc{
		URL: c.URL, Host: c.Host, Token: c.Token, Verified: false, Method: "",
		ClientIP: c.ClientIP, CreatedAt: c.CreatedAt, ExpiresAt: c.ExpiresAt,
	})
	return err
}

func (s *Store) GetChallenge(ctx context.Context, id string) (store.Challenge, error) {
	snap, err := s.c.Collection("challenges").Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return store.Challenge{}, store.ErrNotFound
	}
	if err != nil {
		return store.Challenge{}, err
	}
	var d challengeDoc
	if err := snap.DataTo(&d); err != nil {
		return store.Challenge{}, err
	}
	return store.Challenge{
		ID: id, URL: d.URL, Host: d.Host, Token: d.Token, Verified: d.Verified, Method: d.Method,
		ClientIP: d.ClientIP, CreatedAt: d.CreatedAt, ExpiresAt: d.ExpiresAt, VerifiedAt: d.VerifiedAt,
	}, nil
}

func (s *Store) MarkVerified(ctx context.Context, id, method string, at time.Time) error {
	_, err := s.c.Collection("challenges").Doc(id).Update(ctx, []firestore.Update{
		{Path: "verified", Value: true},
		{Path: "method", Value: method},
		{Path: "verified_at", Value: at},
		{Path: "expires_at", Value: at.Add(24 * time.Hour)},
	})
	if status.Code(err) == codes.NotFound {
		return store.ErrNotFound
	}
	return err
}

func (s *Store) CountChallenges(ctx context.Context, ip string, since time.Time) (int, error) {
	q := s.c.Collection("challenges").Where("client_ip", "==", ip).Where("created_at", ">=", since)
	docs, err := q.Documents(ctx).GetAll()
	if err != nil {
		return 0, err
	}
	return len(docs), nil
}

func (s *Store) CreateRun(ctx context.Context, r store.Run, domainQuota, ipQuota int) error {
	day := store.DayKey(r.StartedAt)
	hostRef := s.c.Collection("quotas").Doc("host_" + r.Host + "_" + day)
	var ipRef *firestore.DocumentRef
	if r.ClientIP != "" {
		ipRef = s.c.Collection("quotas").Doc("ip_" + r.ClientIP + "_" + day)
	}
	runRef := s.c.Collection("runs").Doc(r.ID)
	err := s.c.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if err := charge(tx, hostRef, domainQuota); err != nil {
			return err
		}
		if ipRef != nil {
			if err := charge(tx, ipRef, ipQuota); err != nil {
				return err
			}
		}
		return tx.Set(runRef, toRunDoc(r))
	})
	return err
}

func charge(tx *firestore.Transaction, ref *firestore.DocumentRef, quota int) error {
	snap, err := tx.Get(ref)
	n := 0
	if err == nil {
		if v, ok := snap.Data()["count"].(int64); ok {
			n = int(v)
		}
	} else if status.Code(err) != codes.NotFound {
		return err
	}
	if quota > 0 && n >= quota {
		return store.ErrQuota
	}
	return tx.Set(ref, map[string]any{"count": n + 1})
}

func toRunDoc(r store.Run) runDoc {
	return runDoc{
		ChallengeID: r.ChallengeID, URL: r.URL, Host: r.Host, Preset: r.Preset, ClientIP: r.ClientIP,
		Status: r.Status, StartedAt: r.StartedAt, EndedAt: r.EndedAt, MaxRPS: r.MaxRPS,
		DurationMS: r.Duration.Milliseconds(), HasBreaking: r.HasBreaking, BreakingRPS: r.BreakingRPS,
		PeakRPS: r.PeakRPS, PeakP95: r.PeakP95, PeakError: r.PeakError, Verdict: r.Verdict,
		ReportJSON: r.ReportJSON, ProbeJSON: r.ProbeJSON, OptIn: r.OptIn, Error: r.Error,
		TaskCount: r.TaskCount, TasksDone: r.TasksDone,
	}
}

func fromRunDoc(id string, d runDoc) store.Run {
	return store.Run{
		ID: id, ChallengeID: d.ChallengeID, URL: d.URL, Host: d.Host, Preset: d.Preset, ClientIP: d.ClientIP,
		Status: d.Status, StartedAt: d.StartedAt, EndedAt: d.EndedAt, MaxRPS: d.MaxRPS,
		Duration: time.Duration(d.DurationMS) * time.Millisecond, HasBreaking: d.HasBreaking,
		BreakingRPS: d.BreakingRPS, PeakRPS: d.PeakRPS, PeakP95: d.PeakP95, PeakError: d.PeakError,
		Verdict: d.Verdict, ReportJSON: d.ReportJSON, ProbeJSON: d.ProbeJSON, OptIn: d.OptIn, Error: d.Error,
		TaskCount: d.TaskCount, TasksDone: d.TasksDone,
	}
}

func (s *Store) GetRun(ctx context.Context, id string) (store.Run, error) {
	snap, err := s.c.Collection("runs").Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return store.Run{}, store.ErrNotFound
	}
	if err != nil {
		return store.Run{}, err
	}
	var d runDoc
	if err := snap.DataTo(&d); err != nil {
		return store.Run{}, err
	}
	return fromRunDoc(id, d), nil
}

func (s *Store) SaveProbe(ctx context.Context, runID, probeJSON string) error {
	_, err := s.c.Collection("runs").Doc(runID).Update(ctx, []firestore.Update{
		{Path: "probe_json", Value: probeJSON},
	})
	if status.Code(err) == codes.NotFound {
		return store.ErrNotFound
	}
	return err
}

func (s *Store) UpdateRun(ctx context.Context, r store.Run) error {
	_, err := s.c.Collection("runs").Doc(r.ID).Set(ctx, toRunDoc(r))
	if status.Code(err) == codes.NotFound {
		return store.ErrNotFound
	}
	return err
}

func (s *Store) MergeSample(ctx context.Context, in store.Sample) error {
	ref := s.c.Collection("runs").Doc(in.RunID).Collection("samples").Doc(fmt.Sprintf("%04d", in.Sec))
	return s.c.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		cur := store.Sample{RunID: in.RunID, Sec: in.Sec}
		snap, err := tx.Get(ref)
		if err == nil {
			var d sampleDoc
			if err := snap.DataTo(&d); err != nil {
				return err
			}
			cur.Workers = d.Workers
			cur.Requests = d.Requests
			cur.Errors = d.Errors
			cur.LatencyMS = d.LatencyMS
		} else if status.Code(err) != codes.NotFound {
			return err
		}
		cur.Workers += in.Workers
		cur.Requests += in.Requests
		cur.Errors += in.Errors
		cur.LatencyMS = append(cur.LatencyMS, in.LatencyMS...)
		if len(cur.LatencyMS) > 400 {
			cur.LatencyMS = cur.LatencyMS[len(cur.LatencyMS)-400:]
		}
		return tx.Set(ref, sampleDoc{
			Workers: cur.Workers, Requests: cur.Requests, Errors: cur.Errors, LatencyMS: cur.LatencyMS,
		})
	})
}

func (s *Store) ListSamples(ctx context.Context, runID string) ([]store.Sample, error) {
	docs, err := s.c.Collection("runs").Doc(runID).Collection("samples").Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}
	out := make([]store.Sample, 0, len(docs))
	for _, doc := range docs {
		var d sampleDoc
		if err := doc.DataTo(&d); err != nil {
			return nil, err
		}
		var sec int
		_, _ = fmt.Sscanf(doc.Ref.ID, "%d", &sec)
		sm := store.Sample{
			RunID: runID, Sec: sec, Workers: d.Workers, Requests: d.Requests, Errors: d.Errors, LatencyMS: d.LatencyMS,
		}
		store.ApplyLatencies(&sm)
		out = append(out, sm)
	}
	// Insertion order is not guaranteed. Sort by second.
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && out[j].Sec < out[j-1].Sec {
			out[j], out[j-1] = out[j-1], out[j]
			j--
		}
	}
	return out, nil
}

func (s *Store) MarkTaskDone(ctx context.Context, runID string) (int, int, bool, error) {
	ref := s.c.Collection("runs").Doc(runID)
	var done, total int
	err := s.c.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if status.Code(err) == codes.NotFound {
			return store.ErrNotFound
		}
		if err != nil {
			return err
		}
		var d runDoc
		if err := snap.DataTo(&d); err != nil {
			return err
		}
		d.TasksDone++
		done = d.TasksDone
		total = d.TaskCount
		return tx.Set(ref, d)
	})
	if err != nil {
		return 0, 0, false, err
	}
	if total < 1 {
		total = 1
	}
	return done, total, done >= total, nil
}

func (s *Store) SetKill(ctx context.Context, on bool) error {
	_, err := s.c.Collection("meta").Doc("kill").Set(ctx, map[string]any{"enabled": on})
	return err
}

func (s *Store) KillEnabled(ctx context.Context) (bool, error) {
	snap, err := s.c.Collection("meta").Doc("kill").Get(ctx)
	if status.Code(err) == codes.NotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	v, _ := snap.Data()["enabled"].(bool)
	return v, nil
}

func (s *Store) UpsertHost(ctx context.Context, h store.Host) error {
	_, err := s.c.Collection("hosts").Doc(h.Host).Set(ctx, hostDoc{
		URL: h.URL, OptIn: h.OptIn, VerifiedAt: h.VerifiedAt, ChallengeID: h.ChallengeID,
	})
	return err
}

func (s *Store) ListOptIn(ctx context.Context, verifiedAfter time.Time) ([]store.Host, error) {
	docs, err := s.c.Collection("hosts").Where("opt_in", "==", true).Where("verified_at", ">=", verifiedAfter).Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}
	out := make([]store.Host, 0, len(docs))
	for _, doc := range docs {
		var d hostDoc
		if err := doc.DataTo(&d); err != nil {
			return nil, err
		}
		out = append(out, store.Host{
			Host: doc.Ref.ID, URL: d.URL, OptIn: d.OptIn, VerifiedAt: d.VerifiedAt, ChallengeID: d.ChallengeID,
		})
	}
	return out, nil
}

var _ store.Store = (*Store)(nil)
