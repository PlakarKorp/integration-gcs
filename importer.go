package gcs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/PlakarKorp/kloset/objects"
	"github.com/PlakarKorp/kloset/snapshot/importer"
	"google.golang.org/api/iterator"
)

type gcsImporter struct {
	ctx        context.Context
	bucketName string
	path       string
	base       string

	client *storage.Client
	bucket *storage.BucketHandle
}

func NewImporter(ctx context.Context, opts *importer.Options, proto string, params map[string]string) (importer.Importer, error) {
	target := params["location"]
	bucket, path, _ := strings.Cut(strings.TrimPrefix(target, proto+"://"), "/")

	path = strings.TrimLeft(path, "/")

	return &gcsImporter{
		ctx:        ctx,
		bucketName: bucket,
		path:       path,
		base:       "/" + path,
	}, nil
}

func (g *gcsImporter) Scan() (<-chan *importer.ScanResult, error) {
	client, err := storage.NewClient(g.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create a GCS client: %w", err)
	}

	g.client = client
	g.bucket = client.Bucket(g.bucketName)

	results := make(chan *importer.ScanResult, 1000)
	go g.scan(results)
	return results, nil
}

func (g *gcsImporter) mkparents(results chan<- *importer.ScanResult, p string) {
	for {
		fi := objects.FileInfo{
			Lname: path.Base(p),
			Lmode: 0700 | os.ModeDir,
		}
		results <- importer.NewScanRecord(p, "", fi, nil, nil)

		if p == "/" {
			break
		}
		p = path.Dir(p)
	}
}

func (g *gcsImporter) scan(results chan<- *importer.ScanResult) {
	defer close(results)

	g.mkparents(results, g.base)

	query := storage.Query{Prefix: g.path}
	it := g.bucket.Objects(g.ctx, &query)
	for {
		obj, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			results <- importer.NewScanError(g.path, err)
			break
		}

		fullpath := "/" + obj.Name

		g.mkparents(results, path.Dir(fullpath))

		fi := objects.FileInfo{
			Lname:     path.Base(obj.Name),
			Lsize:     obj.Size,
			Lmode:     0644,
			LmodTime:  obj.Updated,
			Lusername: obj.Owner,
		}

		results <- importer.NewScanRecord(fullpath, "", fi, nil, func() (io.ReadCloser, error) {
			return g.bucket.Object(obj.Name).NewReader(g.ctx)
		})
	}
}

func (g *gcsImporter) Location() string { return "gcs://" + path.Join(g.bucketName, g.path) }
func (g *gcsImporter) Origin() string   { return g.bucketName }
func (g *gcsImporter) Type() string     { return "gcs" }
func (g *gcsImporter) Root() string     { return g.base }

func (g *gcsImporter) Close() error {
	if g.client != nil {
		return g.client.Close()
	}
	return nil
}
