package firestore

import (
	"context"
	"testing"
)

func TestOpenRequiresProject(t *testing.T) {
	_, err := Open(context.Background(), "")
	if err == nil {
		t.Fatal("empty project should fail before any network call")
	}
}
