// Package memory is an in-process store for tests and single-process demos.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/icohangar-ops/stampede/internal/store"
)

// Store keeps everything in maps guarded by one mutex.
type Store struct {
	mu         sync.Mutex
	challenges map[string]store.Challenge
	runs       map[string]store.Run
	samples    map[string]map[int]store.Sample
	quota      map[string]int
	hosts      map[string]store.Host
	kill       bool
}

func New() *Store {
	return &Store{
		challenges: map[string]store.Challenge{},
		runs:       map[string]store.Run{},
		samples:    map[string]map[int]store.Sample{},
		quota:      map[string]int{},
		hosts:      map[string]store.Host{},
	}
}

func (s *Store) CreateChallenge(_ context.Context, c store.Challenge) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.challenges[c.ID] = c
	return nil
}

func (s *Store) GetChallenge(_ context.Context, id string) (store.Challenge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.challenges[id]
	if !ok {
		return store.Challenge{}, store.ErrNotFound
	}
	return c, nil
}

func (s *Store) MarkVerified(_ context.Context, id, method string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.challenges[id]
	if !ok {
		return store.ErrNotFound
	}
	c.Verified = true
	c.Method = method
	c.VerifiedAt = at
	c.ExpiresAt = at.Add(24 * time.Hour)
	s.challenges[id] = c
	return nil
}

func (s *Store) CountChallenges(_ context.Context, ip string, since time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.challenges {
		if c.ClientIP == ip && !c.CreatedAt.Before(since) {
			n++
		}
	}
	return n, nil
}

func (s *Store) CreateRun(_ context.Context, r store.Run, domainQuota, ipQuota int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	day := store.DayKey(r.StartedAt)
	hk := "host:" + r.Host + "|" + day
	ik := "ip:" + r.ClientIP + "|" + day
	if domainQuota > 0 && s.quota[hk] >= domainQuota {
		return store.ErrQuota
	}
	if r.ClientIP != "" && ipQuota > 0 && s.quota[ik] >= ipQuota {
		return store.ErrQuota
	}
	if _, ok := s.runs[r.ID]; ok {
		return store.ErrQuota
	}
	s.quota[hk]++
	if r.ClientIP != "" {
		s.quota[ik]++
	}
	s.runs[r.ID] = r
	return nil
}

func (s *Store) GetRun(_ context.Context, id string) (store.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return store.Run{}, store.ErrNotFound
	}
	return r, nil
}

func (s *Store) SaveProbe(_ context.Context, runID, probeJSON string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runID]
	if !ok {
		return store.ErrNotFound
	}
	if r.ProbeJSON == "" {
		r.ProbeJSON = probeJSON
		s.runs[runID] = r
	}
	return nil
}

func (s *Store) UpdateRun(_ context.Context, r store.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[r.ID]; !ok {
		return store.ErrNotFound
	}
	s.runs[r.ID] = r
	return nil
}

func (s *Store) MergeSample(_ context.Context, in store.Sample) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	bySec, ok := s.samples[in.RunID]
	if !ok {
		bySec = map[int]store.Sample{}
		s.samples[in.RunID] = bySec
	}
	cur, exists := bySec[in.Sec]
	if !exists {
		cur = store.Sample{RunID: in.RunID, Sec: in.Sec}
	}
	cur.Workers += in.Workers
	cur.Requests += in.Requests
	cur.Errors += in.Errors
	cur.LatencyMS = append(cur.LatencyMS, in.LatencyMS...)
	store.ApplyLatencies(&cur)
	bySec[in.Sec] = cur
	return nil
}

func (s *Store) ListSamples(_ context.Context, runID string) ([]store.Sample, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bySec := s.samples[runID]
	out := make([]store.Sample, 0, len(bySec))
	for _, sm := range bySec {
		cp := sm
		cp.LatencyMS = append([]float64(nil), sm.LatencyMS...)
		out = append(out, cp)
	}
	sortSamples(out)
	return out, nil
}

func (s *Store) MarkTaskDone(_ context.Context, runID string) (int, int, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runID]
	if !ok {
		return 0, 0, false, store.ErrNotFound
	}
	r.TasksDone++
	s.runs[runID] = r
	total := r.TaskCount
	if total < 1 {
		total = 1
	}
	return r.TasksDone, total, r.TasksDone >= total, nil
}

func (s *Store) SetKill(_ context.Context, on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kill = on
	return nil
}

func (s *Store) KillEnabled(_ context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.kill, nil
}

func (s *Store) UpsertHost(_ context.Context, h store.Host) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hosts[h.Host] = h
	return nil
}

func (s *Store) ListOptIn(_ context.Context, verifiedAfter time.Time) ([]store.Host, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.Host
	for _, h := range s.hosts {
		if h.OptIn && !h.VerifiedAt.Before(verifiedAfter) {
			out = append(out, h)
		}
	}
	return out, nil
}

func sortSamples(s []store.Sample) {
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && s[j].Sec < s[j-1].Sec {
			s[j], s[j-1] = s[j-1], s[j]
			j--
		}
	}
}
