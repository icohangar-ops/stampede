package api

import (
	"bytes"
	"context"
	"io"

	"github.com/icohangar-ops/stampede/internal/gcpauth"
)

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

func identity(ctx context.Context, audience string) (string, error) {
	return gcpauth.Identity(ctx, audience)
}
