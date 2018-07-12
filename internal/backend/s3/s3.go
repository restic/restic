package s3

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/minio/minio-go"
	"github.com/minio/minio-go/pkg/credentials"

	"github.com/restic/restic/internal/debug"
)

// Backend stores data on an S3 endpoint.
type Backend struct {
	client *minio.Client
	sem    *backend.Semaphore
	cfg    Config
	backend.Layout
}

// make sure that *Backend implements backend.Backend
var _ restic.Backend = &Backend{}

const defaultLayout = "default"

func open(cfg Config, rt http.RoundTripper) (*Backend, error) {
	debug.Log("open, config %#v", cfg)

	if cfg.MaxRetries > 0 {
		minio.MaxRetry = int(cfg.MaxRetries)
	}

	// Chains all credential types, in the following order:
	// 	- Static credentials provided by user
	//	- AWS env vars (i.e. AWS_ACCESS_KEY_ID)
	//  - Minio env vars (i.e. MINIO_ACCESS_KEY)
	//  - AWS creds file (i.e. AWS_SHARED_CREDENTIALS_FILE or ~/.aws/credentials)
	//  - Minio creds file (i.e. MINIO_SHARED_CREDENTIALS_FILE or ~/.mc/config.json)
	//  - IAM profile based credentials. (performs an HTTP
	//    call to a pre-defined endpoint, only valid inside
	//    configured ec2 instances)
	creds := credentials.NewChainCredentials([]credentials.Provider{
		&credentials.EnvAWS{},
		&credentials.Static{
			Value: credentials.Value{
				AccessKeyID:     cfg.KeyID,
				SecretAccessKey: cfg.Secret,
			},
		},
		&credentials.EnvMinio{},
		&credentials.FileAWSCredentials{},
		&credentials.FileMinioClient{},
		&credentials.IAM{
			Client: &http.Client{
				Transport: http.DefaultTransport,
			},
		},
	})
	client, err := minio.NewWithCredentials(cfg.Endpoint, creds, !cfg.UseHTTP, "")
	if err != nil {
		return nil, errors.Wrap(err, "minio.NewWithCredentials")
	}

	sem, err := backend.NewSemaphore(cfg.Connections)
	if err != nil {
		return nil, err
	}

	be := &Backend{
		client: client,
		sem:    sem,
		cfg:    cfg,
	}

	client.SetCustomTransport(rt)

	l, err := backend.ParseLayout(be, cfg.Layout, defaultLayout, cfg.Prefix)
	if err != nil {
		return nil, err
	}

	be.Layout = l

	return be, nil
}

// Open opens the S3 backend at bucket and region. The bucket is created if it
// does not exist yet.
func Open(cfg Config, rt http.RoundTripper) (restic.Backend, error) {
	return open(cfg, rt)
}

// Create opens the S3 backend at bucket and region and creates the bucket if
// it does not exist yet.
func Create(cfg Config, rt http.RoundTripper) (restic.Backend, error) {
	be, err := open(cfg, rt)
	if err != nil {
		return nil, errors.Wrap(err, "open")
	}
	found, err := be.client.BucketExists(cfg.Bucket)

	if err != nil && be.IsAccessDenied(err) {
		err = nil
		found = true
	}

	if err != nil {
		debug.Log("BucketExists(%v) returned err %v", cfg.Bucket, err)
		return nil, errors.Wrap(err, "client.BucketExists")
	}

	if !found {
		// create new bucket with default ACL in default region
		err = be.client.MakeBucket(cfg.Bucket, "")
		if err != nil {
			return nil, errors.Wrap(err, "client.MakeBucket")
		}
	}

	return be, nil
}

// IsAccessDenied returns true if the error is caused by Access Denied.
func (be *Backend) IsAccessDenied(err error) bool {
	debug.Log("IsAccessDenied(%T, %#v)", err, err)

	if e, ok := errors.Cause(err).(minio.ErrorResponse); ok && e.Code == "AccessDenied" {
		return true
	}

	return false
}

// IsNotExist returns true if the error is caused by a not existing file.
func (be *Backend) IsNotExist(err error) bool {
	debug.Log("IsNotExist(%T, %#v)", err, err)
	if os.IsNotExist(errors.Cause(err)) {
		return true
	}

	if e, ok := errors.Cause(err).(minio.ErrorResponse); ok && e.Code == "NoSuchKey" {
		return true
	}

	return false
}

