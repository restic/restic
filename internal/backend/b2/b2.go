package b2

import (
	"context"
	"hash"
	"io"
	"net/http"
	"path"
	"sync"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/sema"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/cenkalti/backoff/v4"
	"github.com/kurin/blazer/b2"
)

// b2Backend is a backend which stores its data on Backblaze B2.
type b2Backend struct {
	client       *b2.Client
	bucket       *b2.Bucket
	cfg          Config
	listMaxItems int
	backend.Layout
	sem sema.Semaphore
}

// Billing happens in 1000 item granlarity, but we are more interested in reducing the number of network round trips
const defaultListMaxItems = 10 * 1000

// ensure statically that *b2Backend implements restic.Backend.
var _ restic.Backend = &b2Backend{}

type sniffingRoundTripper struct {
	sync.Mutex
	lastErr error
	http.RoundTripper
}

func (s *sniffingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := s.RoundTripper.RoundTrip(req)
	if err != nil {
		s.Lock()
		s.lastErr = err
		s.Unlock()
	}
	return res, err
}

func newClient(ctx context.Context, cfg Config, rt http.RoundTripper) (*b2.Client, error) {
	sniffer := &sniffingRoundTripper{RoundTripper: rt}
	opts := []b2.ClientOption{b2.Transport(sniffer)}

	// if the connection B2 fails, this can cause the client to hang
	// cancel the connection after a minute to at least provide some feedback to the user
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	c, err := b2.NewClient(ctx, cfg.AccountID, cfg.Key.Unwrap(), opts...)
	if err == context.DeadlineExceeded {
		if sniffer.lastErr != nil {
			return nil, sniffer.lastErr
		}
		return nil, errors.New("connection to B2 failed")
	} else if err != nil {
		return nil, errors.Wrap(err, "b2.NewClient")
	}
	return c, nil
}

// Open opens a connection to the B2 service.
func Open(ctx context.Context, cfg Config, rt http.RoundTripper) (restic.Backend, error) {
	debug.Log("cfg %#v", cfg)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	client, err := newClient(ctx, cfg, rt)
	if err != nil {
		return nil, err
	}

	bucket, err := client.Bucket(ctx, cfg.Bucket)
	if err != nil {
		return nil, errors.Wrap(err, "Bucket")
	}

	sem, err := sema.New(cfg.Connections)
	if err != nil {
		return nil, err
	}

	be := &b2Backend{
		client: client,
		bucket: bucket,
		cfg:    cfg,
		Layout: &backend.DefaultLayout{
			Join: path.Join,
			Path: cfg.Prefix,
		},
		listMaxItems: defaultListMaxItems,
		sem:          sem,
	}

	return be, nil
}

// Create opens a connection to the B2 service. If the bucket does not exist yet,
// it is created.
func Create(ctx context.Context, cfg Config, rt http.RoundTripper) (restic.Backend, error) {
	debug.Log("cfg %#v", cfg)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	client, err := newClient(ctx, cfg, rt)
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

	sem, err := sema.New(cfg.Connections)
	if err != nil {
		return nil, err
	}

	be := &b2Backend{
		client: client,
		bucket: bucket,
		cfg:    cfg,
		Layout: &backend.DefaultLayout{
			Join: path.Join,
			Path: cfg.Prefix,
		},
		listMaxItems: defaultListMaxItems,
		sem:          sem,
	}

	present, err := be.Test(ctx, restic.Handle{Type: restic.ConfigFile})
	if err != nil {
		return nil, err
	}

	if present {
		return nil, errors.New("config already exists")
	}

	return be, nil
}

// SetListMaxItems sets the number of list items to load per request.
func (be *b2Backend) SetListMaxItems(i int) {
	be.listMaxItems = i
}

func (be *b2Backend) Connections() uint {
	return be.cfg.Connections
}

// Location returns the location for the backend.
func (be *b2Backend) Location() string {
	return be.cfg.Bucket
}

// Hasher may return a hash function for calculating a content hash for the backend
func (be *b2Backend) Hasher() hash.Hash {
	return nil
}

// HasAtomicReplace returns whether Save() can atomically replace files
func (be *b2Backend) HasAtomicReplace() bool {
	return true
}

// IsNotExist returns true if the error is caused by a non-existing file.
func (be *b2Backend) IsNotExist(err error) bool {
	return b2.IsNotExist(errors.Cause(err))
}

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (be *b2Backend) Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	return backend.DefaultLoad(ctx, h, length, offset, be.openReader, fn)
}

