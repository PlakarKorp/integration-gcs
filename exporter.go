package gcs

import (
	"context"
	"fmt"
	"io"
	"strings"
	"path"

	"cloud.google.com/go/storage"
	"github.com/PlakarKorp/kloset/objects"
	"github.com/PlakarKorp/kloset/snapshot/exporter"
)

type gcsExporter struct {
	ctx        context.Context
	bucketName string
	path       string

	client *storage.Client
	bucket *storage.BucketHandle
}

func NewExporter(ctx context.Context, opts *exporter.Options, proto string, params map[string]string) (exporter.Exporter, error) {
	target := params["location"]
	bucket, path, _ := strings.Cut(strings.TrimPrefix(target, proto+"://"), "/")

	path = strings.Trim(path, "/")

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create a GCS client: %w", err)
	}

	return &gcsExporter{
		ctx:        ctx,
		bucketName: bucket,
		path:       path,

		client: client,
		bucket: client.Bucket(bucket),
	}, nil
}

func (g *gcsExporter) realpath(rel string) string { return path.Join(g.path, rel) }

func (g *gcsExporter) Root() string { return g.path }

func (g *gcsExporter) CreateDirectory(pathname string) error { return nil }

func (g *gcsExporter) StoreFile(pathname string, fp io.Reader, size int64) error {
	pathname = g.realpath(strings.TrimLeft(pathname, "/"))

	w := g.bucket.Object(pathname).NewWriter(g.ctx)
	if _, err := io.Copy(w, fp); err != nil {
		return err
	}
	return w.Close()
}

func (g *gcsExporter) SetPermissions(pathname string, fileinfo *objects.FileInfo) error { return nil }

func (g *gcsExporter) Close() error {
	if g.client != nil {
		return g.client.Close()
	}
	return nil
}
