// Package blob stores report JSON and badge SVG.
// Local disk is the default. Cloud Storage is used when a bucket is configured.
package blob

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
)

// Store saves named objects.
type Store interface {
	Put(ctx context.Context, name string, data []byte, contentType string) error
	Get(ctx context.Context, name string) ([]byte, string, error)
}

// Local is a directory tree.
type Local struct {
	Dir string
}

func (l Local) Put(_ context.Context, name string, data []byte, contentType string) error {
	path, err := safe(l.Dir, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	if contentType != "" {
		return os.WriteFile(path+".ctype", []byte(contentType), 0o644)
	}
	return nil
}

func (l Local) Get(_ context.Context, name string) ([]byte, string, error) {
	path, err := safe(l.Dir, name)
	if err != nil {
		return nil, "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", os.ErrNotExist
		}
		return nil, "", err
	}
	ct, _ := os.ReadFile(path + ".ctype")
	return b, strings.TrimSpace(string(ct)), nil
}

func safe(dir, name string) (string, error) {
	if name == "" || strings.Contains(name, "..") {
		return "", errors.New("invalid object name")
	}
	clean := filepath.Clean(name)
	if filepath.IsAbs(clean) {
		return "", errors.New("invalid object name")
	}
	full := filepath.Join(dir, clean)
	rel, err := filepath.Rel(dir, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", errors.New("invalid object name")
	}
	return full, nil
}

// GCS is a private Cloud Storage bucket.
type GCS struct {
	client *storage.Client
	bucket string
}

// OpenGCS uses application default credentials.
func OpenGCS(ctx context.Context, bucket string) (*GCS, error) {
	if bucket == "" {
		return nil, errors.New("bucket is required")
	}
	c, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &GCS{client: c, bucket: bucket}, nil
}

func (g *GCS) Close() error { return g.client.Close() }

func (g *GCS) Put(ctx context.Context, name string, data []byte, contentType string) error {
	w := g.client.Bucket(g.bucket).Object(name).NewWriter(ctx)
	w.ContentType = contentType
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

func (g *GCS) Get(ctx context.Context, name string) ([]byte, string, error) {
	r, err := g.client.Bucket(g.bucket).Object(name).NewReader(ctx)
	if err != nil {
		return nil, "", err
	}
	defer r.Close()
	b, err := io.ReadAll(io.LimitReader(r, 4<<20))
	if err != nil {
		return nil, "", err
	}
	return b, r.Attrs.ContentType, nil
}