// Join combines path components with slashes.
func (be *Backend) Join(p ...string) string {
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
func (be *Backend) ReadDir(dir string) (list []os.FileInfo, err error) {
	debug.Log("ReadDir(%v)", dir)

	// make sure dir ends with a slash
	if dir[len(dir)-1] != '/' {
		dir += "/"
	}

	done := make(chan struct{})
	defer close(done)

	for obj := range be.client.ListObjects(be.cfg.Bucket, dir, false, done) {
		if obj.Err != nil {
			return nil, err
		}

		if obj.Key == "" {
			continue
		}

		name := strings.TrimPrefix(obj.Key, dir)
		// Sometimes s3 returns an entry for the dir itself. Ignore it.
		if name == "" {
			continue
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
func (be *Backend) Location() string {
	return be.Join(be.cfg.Bucket, be.cfg.Prefix)
}

// Path returns the path in the bucket that is used for this backend.
func (be *Backend) Path() string {
	return be.cfg.Prefix
}

// lenForFile returns the length of the file.
func lenForFile(f *os.File) (int64, error) {
	fi, err := f.Stat()
	if err != nil {
		return 0, errors.Wrap(err, "Stat")
	}

	pos, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, errors.Wrap(err, "Seek")
	}

	size := fi.Size() - pos
	return size, nil
}

// Save stores data in the backend at the handle.
func (be *Backend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	debug.Log("Save %v", h)

	if err := h.Valid(); err != nil {
		return err
	}

	objName := be.Filename(h)

	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	opts := minio.PutObjectOptions{}
	opts.ContentType = "application/octet-stream"

	debug.Log("PutObject(%v, %v, %v)", be.cfg.Bucket, objName, rd.Length())
	n, err := be.client.PutObjectWithContext(ctx, be.cfg.Bucket, objName, ioutil.NopCloser(rd), int64(rd.Length()), opts)

	debug.Log("%v -> %v bytes, err %#v: %v", objName, n, err, err)

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

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (be *Backend) Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	return backend.DefaultLoad(ctx, h, length, offset, be.openReader, fn)
}

func (be *Backend) openReader(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
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
	opts := minio.GetObjectOptions{}

	var err error
	if length > 0 {
		debug.Log("range: %v-%v", offset, offset+int64(length)-1)
		err = opts.SetRange(offset, offset+int64(length)-1)
	} else if offset > 0 {
		debug.Log("range: %v-", offset)
		err = opts.SetRange(offset, 0)
	}

	if err != nil {
		return nil, errors.Wrap(err, "SetRange")
	}

	be.sem.GetToken()
	coreClient := minio.Core{Client: be.client}
	rd, err := coreClient.GetObjectWithContext(ctx, be.cfg.Bucket, objName, opts)
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
func (be *Backend) Stat(ctx context.Context, h restic.Handle) (bi restic.FileInfo, err error) {
	debug.Log("%v", h)

	objName := be.Filename(h)
	var obj *minio.Object

	opts := minio.GetObjectOptions{}

	be.sem.GetToken()
	obj, err = be.client.GetObjectWithContext(ctx, be.cfg.Bucket, objName, opts)
	if err != nil {
		debug.Log("GetObject() err %v", err)
		be.sem.ReleaseToken()
		return restic.FileInfo{}, errors.Wrap(err, "client.GetObject")
	}

	// make sure that the object is closed properly.
	defer func() {
		e := obj.Close()
		be.sem.ReleaseToken()
		if err == nil {
			err = errors.Wrap(e, "Close")
		}
	}()

	fi, err := obj.Stat()
	if err != nil {
		debug.Log("Stat() err %v", err)
		return restic.FileInfo{}, errors.Wrap(err, "Stat")
	}

	return restic.FileInfo{Size: fi.Size, Name: h.Name}, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (be *Backend) Test(ctx context.Context, h restic.Handle) (bool, error) {
	found := false
	objName := be.Filename(h)

	be.sem.GetToken()
	_, err := be.client.StatObject(be.cfg.Bucket, objName, minio.StatObjectOptions{})
	be.sem.ReleaseToken()

	if err == nil {
		found = true
	}

	// If error, then not found
	return found, nil
}

// Remove removes the blob with the given name and type.
func (be *Backend) Remove(ctx context.Context, h restic.Handle) error {
	objName := be.Filename(h)

	be.sem.GetToken()
	err := be.client.RemoveObject(be.cfg.Bucket, objName)
	be.sem.ReleaseToken()

	debug.Log("Remove(%v) at %v -> err %v", h, objName, err)

	if be.IsNotExist(err) {
		err = nil
	}

	return errors.Wrap(err, "client.RemoveObject")
}

// List runs fn for each file in the backend which has the type t. When an
// error occurs (or fn returns an error), List stops and returns it.
func (be *Backend) List(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error {
	debug.Log("listing %v", t)

	prefix, recursive := be.Basedir(t)

	// make sure prefix ends with a slash
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// NB: unfortunately we can't protect this with be.sem.GetToken() here.
	// Doing so would enable a deadlock situation (gh-1399), as ListObjects()
	// starts its own goroutine and returns results via a channel.
	listresp := be.client.ListObjects(be.cfg.Bucket, prefix, recursive, ctx.Done())

	for obj := range listresp {
		if obj.Err != nil {
			return obj.Err
		}

		m := strings.TrimPrefix(obj.Key, prefix)
		if m == "" {
			continue
		}

		fi := restic.FileInfo{
			Name: path.Base(m),
			Size: obj.Size,
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := fn(fi)
		if err != nil {
			return err
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return ctx.Err()
}

// Remove keys for a specified backend type.
func (be *Backend) removeKeys(ctx context.Context, t restic.FileType) error {
	return be.List(ctx, restic.DataFile, func(fi restic.FileInfo) error {
		return be.Remove(ctx, restic.Handle{Type: t, Name: fi.Name})
	})
}

// Delete removes all restic keys in the bucket. It will not remove the bucket itself.
func (be *Backend) Delete(ctx context.Context) error {
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
func (be *Backend) Close() error { return nil }

// Rename moves a file based on the new layout l.
func (be *Backend) Rename(h restic.Handle, l backend.Layout) error {
	debug.Log("Rename %v to %v", h, l)
	oldname := be.Filename(h)
	newname := l.Filename(h)

	if oldname == newname {
		debug.Log("  %v is already renamed", newname)
		return nil
	}

	debug.Log("  %v -> %v", oldname, newname)

	src := minio.NewSourceInfo(be.cfg.Bucket, oldname, nil)

	dst, err := minio.NewDestinationInfo(be.cfg.Bucket, newname, nil, nil)
	if err != nil {
		return errors.Wrap(err, "NewDestinationInfo")
	}

	err = be.client.CopyObject(dst, src)
	if err != nil && be.IsNotExist(err) {
		debug.Log("copy failed: %v, seems to already have been renamed", err)
		return nil
	}

	if err != nil {
		debug.Log("copy failed: %v", err)
		return err
	}

	return be.client.RemoveObject(be.cfg.Bucket, oldname)
}
