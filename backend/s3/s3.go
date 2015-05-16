package s3

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/s3"

	"github.com/restic/restic/backend"
)

const maxKeysInList = 1000

func s3path(t backend.Type, name string) string {
	if t == backend.Config {
		return string(t)
	}
	return string(t) + "/" + name
}

type S3 struct {
	bucket *s3.Bucket
	mput   sync.Mutex
	path   string
}

// Open a backend using an S3 bucket object
func OpenS3Bucket(bucket *s3.Bucket, bucketname string) *S3 {
	return &S3{bucket: bucket, path: bucketname}
}

// Open opens the s3 backend at bucket and region.
func Open(regionname, bucketname string) (*S3, error) {
	auth, err := aws.EnvAuth()
	if err != nil {
		return nil, err
	}

	client := s3.New(auth, aws.Regions[regionname])

	return &S3{bucket: client.Bucket(bucketname), path: bucketname}, nil
}

// Location returns this backend's location (the bucket name).
func (b *S3) Location() string {
	return b.path
}

type s3Blob struct {
	b     *S3
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
	key, err := bb.b.bucket.GetKey(path)
	if err == nil && key.Key == path {
		return errors.New("key already exists!")
	}

	bb.b.mput.Lock()
	err = bb.b.bucket.Put(path, bb.buf.Bytes(), "binary/octet-stream", "private")
	bb.b.mput.Unlock()
	bb.buf.Reset()
	return err
}

// Create creates a new Blob. The data is available only after Finalize()
// has been called on the returned Blob.
func (b *S3) Create() (backend.Blob, error) {
	blob := s3Blob{
		b:   b,
		buf: &bytes.Buffer{},
	}

	return &blob, nil
}

func (b *S3) get(t backend.Type, name string) (*s3Blob, error) {
	blob := &s3Blob{
		b: b,
	}

	path := s3path(t, name)
	data, err := b.bucket.Get(path)
	blob.buf = bytes.NewBuffer(data)
	return blob, err
}

// Get returns a reader that yields the content stored under the given
// name. The reader should be closed after draining it.
func (b *S3) Get(t backend.Type, name string) (io.ReadCloser, error) {
	return b.get(t, name)
}

// GetReader returns an io.ReadCloser for the Blob with the given name of
// type t at offset and length. If length is 0, the reader reads until EOF.
func (b *S3) GetReader(t backend.Type, name string, offset, length uint) (io.ReadCloser, error) {
	blob, err := b.get(t, name)
	if err != nil {
		return nil, err
	}

	blob.buf.Next(int(offset))

	if length == 0 {
		return blob, nil
	}

	return backend.LimitReadCloser(blob, int64(length)), nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (b *S3) Test(t backend.Type, name string) (bool, error) {
	found := false
	path := s3path(t, name)
	key, err := b.bucket.GetKey(path)
	if err == nil && key.Key == path {
		found = true
	}

	// If error, then not found
	return found, nil
}

// Remove removes the blob with the given name and type.
func (b *S3) Remove(t backend.Type, name string) error {
	path := s3path(t, name)
	return b.bucket.Del(path)
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (b *S3) List(t backend.Type, done <-chan struct{}) <-chan string {
	ch := make(chan string)

	prefix := string(t) + "/"

	listresp, err := b.bucket.List(prefix, "/", "", maxKeysInList)

	if err != nil {
		close(ch)
		return ch
	}

	matches := make([]string, len(listresp.Contents))
	for idx, key := range listresp.Contents {
		matches[idx] = strings.TrimPrefix(key.Key, prefix)
	}

	// Continue making requests to get full list.
	for listresp.IsTruncated {
		listresp, err = b.bucket.List(prefix, "/", listresp.NextMarker, maxKeysInList)
		if err != nil {
			close(ch)
			return ch
		}

		for _, key := range listresp.Contents {
			matches = append(matches, strings.TrimPrefix(key.Key, prefix))
		}
	}

	go func() {
		defer close(ch)
		for _, m := range matches {
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

// Delete removes the repository and all files.
func (b *S3) Delete() error {
	return b.bucket.DelBucket()
}

// Close does nothing
func (b *S3) Close() error { return nil }
