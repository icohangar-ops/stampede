package launch

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLaunchPostsRunOverride(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		if !strings.HasSuffix(r.URL.Path, ":run") {
			t.Errorf("path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"executions/1"}`))
	}))
	defer srv.Close()
	c := CloudRun{
		Project: "p", Region: "us-central1", Job: "stampede-loadgen", TasksN: 50,
		Endpoint: srv.URL + "/v2/projects/p/locations/us-central1/jobs/stampede-loadgen:run",
		Token:    func(context.Context) (string, error) { return "ya29.test", nil },
	}
	if c.Tasks() != 10 {
		t.Fatalf("task cap %d", c.Tasks())
	}
	if err := c.Launch(context.Background(), "run123"); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer ya29.test" || !strings.Contains(gotBody, "RUN_ID") || !strings.Contains(gotBody, "run123") || !strings.Contains(gotBody, `"taskCount":10`) {
		t.Fatalf("auth %s body %s", gotAuth, gotBody)
	}
}
