package s3

import (
	"bytes"
	"io"
	"strings"

	"github.com/pkg/errors"

	"github.com/minio/minio-go"

	"restic/backend"
	"restic/debug"
)

const connLimit = 10

// s3 is a backend which stores the data on an S3 endpoint.
type s3 struct {
	client     *minio.Client
	connChan   chan struct{}
	bucketname string
	prefix     string
}

// Open opens the S3 backend at bucket and region. The bucket is created if it
// does not exist yet.
func Open(cfg Config) (backend.Backend, error) {
	debug.Log("s3.Open", "open, config %#v", cfg)

	client, err := minio.New(cfg.Endpoint, cfg.KeyID, cfg.Secret, !cfg.UseHTTP)
	if err != nil {
		return nil, err
	}

	be := &s3{client: client, bucketname: cfg.Bucket, prefix: cfg.Prefix}
	be.createConnections()

	ok, err := client.BucketExists(cfg.Bucket)
	if err != nil {
		debug.Log("s3.Open", "BucketExists(%v) returned err %v, trying to create the bucket", cfg.Bucket, err)
		return nil, err
	}

	if !ok {
		// create new bucket with default ACL in default region
		err = client.MakeBucket(cfg.Bucket, "")
		if err != nil {
			return nil, err
		}
	}

	return be, nil
}

func (be *s3) s3path(t backend.Type, name string) string {
	var path string

	if be.prefix != "" {
		path = be.prefix + "/"
	}
	path += string(t)

	if t == backend.Config {
		return path
	}
	return path + "/" + name
}

func (be *s3) createConnections() {
	be.connChan = make(chan struct{}, connLimit)
	for i := 0; i < connLimit; i++ {
		be.connChan <- struct{}{}
	}
}

// Location returns this backend's location (the bucket name).
func (be *s3) Location() string {
	return be.bucketname
}

// Load returns the data stored in the backend for h at the given offset
// and saves it in p. Load has the same semantics as io.ReaderAt.
func (be s3) Load(h backend.Handle, p []byte, off int64) (n int, err error) {
	var obj *minio.Object

	debug.Log("s3.Load", "%v, offset %v, len %v", h, off, len(p))
	path := be.s3path(h.Type, h.Name)

	<-be.connChan
	defer func() {
		be.connChan <- struct{}{}
	}()

	obj, err = be.client.GetObject(be.bucketname, path)
	if err != nil {
		debug.Log("s3.Load", "  err %v", err)
		return 0, err
	}

	// make sure that the object is closed properly.
	defer func() {
		e := obj.Close()
		if err == nil {
			err = e
		}
	}()

	info, err := obj.Stat()
	if err != nil {
		return 0, err
	}

	// handle negative offsets
	if off < 0 {
		// if the negative offset is larger than the object itself, read from
		// the beginning.
		if -off > info.Size {
			off = 0
		} else {
			// otherwise compute the offset from the end of the file.
			off = info.Size + off
		}
	}

	// return an error if the offset is beyond the end of the file
	if off > info.Size {
		return 0, io.EOF
	}

	var nextError error

	// manually create an io.ErrUnexpectedEOF
	if off+int64(len(p)) > info.Size {
		newlen := info.Size - off
		p = p[:newlen]

		nextError = io.ErrUnexpectedEOF

		debug.Log("s3.Load", "    capped buffer to %v byte", len(p))
	}

	n, err = obj.ReadAt(p, off)
	if int64(n) == info.Size-off && err == io.EOF {
		err = nil
	}

	if err == nil {
		err = nextError
	}

	return n, err
}

// Save stores data in the backend at the handle.
func (be s3) Save(h backend.Handle, p []byte) (err error) {
	if err := h.Valid(); err != nil {
		return err
	}

	debug.Log("s3.Save", "%v with %d bytes", h, len(p))

	path := be.s3path(h.Type, h.Name)

	// Check key does not already exist
	_, err = be.client.StatObject(be.bucketname, path)
	if err == nil {
		debug.Log("s3.blob.Finalize()", "%v already exists", h)
		return errors.New("key already exists")
	}

	<-be.connChan
	defer func() {
		be.connChan <- struct{}{}
	}()

	debug.Log("s3.Save", "PutObject(%v, %v, %v, %v)",
		be.bucketname, path, int64(len(p)), "binary/octet-stream")
	n, err := be.client.PutObject(be.bucketname, path, bytes.NewReader(p), "binary/octet-stream")
	debug.Log("s3.Save", "%v -> %v bytes, err %#v", path, n, err)

	return err
}

// Stat returns information about a blob.
func (be s3) Stat(h backend.Handle) (bi backend.BlobInfo, err error) {
	debug.Log("s3.Stat", "%v", h)

	path := be.s3path(h.Type, h.Name)
	var obj *minio.Object

	obj, err = be.client.GetObject(be.bucketname, path)
	if err != nil {
		debug.Log("s3.Stat", "GetObject() err %v", err)
		return backend.BlobInfo{}, err
	}

	// make sure that the object is closed properly.
	defer func() {
		e := obj.Close()
		if err == nil {
			err = e
		}
	}()

	fi, err := obj.Stat()
	if err != nil {
		debug.Log("s3.Stat", "Stat() err %v", err)
		return backend.BlobInfo{}, err
	}

	return backend.BlobInfo{Size: fi.Size}, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (be *s3) Test(t backend.Type, name string) (bool, error) {
	found := false
	path := be.s3path(t, name)
	_, err := be.client.StatObject(be.bucketname, path)
	if err == nil {
		found = true
	}

	// If error, then not found
	return found, nil
}

// Remove removes the blob with the given name and type.
func (be *s3) Remove(t backend.Type, name string) error {
	path := be.s3path(t, name)
	err := be.client.RemoveObject(be.bucketname, path)
	debug.Log("s3.Remove", "%v %v -> err %v", t, name, err)
	return err
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (be *s3) List(t backend.Type, done <-chan struct{}) <-chan string {
	debug.Log("s3.List", "listing %v", t)
	ch := make(chan string)

	prefix := be.s3path(t, "")

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
func (be *s3) removeKeys(t backend.Type) error {
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
func (be *s3) Delete() error {
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
func (be *s3) Close() error { return nil }
