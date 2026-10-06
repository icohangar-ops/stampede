package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Vertex calls Gemini on Vertex AI. No GPUs. Text only.
type Vertex struct {
	Project  string
	Location string
	Model    string
	Client   *http.Client
	Token    func(ctx context.Context) (string, error)
}

func (v Vertex) Name() string {
	if v.Model == "" {
		return "gemini"
	}
	return v.Model
}

func (v Vertex) Complete(ctx context.Context, prompt string) (string, error) {
	if v.Project == "" {
		return "", fmt.Errorf("vertex project is not configured")
	}
	loc := v.Location
	if loc == "" {
		loc = "us-central1"
	}
	model := v.Model
	if model == "" {
		model = "gemini-2.5-flash"
	}
	endpoint := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
		loc, v.Project, loc, model)
	payload := map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]string{{"text": prompt}}},
		},
		"generationConfig": map[string]any{
			"temperature":      0.2,
			"maxOutputTokens":  800,
			"responseMimeType": "application/json",
		},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	tok, err := v.token(ctx)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	client := v.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("vertex status %d", resp.StatusCode)
	}
	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("vertex returned no text")
	}
	return parsed.Candidates[0].Content.Parts[0].Text, nil
}

func (v Vertex) token(ctx context.Context) (string, error) {
	if v.Token != nil {
		return v.Token(ctx)
	}
	ts, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", err
	}
	tok, err := ts.Token()
	if err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

// Ensure oauth2 is referenced if a caller wants to inject a source later.
var _ = oauth2.Token{}

func Usable(project string) bool { return strings.TrimSpace(project) != "" }
