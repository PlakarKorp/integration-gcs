package gcs

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/PlakarKorp/kloset/objects"
	kstorage "github.com/PlakarKorp/kloset/storage"
	"google.golang.org/api/iterator"
)

type gcsStore struct {
	ctx        context.Context
	bucketName string
	path       string

	client *storage.Client
	bucket *storage.BucketHandle
}

func NewStore(ctx context.Context, proto string, params map[string]string) (kstorage.Store, error) {
	target := params["location"]
	bucket, path, _ := strings.Cut(strings.TrimPrefix(target, proto+"://"), "/")

	path = strings.Trim(path, "/")
	return &gcsStore{
		ctx:        ctx,
		bucketName: bucket,
		path:       path,
	}, nil
}

func (g *gcsStore) connect() error {
	client, err := storage.NewClient(g.ctx)
	if err != nil {
		return err
	}

	g.client = client
	g.bucket = client.Bucket(g.bucketName)
	return nil
}

func (g *gcsStore) realpath(rel string) string { return path.Join(g.path, rel) }

func (g *gcsStore) Create(ctx context.Context, config []byte) error {
	if err := g.connect(); err != nil {
		return err
	}

	obj := g.bucket.Object(g.realpath("CONFIG"))
	_, err := obj.Attrs(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		w := obj.NewWriter(ctx)
		defer w.Close()
		_, err = w.Write(config)
		return err
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("repository already exists")
}

func (g *gcsStore) Open(ctx context.Context) ([]byte, error) {
	if err := g.connect(); err != nil {
		return nil, err
	}

	rd, err := g.bucket.Object(g.realpath("CONFIG")).NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer rd.Close()

	data, err := io.ReadAll(rd)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (g *gcsStore) Location() string    { return "gcs://" + path.Join(g.bucketName, g.path) }
func (g *gcsStore) Mode() kstorage.Mode { return kstorage.ModeRead | kstorage.ModeWrite }
func (g *gcsStore) Size() int64         { return -1 }

func (g *gcsStore) GetStates() (states []objects.MAC, err error) {
	prefix := g.realpath("states")
	l := len(prefix) + 4 // /%02x/

	query := &storage.Query{Prefix: prefix}
	it := g.bucket.Objects(g.ctx, query)
	for {
		obj, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		if len(obj.Name) > l {
			t, err := hex.DecodeString(obj.Name[l:])
			if err != nil {
				return nil, fmt.Errorf("decode state key: %w", err)
			}
			if len(t) != 32 {
				return nil, fmt.Errorf("invalid state name: %s", obj.Name)
			}
			states = append(states, objects.MAC(t))
		}
	}
	return
}

func (g *gcsStore) PutState(mac objects.MAC, rd io.Reader) (int64, error) {
	name := fmt.Sprintf("states/%02x/%016x", mac[0], mac)
	w := g.bucket.Object(g.realpath(name)).NewWriter(g.ctx)

	len, err := io.Copy(w, rd)
	if err != nil {
		w.Close()
		return 0, fmt.Errorf("failed to write object: %w", err)
	}

	if err := w.Close(); err != nil {
		return 0, fmt.Errorf("failed to write object: %w", err)
	}

	return len, nil
}

func (g *gcsStore) GetState(mac objects.MAC) (io.Reader, error) {
	name := fmt.Sprintf("states/%02x/%016x", mac[0], mac)
	rd, err := g.bucket.Object(g.realpath(name)).NewReader(g.ctx)
	if err != nil {
		return nil, err
	}
	defer rd.Close()

	buf, err := io.ReadAll(rd)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(buf), nil
}

func (g *gcsStore) DeleteState(mac objects.MAC) error {
	name := fmt.Sprintf("states/%02x/%016x", mac[0], mac)
	return g.bucket.Object(g.realpath(name)).Delete(g.ctx)
}

func (g *gcsStore) GetPackfiles() (packfiles []objects.MAC, err error) {
	prefix := g.realpath("packfiles")
	l := len(prefix) + 4 // /%02x/

	query := &storage.Query{Prefix: prefix}
	it := g.bucket.Objects(g.ctx, query)
	for {
		obj, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		if len(obj.Name) > l {
			t, err := hex.DecodeString(obj.Name[l:])
			if err != nil {
				return nil, fmt.Errorf("decode packfile name: %w", err)
			}
			if len(t) != 32 {
				return nil, fmt.Errorf("invalid packfile name: %s", obj.Name)
			}
			packfiles = append(packfiles, objects.MAC(t))
		}
	}
	return
}

func (g *gcsStore) PutPackfile(mac objects.MAC, rd io.Reader) (int64, error) {
	name := fmt.Sprintf("packfiles/%02x/%016x", mac[0], mac)
	w := g.bucket.Object(g.realpath(name)).NewWriter(g.ctx)

	len, err := io.Copy(w, rd)
	if err != nil {
		w.Close()
		return 0, fmt.Errorf("failed to write object: %w", err)
	}

	if err := w.Close(); err != nil {
		return 0, fmt.Errorf("failed to write object: %w", err)
	}

	return len, nil
}

func (g *gcsStore) GetPackfile(mac objects.MAC) (io.Reader, error) {
	name := fmt.Sprintf("packfiles/%02x/%016x", mac[0], mac)
	rd, err := g.bucket.Object(g.realpath(name)).NewReader(g.ctx)
	if err != nil {
		return nil, err
	}
	defer rd.Close()

	buf, err := io.ReadAll(rd)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(buf), nil
}

func (g *gcsStore) GetPackfileBlob(mac objects.MAC, offset uint64, length uint32) (io.Reader, error) {
	name := fmt.Sprintf("packfiles/%02x/%016x", mac[0], mac)
	rd, err := g.bucket.Object(g.realpath(name)).NewRangeReader(g.ctx, int64(offset), int64(length))
	if err != nil {
		return nil, err
	}
	defer rd.Close()

	buf, err := io.ReadAll(rd)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(buf), nil
}

func (g *gcsStore) DeletePackfile(mac objects.MAC) error {
	name := fmt.Sprintf("packfiles/%02x/%016x", mac[0], mac)
	return g.bucket.Object(g.realpath(name)).Delete(g.ctx)
}

func (g *gcsStore) GetLocks() (locks []objects.MAC, error error) {
	prefix := g.realpath("locks")
	l := len(prefix) + 4 // /%02x/

	query := &storage.Query{Prefix: prefix}
	it := g.bucket.Objects(g.ctx, query)
	for {
		obj, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		if len(obj.Name) > l {
			t, err := hex.DecodeString(obj.Name[l:])
			if err != nil {
				return nil, fmt.Errorf("decode locks name: %w", err)
			}
			if len(t) != 32 {
				return nil, fmt.Errorf("invalid packfile name: %s", obj.Name)
			}
			locks = append(locks, objects.MAC(t))
		}
	}
	return
}

func (g *gcsStore) PutLock(lockID objects.MAC, rd io.Reader) (int64, error) {
	name := fmt.Sprintf("locks/%02x/%016x", lockID[0], lockID)
	w := g.bucket.Object(g.realpath(name)).NewWriter(g.ctx)

	len, err := io.Copy(w, rd)
	if err != nil {
		w.Close()
		return 0, fmt.Errorf("failed to write object: %w", err)
	}

	if err := w.Close(); err != nil {
		return 0, fmt.Errorf("failed to write object: %w", err)
	}

	return len, nil
}

func (g *gcsStore) GetLock(lockID objects.MAC) (io.Reader, error) {
	name := fmt.Sprintf("locks/%02x/%016x", lockID[0], lockID)
	rd, err := g.bucket.Object(g.realpath(name)).NewReader(g.ctx)
	if err != nil {
		return nil, err
	}
	defer rd.Close()

	buf, err := io.ReadAll(rd)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(buf), nil
}

func (g *gcsStore) DeleteLock(lockID objects.MAC) error {
	name := fmt.Sprintf("locks/%02x/%016x", lockID[0], lockID)
	return g.bucket.Object(g.realpath(name)).Delete(g.ctx)
}

func (g *gcsStore) Close() error {
	return g.client.Close()
}
