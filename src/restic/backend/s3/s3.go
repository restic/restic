package s3

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"restic"
	"strings"
	"time"

	"restic/backend"
	"restic/errors"

	"github.com/minio/minio-go"

	"restic/debug"
)

const connLimit = 10

// s3 is a backend which stores the data on an S3 endpoint.
type s3 struct {
	client     *minio.Client
	sem        *backend.Semaphore
	bucketname string
	prefix     string
	backend.Layout
}

// make sure that *s3 implements backend.Backend
var _ restic.Backend = &s3{}

const defaultLayout = "s3legacy"

// Open opens the S3 backend at bucket and region. The bucket is created if it
// does not exist yet.
func Open(cfg Config) (restic.Backend, error) {
	debug.Log("open, config %#v", cfg)

	client, err := minio.New(cfg.Endpoint, cfg.KeyID, cfg.Secret, !cfg.UseHTTP)
	if err != nil {
		return nil, errors.Wrap(err, "minio.New")
	}

	sem, err := backend.NewSemaphore(cfg.Connections)
	if err != nil {
		return nil, err
	}

	be := &s3{
		client:     client,
		sem:        sem,
		bucketname: cfg.Bucket,
		prefix:     cfg.Prefix,
	}

	client.SetCustomTransport(backend.Transport())

	l, err := backend.ParseLayout(be, cfg.Layout, defaultLayout, cfg.Prefix)
	if err != nil {
		return nil, err
	}

	be.Layout = l

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

// IsNotExist returns true if the error is caused by a not existing file.
func (be *s3) IsNotExist(err error) bool {
	debug.Log("IsNotExist(%T, %#v)", err, err)
	return os.IsNotExist(err)
}

// Join combines path components with slashes.
func (be *s3) Join(p ...string) string {
	return path.Join(p...)
}

type fileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
}

func (fi fileInfo) Name() string       { return fi.name }    // base name of the file
func (fi fileInfo) Size() int64        { return fi.size }    // length in bytes for regular files; system-dependent for others
func (fi fileInfo) Mode() os.FileMode  { return fi.mode }    // file mode bits
func (fi fileInfo) ModTime() time.Time { return fi.modTime } // modification time
func (fi fileInfo) IsDir() bool        { return fi.isDir }   // abbreviation for Mode().IsDir()
func (fi fileInfo) Sys() interface{}   { return nil }        // underlying data source (can return nil)

// ReadDir returns the entries for a directory.
func (be *s3) ReadDir(dir string) (list []os.FileInfo, err error) {
	debug.Log("ReadDir(%v)", dir)

	// make sure dir ends with a slash
	if dir[len(dir)-1] != '/' {
		dir += "/"
	}

	done := make(chan struct{})
	defer close(done)

	for obj := range be.client.ListObjects(be.bucketname, dir, false, done) {
		if obj.Key == "" {
			continue
		}

		name := strings.TrimPrefix(obj.Key, dir)
		if name == "" {
			return nil, errors.Errorf("invalid key name %v, removing prefix %v yielded empty string", obj.Key, dir)
		}
		entry := fileInfo{
			name:    name,
			size:    obj.Size,
			modTime: obj.LastModified,
		}

		if name[len(name)-1] == '/' {
			entry.isDir = true
			entry.mode = os.ModeDir | 0755
			entry.name = name[:len(name)-1]
		} else {
			entry.mode = 0644
		}

		list = append(list, entry)
	}

	return list, nil
}

// Location returns this backend's location (the bucket name).
func (be *s3) Location() string {
	return be.bucketname
}

// getRemainingSize returns number of bytes remaining. If it is not possible to
// determine the size, panic() is called.
func getRemainingSize(rd io.Reader) (size int64, err error) {
	type Sizer interface {
		Size() int64
	}

	type Lenner interface {
		Len() int
	}

	if r, ok := rd.(Lenner); ok {
		size = int64(r.Len())
	} else if r, ok := rd.(Sizer); ok {
		size = r.Size()
	} else if f, ok := rd.(*os.File); ok {
		fi, err := f.Stat()
		if err != nil {
			return 0, err
		}

		pos, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, err
		}

		size = fi.Size() - pos
	} else {
		panic(fmt.Sprintf("Save() got passed a reader without a method to determine the data size, type is %T", rd))
	}
	return size, nil
}

