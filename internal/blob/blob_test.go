package blob

import (
	"context"
	"testing"
)

func TestLocalRoundTripAndTraversal(t *testing.T) {
	l := Local{Dir: t.TempDir()}
	ctx := context.Background()
	if err := l.Put(ctx, "runs/abc/badge.svg", []byte("<svg></svg>"), "image/svg+xml"); err != nil {
		t.Fatal(err)
	}
	b, ct, err := l.Get(ctx, "runs/abc/badge.svg")
	if err != nil || string(b) != "<svg></svg>" || ct != "image/svg+xml" {
		t.Fatalf("%s %s %v", b, ct, err)
	}
	if _, _, err := l.Get(ctx, "../secret"); err == nil {
		t.Fatal("traversal should fail")
	}
}
