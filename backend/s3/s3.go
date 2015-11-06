package s3

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/minio/minio-go"

	"github.com/restic/restic/backend"
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
	s3api      minio.API
	connChan   chan struct{}
	bucketname string
}

// Open opens the S3 backend at bucket and region.
func Open(regionname, bucketname string) (backend.Backend, error) {
	config := minio.Config{
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
	}

	if !strings.Contains(regionname, ".") {
		// Amazon region name
		switch regionname {
		case "us-east-1":
			config.Endpoint = "https://s3.amazonaws.com"
		default:
			config.Endpoint = "https://s3-" + regionname + ".amazonaws.com"
		}
	} else {
		// S3 compatible endpoint
		config.Endpoint = "https://" + regionname
	}

	s3api, s3err := minio.New(config)
	if s3err != nil {
		return nil, s3err
	}

	connChan := make(chan struct{}, connLimit)
	for i := 0; i < connLimit; i++ {
		connChan <- struct{}{}
	}

	return &S3Backend{s3api: s3api, bucketname: bucketname, connChan: connChan}, nil
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
	if bb.final {
		return errors.New("Already finalized")
	}

	bb.final = true

	path := s3path(t, name)

	// Check key does not already exist
	_, err := bb.b.s3api.StatObject(bb.b.bucketname, path)
	if err == nil {
		return errors.New("key already exists!")
	}

	<-bb.b.connChan
	err = bb.b.s3api.PutObject(bb.b.bucketname, path, "binary/octet-stream", int64(bb.buf.Len()), bb.buf)
	bb.b.connChan <- struct{}{}
	bb.buf.Reset()
	return err
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
	rc, _, err := be.s3api.GetObject(be.bucketname, path)
	return rc, err
}

// GetReader returns an io.ReadCloser for the Blob with the given name of
// type t at offset and length. If length is 0, the reader reads until EOF.
func (be *S3Backend) GetReader(t backend.Type, name string, offset, length uint) (io.ReadCloser, error) {
	rc, err := be.Get(t, name)
	if err != nil {
		return nil, err

	}

	n, errc := io.CopyN(ioutil.Discard, rc, int64(offset))
	if errc != nil {
		return nil, errc

	} else if n != int64(offset) {
		return nil, fmt.Errorf("less bytes read than expected, read: %d, expected: %d", n, offset)

	}

	if length == 0 {
		return rc, nil

	}

	return backend.LimitReadCloser(rc, int64(length)), nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (be *S3Backend) Test(t backend.Type, name string) (bool, error) {
	found := false
	path := s3path(t, name)
	_, err := be.s3api.StatObject(be.bucketname, path)
	if err == nil {
		found = true
	}

	// If error, then not found
	return found, nil
}

// Remove removes the blob with the given name and type.
func (be *S3Backend) Remove(t backend.Type, name string) error {
	path := s3path(t, name)
	return be.s3api.RemoveObject(be.bucketname, path)
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (be *S3Backend) List(t backend.Type, done <-chan struct{}) <-chan string {
	ch := make(chan string)

	prefix := s3path(t, "")

	listresp := be.s3api.ListObjects(be.bucketname, prefix, true)

	go func() {
		defer close(ch)
		for obj := range listresp {
			m := strings.TrimPrefix(obj.Stat.Key, prefix)
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

// Remove keys for a specified backend type
func (be *S3Backend) removeKeys(t backend.Type) {
	doneChan := make(chan struct{})
	for key := range be.List(backend.Data, doneChan) {
		be.Remove(backend.Data, key)
	}
	doneChan <- struct{}{}
}

// Delete removes all restic keys
func (be *S3Backend) Delete() error {
	be.removeKeys(backend.Data)
	be.removeKeys(backend.Key)
	be.removeKeys(backend.Lock)
	be.removeKeys(backend.Snapshot)
	be.removeKeys(backend.Index)
	be.removeKeys(backend.Config)
	return nil
}

// Close does nothing
func (be *S3Backend) Close() error { return nil }
