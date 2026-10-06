// Package launch starts the Cloud Run job that hosts the parallel load tasks.
package launch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Launcher starts load-generator work for a run.
type Launcher interface {
	Launch(ctx context.Context, runID string) error
	Tasks() int
}

// CloudRun calls the Cloud Run jobs API.
type CloudRun struct {
	Project  string
	Region   string
	Job      string
	TasksN   int
	Token    func(ctx context.Context) (string, error)
	Client   *http.Client
	Endpoint string // optional override for tests
}

func (c CloudRun) Tasks() int {
	if c.TasksN < 1 {
		return 1
	}
	if c.TasksN > 10 {
		return 10
	}
	return c.TasksN
}

func (c CloudRun) Launch(ctx context.Context, runID string) error {
	if c.Project == "" || c.Region == "" || c.Job == "" {
		return fmt.Errorf("cloud run job is not configured")
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://run.googleapis.com/v2/projects/%s/locations/%s/jobs/%s:run", c.Project, c.Region, c.Job)
	}
	payload := map[string]any{
		"overrides": map[string]any{
			"taskCount": c.Tasks(),
			"containerOverrides": []map[string]any{
				{"env": []map[string]string{
					{"name": "RUN_ID", "value": runID},
				}},
			},
		},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != nil {
		tok, err := c.Token(ctx)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("cloud run job status %d: %s", resp.StatusCode, truncate(raw))
	}
	return nil
}

func truncate(b []byte) string {
	if len(b) > 300 {
		b = b[:300]
	}
	return string(b)
}
