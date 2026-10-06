// Package sqlite persists Stampede in a single file for local runs and CI.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/icohangar-ops/stampede/internal/store"

	_ "modernc.org/sqlite"
)

// Store is a SQLite-backed store. One connection avoids SQLITE_BUSY.
type Store struct {
	db *sql.DB
}

// Open creates the file and schema.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		if filepath.Dir(path) != "." {
			return nil, err
		}
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS challenges (
  id TEXT PRIMARY KEY,
  url TEXT NOT NULL,
  host TEXT NOT NULL,
  token TEXT NOT NULL,
  verified INTEGER NOT NULL,
  method TEXT,
  client_ip TEXT,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  verified_at INTEGER
);
CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  challenge_id TEXT,
  url TEXT,
  host TEXT,
  preset TEXT,
  client_ip TEXT,
  status TEXT,
  started_at INTEGER,
  ended_at INTEGER,
  max_rps REAL,
  duration_ms INTEGER,
  has_breaking INTEGER,
  breaking_rps REAL,
  peak_rps REAL,
  peak_p95 REAL,
  peak_error REAL,
  verdict TEXT,
  report_json TEXT,
  probe_json TEXT,
  opt_in INTEGER,
  error TEXT,
  task_count INTEGER,
  tasks_done INTEGER
);
CREATE TABLE IF NOT EXISTS samples (
  run_id TEXT NOT NULL,
  sec INTEGER NOT NULL,
  workers INTEGER,
  requests INTEGER,
  errors INTEGER,
  latency_json TEXT,
  PRIMARY KEY (run_id, sec)
);
CREATE TABLE IF NOT EXISTS quota (
  subject TEXT NOT NULL,
  day TEXT NOT NULL,
  count INTEGER NOT NULL,
  PRIMARY KEY (subject, day)
);
CREATE TABLE IF NOT EXISTS hosts (
  host TEXT PRIMARY KEY,
  url TEXT,
  opt_in INTEGER,
  verified_at INTEGER,
  challenge_id TEXT
);
CREATE TABLE IF NOT EXISTS kv (
  k TEXT PRIMARY KEY,
  v TEXT
);
`)
	return err
}

func ms(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func fromMS(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.UnixMilli(v).UTC()
}

func (s *Store) CreateChallenge(_ context.Context, c store.Challenge) error {
	_, err := s.db.Exec(`INSERT INTO challenges
		(id, url, host, token, verified, method, client_ip, created_at, expires_at, verified_at)
		VALUES (?, ?, ?, ?, 0, '', ?, ?, ?, 0)`,
		c.ID, c.URL, c.Host, c.Token, c.ClientIP, ms(c.CreatedAt), ms(c.ExpiresAt))
	return err
}

func (s *Store) GetChallenge(_ context.Context, id string) (store.Challenge, error) {
	var c store.Challenge
	var verified int
	var created, expires, verifiedAt int64
	err := s.db.QueryRow(`SELECT id, url, host, token, verified, method, client_ip, created_at, expires_at, verified_at
		FROM challenges WHERE id = ?`, id).Scan(
		&c.ID, &c.URL, &c.Host, &c.Token, &verified, &c.Method, &c.ClientIP, &created, &expires, &verifiedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Challenge{}, store.ErrNotFound
	}
	if err != nil {
		return store.Challenge{}, err
	}
	c.Verified = verified == 1
	c.CreatedAt = fromMS(created)
	c.ExpiresAt = fromMS(expires)
	c.VerifiedAt = fromMS(verifiedAt)
	return c, nil
}

func (s *Store) MarkVerified(_ context.Context, id, method string, at time.Time) error {
	res, err := s.db.Exec(`UPDATE challenges SET verified = 1, method = ?, verified_at = ?, expires_at = ? WHERE id = ?`,
		method, ms(at), ms(at.Add(24*time.Hour)), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) CountChallenges(_ context.Context, ip string, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM challenges WHERE client_ip = ? AND created_at >= ?`, ip, ms(since)).Scan(&n)
	return n, err
}

