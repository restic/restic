package s3

import (
	"bytes"
	"io"
	"path"
	"restic"
	"strings"
	"sync"

	"restic/backend"
	"restic/errors"

	"github.com/minio/minio-go"

	"restic/debug"
)

const connLimit = 10

// s3 is a backend which stores the data on an S3 endpoint.
type s3 struct {
	client       *minio.Client
	connChan     chan struct{}
	bucketname   string
	prefix       string
	cacheMutex   sync.RWMutex
	cacheObjSize map[string]int64
	backend.Layout
}

// Open opens the S3 backend at bucket and region. The bucket is created if it
// does not exist yet.
func Open(cfg Config) (restic.Backend, error) {
	debug.Log("open, config %#v", cfg)

	client, err := minio.New(cfg.Endpoint, cfg.KeyID, cfg.Secret, !cfg.UseHTTP)
	if err != nil {
		return nil, errors.Wrap(err, "minio.New")
	}

	be := &s3{
		client:       client,
		bucketname:   cfg.Bucket,
		prefix:       cfg.Prefix,
		cacheObjSize: make(map[string]int64),
		Layout:       &backend.S3Layout{Path: cfg.Prefix, Join: path.Join},
	}

	client.SetCustomTransport(backend.Transport())

	be.createConnections()

	found, err := client.BucketExists(cfg.Bucket)
	if err != nil {
		debug.Log("BucketExists(%v) returned err %v", cfg.Bucket, err)
		return nil, errors.Wrap(err, "client.BucketExists")
	}

	if !found {
		// create new bucket with default ACL in default region
		err = client.MakeBucket(cfg.Bucket, "")
		if err != nil {
			return nil, errors.Wrap(err, "client.MakeBucket")
		}
	}

	return be, nil
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

// Save stores data in the backend at the handle.
func (be *s3) Save(h restic.Handle, rd io.Reader) (err error) {
	if err := h.Valid(); err != nil {
		return err
	}

	objName := be.Filename(h)

	debug.Log("Save %v at %v", h, objName)

	// Check key does not already exist
	_, err = be.client.StatObject(be.bucketname, objName)
	if err == nil {
		debug.Log("%v already exists", h)
		return errors.New("key already exists")
	}

	<-be.connChan
	defer func() {
		be.connChan <- struct{}{}
	}()

	debug.Log("PutObject(%v, %v)",
		be.bucketname, objName)
	n, err := be.client.PutObject(be.bucketname, objName, rd, "binary/octet-stream")
	debug.Log("%v -> %v bytes, err %#v", objName, n, err)

	return errors.Wrap(err, "client.PutObject")
}

// wrapReader wraps an io.ReadCloser to run an additional function on Close.
type wrapReader struct {
	io.ReadCloser
	f func()
}

func (wr wrapReader) Close() error {
	err := wr.ReadCloser.Close()
	wr.f()
	return err
}

// Load returns a reader that yields the contents of the file at h at the
// given offset. If length is nonzero, only a portion of the file is
// returned. rd must be closed after use.
func (be *s3) Load(h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
	debug.Log("Load %v, length %v, offset %v from %v", h, length, offset, be.Filename(h))
	if err := h.Valid(); err != nil {
		return nil, err
	}

	if offset < 0 {
		return nil, errors.New("offset is negative")
	}

	if length < 0 {
		return nil, errors.Errorf("invalid length %d", length)
	}

	var obj *minio.Object
	var size int64

	objName := be.Filename(h)

	// get token for connection
	<-be.connChan

	obj, err := be.client.GetObject(be.bucketname, objName)
	if err != nil {
		debug.Log("  err %v", err)

		// return token
		be.connChan <- struct{}{}

		return nil, errors.Wrap(err, "client.GetObject")
	}

	// if we're going to read the whole object, just pass it on.
	if length == 0 {
		debug.Log("Load %v: pass on object", h)

		_, err = obj.Seek(offset, 0)
		if err != nil {
			_ = obj.Close()

			// return token
			be.connChan <- struct{}{}

			return nil, errors.Wrap(err, "obj.Seek")
		}

		rd := wrapReader{
			ReadCloser: obj,
			f: func() {
				debug.Log("Close()")
				// return token
				be.connChan <- struct{}{}
			},
		}
		return rd, nil
	}

	defer func() {
		// return token
		be.connChan <- struct{}{}
	}()

	// otherwise use a buffer with ReadAt
	be.cacheMutex.RLock()
	size, cacheHit := be.cacheObjSize[objName]
	be.cacheMutex.RUnlock()

	if !cacheHit {
		info, err := obj.Stat()
		if err != nil {
			_ = obj.Close()
			return nil, errors.Wrap(err, "obj.Stat")
		}
		size = info.Size
		be.cacheMutex.Lock()
		be.cacheObjSize[objName] = size
		be.cacheMutex.Unlock()
	}

	if offset > size {
		_ = obj.Close()
		return nil, errors.New("offset larger than file size")
	}

	l := int64(length)
	if offset+l > size {
		l = size - offset
	}

	buf := make([]byte, l)
	n, err := obj.ReadAt(buf, offset)
	debug.Log("Load %v: use buffer with ReadAt: %v, %v", h, n, err)
	if err == io.EOF {
		debug.Log("Load %v: shorten buffer %v -> %v", h, len(buf), n)
		buf = buf[:n]
		err = nil
	}

	if err != nil {
		_ = obj.Close()
		return nil, errors.Wrap(err, "obj.ReadAt")
	}

	return backend.Closer{Reader: bytes.NewReader(buf)}, nil
}

// Stat returns information about a blob.
func (be *s3) Stat(h restic.Handle) (bi restic.FileInfo, err error) {
	debug.Log("%v", h)

	objName := be.Filename(h)
	var obj *minio.Object

	obj, err = be.client.GetObject(be.bucketname, objName)
	if err != nil {
		debug.Log("GetObject() err %v", err)
		return restic.FileInfo{}, errors.Wrap(err, "client.GetObject")
	}

	// make sure that the object is closed properly.
	defer func() {
		e := obj.Close()
		if err == nil {
			err = errors.Wrap(e, "Close")
		}
	}()

	fi, err := obj.Stat()
	if err != nil {
		debug.Log("Stat() err %v", err)
		return restic.FileInfo{}, errors.Wrap(err, "Stat")
	}

	return restic.FileInfo{Size: fi.Size}, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (be *s3) Test(h restic.Handle) (bool, error) {
	found := false
	objName := be.Filename(h)
	_, err := be.client.StatObject(be.bucketname, objName)
	if err == nil {
		found = true
	}

	// If error, then not found
	return found, nil
}

// Remove removes the blob with the given name and type.
func (be *s3) Remove(h restic.Handle) error {
	objName := be.Filename(h)
	err := be.client.RemoveObject(be.bucketname, objName)
	debug.Log("Remove(%v) at %v -> err %v", h, objName, err)
	return errors.Wrap(err, "client.RemoveObject")
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (be *s3) List(t restic.FileType, done <-chan struct{}) <-chan string {
	debug.Log("listing %v", t)
	ch := make(chan string)

	prefix := be.Dirname(restic.Handle{Type: t})

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
func (be *s3) removeKeys(t restic.FileType) error {
	done := make(chan struct{})
	defer close(done)
	for key := range be.List(restic.DataFile, done) {
		err := be.Remove(restic.Handle{Type: restic.DataFile, Name: key})
		if err != nil {
			return err
		}
	}

	return nil
}

// Delete removes all restic keys in the bucket. It will not remove the bucket itself.
func (be *s3) Delete() error {
	alltypes := []restic.FileType{
		restic.DataFile,
		restic.KeyFile,
		restic.LockFile,
		restic.SnapshotFile,
		restic.IndexFile}

	for _, t := range alltypes {
		err := be.removeKeys(t)
		if err != nil {
			return nil
		}
	}

	return be.Remove(restic.Handle{Type: restic.ConfigFile})
}

// Close does nothing
func (be *s3) Close() error { return nil }
