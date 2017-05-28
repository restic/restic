package b2

import (
	"context"
	"io"
	"path"
	"restic"
	"strings"

	"restic/backend"
	"restic/debug"
	"restic/errors"

	"github.com/kurin/blazer/b2"
)

// b2Backend is a backend which stores its data on Backblaze B2.
type b2Backend struct {
	client *b2.Client
	bucket *b2.Bucket
	cfg    Config
	backend.Layout
	sem *backend.Semaphore
}

func newClient(ctx context.Context, cfg Config) (*b2.Client, error) {
	opts := []b2.ClientOption{b2.Transport(backend.Transport())}

	c, err := b2.NewClient(ctx, cfg.AccountID, cfg.Key, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "b2.NewClient")
	}
	return c, nil
}

// Open opens a connection to the B2 service.
func Open(cfg Config) (restic.Backend, error) {
	debug.Log("cfg %#v", cfg)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	client, err := newClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	bucket, err := client.Bucket(ctx, cfg.Bucket)
	if err != nil {
		return nil, errors.Wrap(err, "Bucket")
	}

	be := &b2Backend{
		client: client,
		bucket: bucket,
		cfg:    cfg,
		Layout: &backend.DefaultLayout{
			Join: path.Join,
			Path: cfg.Prefix,
		},
		sem: backend.NewSemaphore(cfg.Connections),
	}

	return be, nil
}

// Create opens a connection to the B2 service. If the bucket does not exist yet,
// it is created.
func Create(cfg Config) (restic.Backend, error) {
	debug.Log("cfg %#v", cfg)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	client, err := newClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	attr := b2.BucketAttrs{
		Type: b2.Private,
	}
	bucket, err := client.NewBucket(ctx, cfg.Bucket, &attr)
	if err != nil {
		return nil, errors.Wrap(err, "NewBucket")
	}

	be := &b2Backend{
		client: client,
		bucket: bucket,
		cfg:    cfg,
		Layout: &backend.DefaultLayout{
			Join: path.Join,
			Path: cfg.Prefix,
		},
		sem: backend.NewSemaphore(cfg.Connections),
	}

	present, err := be.Test(restic.Handle{Type: restic.ConfigFile})
	if err != nil {
		return nil, err
	}

	if present {
		return nil, errors.New("config already exists")
	}

	return be, nil
}

// Location returns the location for the backend.
func (be *b2Backend) Location() string {
	return be.cfg.Bucket
}

// wrapReader wraps an io.ReadCloser to run an additional function on Close.
type wrapReader struct {
	io.ReadCloser
	eofSeen bool
	f       func()
}

func (wr *wrapReader) Read(p []byte) (int, error) {
	if wr.eofSeen {
		return 0, io.EOF
	}

	n, err := wr.ReadCloser.Read(p)
	if err == io.EOF {
		wr.eofSeen = true
	}
	return n, err
}

func (wr *wrapReader) Close() error {
	err := wr.ReadCloser.Close()
	wr.f()
	return err
}

// Load returns the data stored in the backend for h at the given offset
// and saves it in p. Load has the same semantics as io.ReaderAt.
func (be *b2Backend) Load(h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
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

	ctx, cancel := context.WithCancel(context.TODO())

	be.sem.GetToken()

	name := be.Layout.Filename(h)
	obj := be.bucket.Object(name)

	if offset == 0 && length == 0 {
		rd := obj.NewReader(ctx)
		wrapper := &wrapReader{
			ReadCloser: rd,
			f: func() {
				cancel()
				be.sem.ReleaseToken()
			},
		}
		return wrapper, nil
	}

	// pass a negative length to NewRangeReader so that the remainder of the
	// file is read.
	if length == 0 {
		length = -1
	}

	rd := obj.NewRangeReader(ctx, offset, int64(length))
	wrapper := &wrapReader{
		ReadCloser: rd,
		f: func() {
			cancel()
			be.sem.ReleaseToken()
		},
	}
	return wrapper, nil
}

// Save stores data in the backend at the handle.
func (be *b2Backend) Save(h restic.Handle, rd io.Reader) (err error) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	if err := h.Valid(); err != nil {
		return err
	}

	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	name := be.Filename(h)
	debug.Log("Save %v, name %v", h, name)
	obj := be.bucket.Object(name)

	_, err = obj.Attrs(ctx)
	if err == nil {
		debug.Log("  %v already exists", h)
		return errors.New("key already exists")
	}

	w := obj.NewWriter(ctx)
	n, err := io.Copy(w, rd)
	debug.Log("  saved %d bytes, err %v", n, err)

	if err != nil {
		_ = w.Close()
		return errors.Wrap(err, "Copy")
	}

	return errors.Wrap(w.Close(), "Close")
}

// Stat returns information about a blob.
func (be *b2Backend) Stat(h restic.Handle) (bi restic.FileInfo, err error) {
	debug.Log("Stat %v", h)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	name := be.Filename(h)
	obj := be.bucket.Object(name)
	info, err := obj.Attrs(ctx)
	if err != nil {
		debug.Log("Attrs() err %v", err)
		return restic.FileInfo{}, errors.Wrap(err, "Stat")
	}
	return restic.FileInfo{Size: info.Size}, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (be *b2Backend) Test(h restic.Handle) (bool, error) {
	debug.Log("Test %v", h)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	found := false
	name := be.Filename(h)
	obj := be.bucket.Object(name)
	info, err := obj.Attrs(ctx)
	if err == nil && info != nil && info.Status == b2.Uploaded {
		found = true
	}
	return found, nil
}

// Remove removes the blob with the given name and type.
func (be *b2Backend) Remove(h restic.Handle) error {
	debug.Log("Remove %v", h)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	obj := be.bucket.Object(be.Filename(h))
	return errors.Wrap(obj.Delete(ctx), "Delete")
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (be *b2Backend) List(t restic.FileType, done <-chan struct{}) <-chan string {
	debug.Log("List %v", t)
	ch := make(chan string)

	ctx, cancel := context.WithCancel(context.TODO())

	be.sem.GetToken()

	go func() {
		defer close(ch)
		defer cancel()
		defer be.sem.ReleaseToken()

		prefix := be.Dirname(restic.Handle{Type: t})
		cur := &b2.Cursor{Prefix: prefix}

		for {
			objs, c, err := be.bucket.ListCurrentObjects(ctx, 1000, cur)
			if err != nil && err != io.EOF {
				return
			}
			for _, obj := range objs {
				// Skip objects returned that do not have the specified prefix.
				if !strings.HasPrefix(obj.Name(), prefix) {
					continue
				}

				m := path.Base(obj.Name())
				if m == "" {
					continue
				}

				select {
				case ch <- m:
				case <-done:
					return
				}
			}
			if err == io.EOF {
				return
			}
			cur = c
		}
	}()

	return ch
}

// Remove keys for a specified backend type.
func (be *b2Backend) removeKeys(t restic.FileType) error {
	debug.Log("removeKeys %v", t)

	done := make(chan struct{})
	defer close(done)
	for key := range be.List(t, done) {
		err := be.Remove(restic.Handle{Type: t, Name: key})
		if err != nil {
			return err
		}
	}
	return nil
}

// Delete removes all restic keys in the bucket. It will not remove the bucket itself.
func (be *b2Backend) Delete() error {
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
	err := be.Remove(restic.Handle{Type: restic.ConfigFile})
	if err != nil && b2.IsNotExist(errors.Cause(err)) {
		err = nil
	}

	return err
}

// Close does nothing
func (be *b2Backend) Close() error { return nil }
