// Package gcs implements a Google Cloud Storage backend for restic.
package gcs

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"sort"
	"strings"

	"github.com/restic/restic/backend"
	"github.com/twinj/uuid"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/cloud"
	"google.golang.org/cloud/storage"
)

const rootName = "restic"

func typePrefix(t backend.Type) string {
	return fmt.Sprintf("%s/%s", rootName, string(t))
}

func objName(t backend.Type, name string) string {
	if t == backend.Config {
		return typePrefix(t)
	}
	return fmt.Sprintf("%s/%s", typePrefix(t), name)
}

func OpenWithKeyfile(keyfile, project, bucket string) (backend.Backend, error) {
	jsonKey, err := ioutil.ReadFile(keyfile)
	if err != nil {
		return nil, err
	}

	conf, err := google.JWTConfigFromJSON(jsonKey, storage.ScopeFullControl)
	if err != nil {
		return nil, err
	}

	ctx := cloud.NewContext(project, conf.Client(oauth2.NoContext))

	return &be{
		gcs: &server{
			ctx:    ctx,
			bucket: bucket,
		},
	}, nil
}

// OpenMemory opens an in-memory backend.  It's used for testing.
func OpenMemory() backend.Backend {
	return &be{
		gcs: &fakeGCS{
			blobs: make(map[string]*fakeRWC),
		},
	}
}

// gcs mediates calls to the GCS service.  It is used for testing.
type gcs interface {
	CopyObject(src, dst string) error
	DeleteObject(name string) error
	StatObject(name string) error
	ListObjects(*storage.Query) (*storage.Objects, error)
	NewReader(name string) (io.ReadCloser, error)
	NewWriter(name string) io.WriteCloser
	String() string
}

// server satisfies the gcs interface by actually making GCS requests.
type server struct {
	ctx    context.Context
	bucket string
}

func (s *server) CopyObject(srcName, dstName string) error {
	_, err := storage.CopyObject(s.ctx, s.bucket, srcName, s.bucket, dstName, nil)
	return err
}

func (s *server) DeleteObject(name string) error {
	return storage.DeleteObject(s.ctx, s.bucket, name)
}

func (s *server) StatObject(name string) error {
	_, err := storage.StatObject(s.ctx, s.bucket, name)
	return err
}

func (s *server) ListObjects(q *storage.Query) (*storage.Objects, error) {
	return storage.ListObjects(s.ctx, s.bucket, q)
}

func (s *server) NewReader(name string) (io.ReadCloser, error) {
	return storage.NewReader(s.ctx, s.bucket, name)
}

func (s *server) NewWriter(name string) io.WriteCloser {
	return storage.NewWriter(s.ctx, s.bucket, name)
}

func (s *server) String() string {
	return s.bucket
}

// blob satisfies the backend.Blob interface.
type blob struct {
	gcs     gcs
	wc      io.WriteCloser
	size    uint
	tmpName string
}

func (b *blob) Write(p []byte) (int, error) {
	s, err := b.wc.Write(p)
	b.size += uint(s)
	return s, err
}

func (b *blob) Finalize(t backend.Type, name string) error {
	if err := b.gcs.StatObject(objName(t, name)); err != storage.ErrObjectNotExist {
		return fmt.Errorf("%s: file exists", objName(t, name))
	}

	if err := b.wc.Close(); err != nil {
		return err
	}

	dstName := objName(t, name)
	if err := b.gcs.CopyObject(b.tmpName, dstName); err != nil {
		return err
	}

	return b.gcs.DeleteObject(b.tmpName)
}

func (b *blob) Size() uint { return b.size }

// be satisfies the backend.Backend interface.
type be struct {
	gcs gcs
}

func (b *be) Close() error {
	return nil
}

func (b *be) Create() (backend.Blob, error) {
	tmpName := uuid.NewV4().String()
	return &blob{
		gcs:     b.gcs,
		wc:      b.gcs.NewWriter(tmpName),
		tmpName: tmpName,
	}, nil
}

func (b *be) Get(t backend.Type, name string) (io.ReadCloser, error) {
	return b.gcs.NewReader(objName(t, name))
}

func (b *be) GetReader(t backend.Type, name string, offset, length uint) (io.ReadCloser, error) {
	r, err := b.Get(t, name)
	if err != nil {
		return nil, err
	}
	if _, err := io.CopyN(ioutil.Discard, r, int64(offset)); err != nil {
		r.Close()
		return nil, err
	}
	return backend.LimitReadCloser(r, int64(length)), nil
}

func (b *be) List(t backend.Type, done <-chan struct{}) <-chan string {
	pfx := typePrefix(t) + "/"
	query := &storage.Query{
		Prefix: pfx,
	}
	// So this is pretty awful, but in order to satisfy the lexical
	// ordering restraint, we have to slurp ALL the objects, sort them, and
	// then emit them on the channel.  This means we might as well do most
	// of this synchronously because the caller's going to be stuck waiting
	// on the channel anyway.
	var results []string
	for {
		objs, err := b.gcs.ListObjects(query)
		if err != nil {
			// There's no good way to return this to the caller.
			log.Printf("gcs list objects: %v", err)
			return nil
		}
		for _, obj := range objs.Results {
			results = append(results, strings.TrimPrefix(obj.Name, pfx))
		}
		query = objs.Next
		if query == nil {
			break
		}
	}
	sort.Strings(results)
	out := make(chan string)
	go func() {
		defer close(out)
		for _, s := range results {
			select {
			case <-done:
				return
			case out <- s:
			}
		}
	}()
	return out
}

func (b *be) Location() string {
	return "gcs://" + b.gcs.String()
}

func (b *be) Remove(t backend.Type, name string) error {
	return b.gcs.DeleteObject(objName(t, name))
}

func (b *be) Test(t backend.Type, name string) (bool, error) {
	err := b.gcs.StatObject(objName(t, name))
	if err == storage.ErrObjectNotExist {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