// preventCloser wraps an io.Reader to run a function instead of the original Close() function.
type preventCloser struct {
	io.Reader
	f func()
}

func (wr preventCloser) Close() error {
	wr.f()
	return nil
}

// Save stores data in the backend at the handle.
func (be *s3) Save(ctx context.Context, h restic.Handle, rd io.Reader) (err error) {
	if err := h.Valid(); err != nil {
		return err
	}

	objName := be.Filename(h)
	size, err := getRemainingSize(rd)
	if err != nil {
		return err
	}

	debug.Log("Save %v at %v", h, objName)

	// Check key does not already exist
	_, err = be.client.StatObject(be.bucketname, objName)
	if err == nil {
		debug.Log("%v already exists", h)
		return errors.New("key already exists")
	}

	be.sem.GetToken()

	// wrap the reader so that net/http client cannot close the reader, return
	// the token instead.
	rd = preventCloser{
		Reader: rd,
		f: func() {
			debug.Log("Close()")
		},
	}

	debug.Log("PutObject(%v, %v)", be.bucketname, objName)
	coreClient := minio.Core{Client: be.client}
	info, err := coreClient.PutObject(be.bucketname, objName, size, rd, nil, nil, nil)

	be.sem.ReleaseToken()
	debug.Log("%v -> %v bytes, err %#v", objName, info.Size, err)

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
func (be *s3) Load(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
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

	objName := be.Filename(h)

	be.sem.GetToken()

	byteRange := fmt.Sprintf("bytes=%d-", offset)
	if length > 0 {
		byteRange = fmt.Sprintf("bytes=%d-%d", offset, offset+int64(length)-1)
	}
	headers := minio.NewGetReqHeaders()
	headers.Add("Range", byteRange)
	debug.Log("Load(%v) send range %v", h, byteRange)

	coreClient := minio.Core{Client: be.client}
	rd, _, err := coreClient.GetObject(be.bucketname, objName, headers)
	if err != nil {
		be.sem.ReleaseToken()
		return nil, err
	}

	closeRd := wrapReader{
		ReadCloser: rd,
		f: func() {
			debug.Log("Close()")
			be.sem.ReleaseToken()
		},
	}

	return closeRd, err
}

// Stat returns information about a blob.
func (be *s3) Stat(ctx context.Context, h restic.Handle) (bi restic.FileInfo, err error) {
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
func (be *s3) Test(ctx context.Context, h restic.Handle) (bool, error) {
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
func (be *s3) Remove(ctx context.Context, h restic.Handle) error {
	objName := be.Filename(h)
	err := be.client.RemoveObject(be.bucketname, objName)
	debug.Log("Remove(%v) at %v -> err %v", h, objName, err)
	return errors.Wrap(err, "client.RemoveObject")
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (be *s3) List(ctx context.Context, t restic.FileType) <-chan string {
	debug.Log("listing %v", t)
	ch := make(chan string)

	prefix := be.Dirname(restic.Handle{Type: t})

	// make sure prefix ends with a slash
	if prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}

	listresp := be.client.ListObjects(be.bucketname, prefix, true, ctx.Done())

	go func() {
		defer close(ch)
		for obj := range listresp {
			m := strings.TrimPrefix(obj.Key, prefix)
			if m == "" {
				continue
			}

			select {
			case ch <- path.Base(m):
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch
}

// Remove keys for a specified backend type.
func (be *s3) removeKeys(ctx context.Context, t restic.FileType) error {
	for key := range be.List(ctx, restic.DataFile) {
		err := be.Remove(ctx, restic.Handle{Type: restic.DataFile, Name: key})
		if err != nil {
			return err
		}
	}

	return nil
}

// Delete removes all restic keys in the bucket. It will not remove the bucket itself.
func (be *s3) Delete(ctx context.Context) error {
	alltypes := []restic.FileType{
		restic.DataFile,
		restic.KeyFile,
		restic.LockFile,
		restic.SnapshotFile,
		restic.IndexFile}

	for _, t := range alltypes {
		err := be.removeKeys(ctx, t)
		if err != nil {
			return nil
		}
	}

	return be.Remove(ctx, restic.Handle{Type: restic.ConfigFile})
}

// Close does nothing
func (be *s3) Close() error { return nil }