func (be *b2Backend) openReader(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
	debug.Log("Load %v, length %v, offset %v from %v", h, length, offset, be.Filename(h))
	if err := h.Valid(); err != nil {
		return nil, backoff.Permanent(err)
	}

	if offset < 0 {
		return nil, errors.New("offset is negative")
	}

	if length < 0 {
		return nil, errors.Errorf("invalid length %d", length)
	}

	ctx, cancel := context.WithCancel(ctx)

	be.sem.GetToken()

	name := be.Layout.Filename(h)
	obj := be.bucket.Object(name)

	if offset == 0 && length == 0 {
		rd := obj.NewReader(ctx)
		return be.sem.ReleaseTokenOnClose(rd, cancel), nil
	}

	// pass a negative length to NewRangeReader so that the remainder of the
	// file is read.
	if length == 0 {
		length = -1
	}

	rd := obj.NewRangeReader(ctx, offset, int64(length))
	return be.sem.ReleaseTokenOnClose(rd, cancel), nil
}

// Save stores data in the backend at the handle.
func (be *b2Backend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := h.Valid(); err != nil {
		return backoff.Permanent(err)
	}

	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	name := be.Filename(h)
	debug.Log("Save %v, name %v", h, name)
	obj := be.bucket.Object(name)

	// b2 always requires sha1 checksums for uploaded file parts
	w := obj.NewWriter(ctx)
	n, err := io.Copy(w, rd)
	debug.Log("  saved %d bytes, err %v", n, err)

	if err != nil {
		_ = w.Close()
		return errors.Wrap(err, "Copy")
	}

	// sanity check
	if n != rd.Length() {
		return errors.Errorf("wrote %d bytes instead of the expected %d bytes", n, rd.Length())
	}
	return errors.Wrap(w.Close(), "Close")
}

// Stat returns information about a blob.
func (be *b2Backend) Stat(ctx context.Context, h restic.Handle) (bi restic.FileInfo, err error) {
	debug.Log("Stat %v", h)

	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	name := be.Filename(h)
	obj := be.bucket.Object(name)
	info, err := obj.Attrs(ctx)
	if err != nil {
		debug.Log("Attrs() err %v", err)
		return restic.FileInfo{}, errors.Wrap(err, "Stat")
	}
	return restic.FileInfo{Size: info.Size, Name: h.Name}, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (be *b2Backend) Test(ctx context.Context, h restic.Handle) (bool, error) {
	debug.Log("Test %v", h)

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
func (be *b2Backend) Remove(ctx context.Context, h restic.Handle) error {
	debug.Log("Remove %v", h)

	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	// the retry backend will also repeat the remove method up to 10 times
	for i := 0; i < 3; i++ {
		obj := be.bucket.Object(be.Filename(h))
		err := obj.Delete(ctx)
		if err == nil {
			// keep deleting until we are sure that no leftover file versions exist
			continue
		}
		// consider a file as removed if b2 informs us that it does not exist
		if b2.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "Delete")
	}

	return errors.New("failed to delete all file versions")
}

type semLocker struct {
	sema.Semaphore
}

func (sm *semLocker) Lock()   { sm.GetToken() }
func (sm *semLocker) Unlock() { sm.ReleaseToken() }

// List returns a channel that yields all names of blobs of type t.
func (be *b2Backend) List(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error {
	debug.Log("List %v", t)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	prefix, _ := be.Basedir(t)
	iter := be.bucket.List(ctx, b2.ListPrefix(prefix), b2.ListPageSize(be.listMaxItems), b2.ListLocker(&semLocker{be.sem}))

	for iter.Next() {
		obj := iter.Object()

		attrs, err := obj.Attrs(ctx)
		if err != nil {
			return err
		}

		fi := restic.FileInfo{
			Name: path.Base(obj.Name()),
			Size: attrs.Size,
		}

		if err := fn(fi); err != nil {
			return err
		}
	}
	if err := iter.Err(); err != nil {
		debug.Log("List: %v", err)
		return err
	}
	return nil
}

// Remove keys for a specified backend type.
func (be *b2Backend) removeKeys(ctx context.Context, t restic.FileType) error {
	debug.Log("removeKeys %v", t)
	return be.List(ctx, t, func(fi restic.FileInfo) error {
		return be.Remove(ctx, restic.Handle{Type: t, Name: fi.Name})
	})
}

// Delete removes all restic keys in the bucket. It will not remove the bucket itself.
func (be *b2Backend) Delete(ctx context.Context) error {
	alltypes := []restic.FileType{
		restic.PackFile,
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
	err := be.Remove(ctx, restic.Handle{Type: restic.ConfigFile})
	if err != nil && b2.IsNotExist(errors.Cause(err)) {
		err = nil
	}

	return err
}

// Close does nothing
func (be *b2Backend) Close() error { return nil }