func (s *Store) CreateRun(_ context.Context, r store.Run, domainQuota, ipQuota int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	day := store.DayKey(r.StartedAt)
	hk := "host:" + r.Host
	if err := bump(tx, hk, day, domainQuota); err != nil {
		return err
	}
	if r.ClientIP != "" {
		if err := bump(tx, "ip:"+r.ClientIP, day, ipQuota); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`INSERT INTO runs (
		id, challenge_id, url, host, preset, client_ip, status, started_at, ended_at,
		max_rps, duration_ms, has_breaking, breaking_rps, peak_rps, peak_p95, peak_error,
		verdict, report_json, probe_json, opt_in, error, task_count, tasks_done
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ChallengeID, r.URL, r.Host, r.Preset, r.ClientIP, r.Status, ms(r.StartedAt), ms(r.EndedAt),
		r.MaxRPS, r.Duration.Milliseconds(), boolInt(r.HasBreaking), r.BreakingRPS, r.PeakRPS, r.PeakP95, r.PeakError,
		r.Verdict, r.ReportJSON, r.ProbeJSON, boolInt(r.OptIn), r.Error, r.TaskCount, r.TasksDone)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func bump(tx *sql.Tx, subject, day string, quota int) error {
	var n int
	err := tx.QueryRow(`SELECT count FROM quota WHERE subject = ? AND day = ?`, subject, day).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		n = 0
		err = nil
	}
	if err != nil {
		return err
	}
	if quota > 0 && n >= quota {
		return store.ErrQuota
	}
	_, err = tx.Exec(`INSERT INTO quota (subject, day, count) VALUES (?, ?, 1)
		ON CONFLICT(subject, day) DO UPDATE SET count = count + 1`, subject, day)
	return err
}

func (s *Store) GetRun(_ context.Context, id string) (store.Run, error) {
	row := s.db.QueryRow(`SELECT id, challenge_id, url, host, preset, client_ip, status, started_at, ended_at,
		max_rps, duration_ms, has_breaking, breaking_rps, peak_rps, peak_p95, peak_error,
		verdict, report_json, probe_json, opt_in, error, task_count, tasks_done FROM runs WHERE id = ?`, id)
	return scanRun(row)
}

func scanRun(row interface{ Scan(...any) error }) (store.Run, error) {
	var r store.Run
	var started, ended, dur int64
	var hasBreaking, optIn int
	err := row.Scan(&r.ID, &r.ChallengeID, &r.URL, &r.Host, &r.Preset, &r.ClientIP, &r.Status, &started, &ended,
		&r.MaxRPS, &dur, &hasBreaking, &r.BreakingRPS, &r.PeakRPS, &r.PeakP95, &r.PeakError,
		&r.Verdict, &r.ReportJSON, &r.ProbeJSON, &optIn, &r.Error, &r.TaskCount, &r.TasksDone)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Run{}, store.ErrNotFound
	}
	if err != nil {
		return store.Run{}, err
	}
	r.StartedAt = fromMS(started)
	r.EndedAt = fromMS(ended)
	r.Duration = time.Duration(dur) * time.Millisecond
	r.HasBreaking = hasBreaking == 1
	r.OptIn = optIn == 1
	return r, nil
}

func (s *Store) SaveProbe(_ context.Context, runID, probeJSON string) error {
	res, err := s.db.Exec(`UPDATE runs SET probe_json = ? WHERE id = ? AND (probe_json IS NULL OR probe_json = '')`, probeJSON, runID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		if _, err := s.GetRun(context.Background(), runID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpdateRun(_ context.Context, r store.Run) error {
	res, err := s.db.Exec(`UPDATE runs SET status=?, ended_at=?, has_breaking=?, breaking_rps=?, peak_rps=?,
		peak_p95=?, peak_error=?, verdict=?, report_json=?, probe_json=?, opt_in=?, error=?, task_count=?, tasks_done=?
		WHERE id=?`,
		r.Status, ms(r.EndedAt), boolInt(r.HasBreaking), r.BreakingRPS, r.PeakRPS, r.PeakP95, r.PeakError,
		r.Verdict, r.ReportJSON, r.ProbeJSON, boolInt(r.OptIn), r.Error, r.TaskCount, r.TasksDone, r.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) MergeSample(_ context.Context, in store.Sample) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var workers, requests, errorsN int
	var latJSON string
	err = tx.QueryRow(`SELECT workers, requests, errors, latency_json FROM samples WHERE run_id=? AND sec=?`, in.RunID, in.Sec).
		Scan(&workers, &requests, &errorsN, &latJSON)
	cur := store.Sample{RunID: in.RunID, Sec: in.Sec, Workers: workers, Requests: requests, Errors: errorsN}
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	} else if err != nil {
		return err
	} else if latJSON != "" {
		_ = json.Unmarshal([]byte(latJSON), &cur.LatencyMS)
	}
	cur.Workers += in.Workers
	cur.Requests += in.Requests
	cur.Errors += in.Errors
	cur.LatencyMS = append(cur.LatencyMS, in.LatencyMS...)
	// Cap stored latencies so a long run cannot bloat the row.
	if len(cur.LatencyMS) > 400 {
		cur.LatencyMS = cur.LatencyMS[len(cur.LatencyMS)-400:]
	}
	store.ApplyLatencies(&cur)
	raw, _ := json.Marshal(cur.LatencyMS)
	_, err = tx.Exec(`INSERT INTO samples (run_id, sec, workers, requests, errors, latency_json)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id, sec) DO UPDATE SET
		  workers=excluded.workers, requests=excluded.requests, errors=excluded.errors, latency_json=excluded.latency_json`,
		cur.RunID, cur.Sec, cur.Workers, cur.Requests, cur.Errors, string(raw))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListSamples(_ context.Context, runID string) ([]store.Sample, error) {
	rows, err := s.db.Query(`SELECT sec, workers, requests, errors, latency_json FROM samples WHERE run_id=? ORDER BY sec`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.Sample
	for rows.Next() {
		var sm store.Sample
		var lat string
		sm.RunID = runID
		if err := rows.Scan(&sm.Sec, &sm.Workers, &sm.Requests, &sm.Errors, &lat); err != nil {
			return nil, err
		}
		if lat != "" {
			_ = json.Unmarshal([]byte(lat), &sm.LatencyMS)
		}
		store.ApplyLatencies(&sm)
		out = append(out, sm)
	}
	return out, rows.Err()
}

func (s *Store) MarkTaskDone(_ context.Context, runID string) (int, int, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, false, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE runs SET tasks_done = tasks_done + 1 WHERE id = ?`, runID)
	if err != nil {
		return 0, 0, false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return 0, 0, false, store.ErrNotFound
	}
	var done, total int
	if err := tx.QueryRow(`SELECT tasks_done, task_count FROM runs WHERE id = ?`, runID).Scan(&done, &total); err != nil {
		return 0, 0, false, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, false, err
	}
	if total < 1 {
		total = 1
	}
	return done, total, done >= total, nil
}

