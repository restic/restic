package s3

import (
	"bytes"
	"io"
	"path"
	"restic"
	"strings"

	"restic/backend"
	"restic/errors"

	"github.com/minio/minio-go"

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
func Open(cfg Config) (restic.Backend, error) {
	debug.Log("open, config %#v", cfg)

	client, err := minio.New(cfg.Endpoint, cfg.KeyID, cfg.Secret, !cfg.UseHTTP)
	if err != nil {
		return nil, errors.Wrap(err, "minio.New")
	}

	be := &s3{client: client, bucketname: cfg.Bucket, prefix: cfg.Prefix}
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

func (be *s3) s3path(h restic.Handle) string {
	if h.Type == restic.ConfigFile {
		return path.Join(be.prefix, string(h.Type))
	}
	return path.Join(be.prefix, string(h.Type), h.Name)
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

	debug.Log("Save %v", h)

	objName := be.s3path(h)

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

// Load returns a reader that yields the contents of the file at h at the
// given offset. If length is nonzero, only a portion of the file is
// returned. rd must be closed after use.
func (be *s3) Load(h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
	debug.Log("Load %v, length %v, offset %v", h, length, offset)
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

	objName := be.s3path(h)

	<-be.connChan
	defer func() {
		be.connChan <- struct{}{}
	}()

	obj, err := be.client.GetObject(be.bucketname, objName)
	if err != nil {
		debug.Log("  err %v", err)
		return nil, errors.Wrap(err, "client.GetObject")
	}

	// if we're going to read the whole object, just pass it on.
	if length == 0 {
		debug.Log("Load %v: pass on object", h)
		_, err = obj.Seek(offset, 0)
		if err != nil {
			_ = obj.Close()
			return nil, errors.Wrap(err, "obj.Seek")
		}

		return obj, nil
	}

	// otherwise use a buffer with ReadAt
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, errors.Wrap(err, "obj.Stat")
	}

	if offset > info.Size {
		_ = obj.Close()
		return nil, errors.Errorf("offset larger than file size")
	}

	l := int64(length)
	if offset+l > info.Size {
		l = info.Size - offset
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

	objName := be.s3path(h)
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
	objName := be.s3path(h)
	_, err := be.client.StatObject(be.bucketname, objName)
	if err == nil {
		found = true
	}

	// If error, then not found
	return found, nil
}

// Remove removes the blob with the given name and type.
func (be *s3) Remove(h restic.Handle) error {
	objName := be.s3path(h)
	err := be.client.RemoveObject(be.bucketname, objName)
	debug.Log("Remove(%v) -> err %v", h, err)
	return errors.Wrap(err, "client.RemoveObject")
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (be *s3) List(t restic.FileType, done <-chan struct{}) <-chan string {
	debug.Log("listing %v", t)
	ch := make(chan string)

	prefix := be.s3path(restic.Handle{Type: t}) + "/"

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
