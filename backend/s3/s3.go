package s3

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
)

const maxKeysInList = 1000
const connLimit = 10
const backendPrefix = "restic"

func s3path(t backend.Type, name string) string {
	if t == backend.Config {
		return backendPrefix + "/" + string(t)
	}
	return backendPrefix + "/" + string(t) + "/" + name
}

type S3Backend struct {
	client     minio.CloudStorageClient
	connChan   chan struct{}
	bucketname string
}

// Open opens the S3 backend at bucket and region. The bucket is created if it
// does not exist yet.
func Open(cfg Config) (backend.Backend, error) {
	debug.Log("s3.Open", "open, config %#v", cfg)

	client, err := minio.New(cfg.Endpoint, cfg.KeyID, cfg.Secret, cfg.UseHTTP)
	if err != nil {
		return nil, err
	}

	be := &S3Backend{client: client, bucketname: cfg.Bucket}
	be.createConnections()

	if err := client.BucketExists(cfg.Bucket); err != nil {
		debug.Log("s3.Open", "BucketExists(%v) returned err %v, trying to create the bucket", err)

		// create new bucket with default ACL in default region
		err = client.MakeBucket(cfg.Bucket, "", "")

		if err != nil {
			return nil, err
		}
	}

	return be, nil
}

func (be *S3Backend) createConnections() {
	be.connChan = make(chan struct{}, connLimit)
	for i := 0; i < connLimit; i++ {
		be.connChan <- struct{}{}
	}
}

// Location returns this backend's location (the bucket name).
func (be *S3Backend) Location() string {
	return be.bucketname
}

type s3Blob struct {
	b     *S3Backend
	buf   *bytes.Buffer
	final bool
}

func (bb *s3Blob) Write(p []byte) (int, error) {
	if bb.final {
		return 0, errors.New("blob already closed")
	}

	n, err := bb.buf.Write(p)
	return n, err
}

func (bb *s3Blob) Read(p []byte) (int, error) {
	return bb.buf.Read(p)
}

func (bb *s3Blob) Close() error {
	bb.final = true
	bb.buf.Reset()
	return nil
}

func (bb *s3Blob) Size() uint {
	return uint(bb.buf.Len())
}

func (bb *s3Blob) Finalize(t backend.Type, name string) error {
	debug.Log("s3.blob.Finalize()", "bucket %v, finalize %v, %d bytes", bb.b.bucketname, name, bb.buf.Len())
	if bb.final {
		return errors.New("Already finalized")
	}

	bb.final = true

	path := s3path(t, name)

	// Check key does not already exist
	_, err := bb.b.client.StatObject(bb.b.bucketname, path)
	if err == nil {
		debug.Log("s3.blob.Finalize()", "%v already exists", name)
		return errors.New("key already exists")
	}

	expectedBytes := bb.buf.Len()

	<-bb.b.connChan
	debug.Log("s3.Finalize", "PutObject(%v, %v, %v, %v)",
		bb.b.bucketname, path, int64(bb.buf.Len()), "binary/octet-stream")
	n, err := bb.b.client.PutObject(bb.b.bucketname, path, bb.buf, int64(bb.buf.Len()), "binary/octet-stream")
	debug.Log("s3.Finalize", "finalized %v -> n %v, err %#v", path, n, err)
	bb.b.connChan <- struct{}{}

	if err != nil {
		return err
	}

	if n != int64(expectedBytes) {
		return errors.New("could not store all bytes")
	}

	return nil
}

// Create creates a new Blob. The data is available only after Finalize()
// has been called on the returned Blob.
func (be *S3Backend) Create() (backend.Blob, error) {
	blob := s3Blob{
		b:   be,
		buf: &bytes.Buffer{},
	}

	return &blob, nil
}

// Get returns a reader that yields the content stored under the given
// name. The reader should be closed after draining it.
func (be *S3Backend) Get(t backend.Type, name string) (io.ReadCloser, error) {
	path := s3path(t, name)
	rc, _, err := be.client.GetObject(be.bucketname, path)
	debug.Log("s3.Get", "%v %v -> err %v", t, name, err)
	return rc, err
}

// GetReader returns an io.ReadCloser for the Blob with the given name of
// type t at offset and length. If length is 0, the reader reads until EOF.
func (be *S3Backend) GetReader(t backend.Type, name string, offset, length uint) (io.ReadCloser, error) {
	debug.Log("s3.GetReader", "%v %v, offset %v len %v", t, name, offset, length)
	path := s3path(t, name)
	rd, stat, err := be.client.GetObjectPartial(be.bucketname, path)
	debug.Log("s3.GetReader", "  stat %v, err %v", stat, err)
	if err != nil {
		return nil, err
	}

	l, o := int64(length), int64(offset)

	if l == 0 {
		l = stat.Size
	}

	if o > stat.Size {
		return nil, fmt.Errorf("offset beyond end of file (%v > %v)", o, stat.Size)
	}

	if o+l > stat.Size {
		l = stat.Size - o
	}

	debug.Log("s3.GetReader", "%v %v, o %v l %v", t, name, o, l)

	var r io.Reader
	r = &ContinuousReader{R: rd, Offset: o}
	if length > 0 {
		r = io.LimitReader(r, int64(length))
	}

	return backend.ReadCloser(r), nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (be *S3Backend) Test(t backend.Type, name string) (bool, error) {
	found := false
	path := s3path(t, name)
	_, err := be.client.StatObject(be.bucketname, path)
	if err == nil {
		found = true
	}

	// If error, then not found
	return found, nil
}

// Remove removes the blob with the given name and type.
func (be *S3Backend) Remove(t backend.Type, name string) error {
	path := s3path(t, name)
	err := be.client.RemoveObject(be.bucketname, path)
	debug.Log("s3.Remove", "%v %v -> err %v", t, name, err)
	return err
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (be *S3Backend) List(t backend.Type, done <-chan struct{}) <-chan string {
	debug.Log("s3.List", "listing %v", t)
	ch := make(chan string)

	prefix := s3path(t, "")

	listresp := be.client.ListObjects(be.bucketname, prefix, true, done)

	go func() {
		defer close(ch)
		for obj := range listresp {
			m := strings.TrimPrefix(obj.Key, prefix)
			if m == "" {
				continue
			}

			select {
			case ch <- m:
			case <-done:
				return
			}
		}
	}()

	return ch
}

// Remove keys for a specified backend type.
func (be *S3Backend) removeKeys(t backend.Type) error {
	done := make(chan struct{})
	defer close(done)
	for key := range be.List(backend.Data, done) {
		err := be.Remove(backend.Data, key)
		if err != nil {
			return err
		}
	}

	return nil
}

// Delete removes all restic keys in the bucket. It will not remove the bucket itself.
func (be *S3Backend) Delete() error {
	alltypes := []backend.Type{
		backend.Data,
		backend.Key,
		backend.Lock,
		backend.Snapshot,
		backend.Index}

	for _, t := range alltypes {
		err := be.removeKeys(t)
		if err != nil {
			return nil
		}
	}

	return be.Remove(backend.Config, "")
}

// Close does nothing
func (be *S3Backend) Close() error { return nil }