func (s *Store) SetKill(_ context.Context, on bool) error {
	v := "0"
	if on {
		v = "1"
	}
	_, err := s.db.Exec(`INSERT INTO kv (k, v) VALUES ('kill', ?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, v)
	return err
}

func (s *Store) KillEnabled(_ context.Context) (bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT v FROM kv WHERE k='kill'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return v == "1", err
}

func (s *Store) UpsertHost(_ context.Context, h store.Host) error {
	_, err := s.db.Exec(`INSERT INTO hosts (host, url, opt_in, verified_at, challenge_id) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(host) DO UPDATE SET url=excluded.url, opt_in=excluded.opt_in, verified_at=excluded.verified_at, challenge_id=excluded.challenge_id`,
		h.Host, h.URL, boolInt(h.OptIn), ms(h.VerifiedAt), h.ChallengeID)
	return err
}

func (s *Store) ListOptIn(_ context.Context, verifiedAfter time.Time) ([]store.Host, error) {
	rows, err := s.db.Query(`SELECT host, url, opt_in, verified_at, challenge_id FROM hosts WHERE opt_in=1 AND verified_at >= ?`, ms(verifiedAfter))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.Host
	for rows.Next() {
		var h store.Host
		var opt int
		var at int64
		if err := rows.Scan(&h.Host, &h.URL, &opt, &at, &h.ChallengeID); err != nil {
			return nil, err
		}
		h.OptIn = opt == 1
		h.VerifiedAt = fromMS(at)
		out = append(out, h)
	}
	return out, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Ensure the package fails to compile if the interface drifts.
var _ store.Store = (*Store)(nil)

func init() {
	_ = fmt.Sprintf
}
