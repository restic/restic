package b2

import (
	"context"
	"fmt"
	"hash"
	"io"
	"net/http"
	"path"
	"sync"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/layout"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/backend/util"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"

	"github.com/Backblaze/blazer/b2"
	"github.com/Backblaze/blazer/base"
)

// b2Backend is a backend which stores its data on Backblaze B2.
type b2Backend struct {
	client       *b2.Client
	bucket       *b2.Bucket
	cfg          Config
	listMaxItems int
	layout.Layout

	canDelete bool
}

var errTooShort = fmt.Errorf("file is too short")

// Billing happens in 1000 item granularity, but we are more interested in reducing the number of network round trips
const defaultListMaxItems = 10 * 1000

// ensure statically that *b2Backend implements backend.Backend.
var _ backend.Backend = &b2Backend{}

func NewFactory() location.Factory {
	return location.NewHTTPBackendFactory("b2", ParseConfig, location.NoPassword, Create, Open)
}

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
	if cfg.AccountID == "" {
		return nil, errors.Fatalf("unable to open B2 backend: Account ID ($B2_ACCOUNT_ID) is empty")
	}
	if cfg.Key.String() == "" {
		return nil, errors.Fatalf("unable to open B2 backend: Key ($B2_ACCOUNT_KEY) is empty")
	}

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
func Open(ctx context.Context, cfg Config, rt http.RoundTripper) (backend.Backend, error) {
	debug.Log("cfg %#v", cfg)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	client, err := newClient(ctx, cfg, rt)
	if err != nil {
		return nil, err
	}

	bucket, err := client.Bucket(ctx, cfg.Bucket)
	if b2.IsNotExist(err) {
		return nil, backend.ErrNoRepository
	} else if err != nil {
		return nil, errors.Wrap(err, "Bucket")
	}

	be := &b2Backend{
		client: client,
		bucket: bucket,
		cfg:    cfg,
		Layout: &layout.DefaultLayout{
			Join: path.Join,
			Path: cfg.Prefix,
		},
		listMaxItems: defaultListMaxItems,
		canDelete:    true,
	}

	return be, nil
}

// Create opens a connection to the B2 service. If the bucket does not exist yet,
// it is created.
func Create(ctx context.Context, cfg Config, rt http.RoundTripper) (backend.Backend, error) {
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

	be := &b2Backend{
		client: client,
		bucket: bucket,
		cfg:    cfg,
		Layout: &layout.DefaultLayout{
			Join: path.Join,
			Path: cfg.Prefix,
		},
		listMaxItems: defaultListMaxItems,
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
	// blazer/b2 does not export its error types and values,
	// so we can't use errors.{As,Is}.
	for ; err != nil; err = errors.Unwrap(err) {
		if b2.IsNotExist(err) {
			return true
		}
	}
	return false
}

func (be *b2Backend) IsPermanentError(err error) bool {
	// the library unfortunately endlessly retries authentication errors
	return be.IsNotExist(err) || errors.Is(err, errTooShort)
}

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (be *b2Backend) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	return util.DefaultLoad(ctx, h, length, offset, be.openReader, func(rd io.Reader) error {
		if length == 0 {
			return fn(rd)
		}

		// there is no direct way to efficiently check whether the file is too short
		// use a LimitedReader to track the number of bytes read
		limrd := &io.LimitedReader{R: rd, N: int64(length)}
		err := fn(limrd)

		// check the underlying reader to be agnostic to however fn() handles the returned error
		_, rderr := rd.Read([]byte{0})
		if rderr == io.EOF && limrd.N != 0 {
			// file is too short
			return fmt.Errorf("%w: %v", errTooShort, err)
		}

		return err
	})
}

func (be *b2Backend) openReader(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
	name := be.Layout.Filename(h)
	obj := be.bucket.Object(name)

	if offset == 0 && length == 0 {
		return obj.NewReader(ctx), nil
	}

	// pass a negative length to NewRangeReader so that the remainder of the
	// file is read.
	if length == 0 {
		length = -1
	}

	return obj.NewRangeReader(ctx, offset, int64(length)), nil
}

// Save stores data in the backend at the handle.
func (be *b2Backend) Save(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	name := be.Filename(h)
	obj := be.bucket.Object(name)

	// b2 always requires sha1 checksums for uploaded file parts
	w := obj.NewWriter(ctx)
	n, err := io.Copy(w, rd)

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
func (be *b2Backend) Stat(ctx context.Context, h backend.Handle) (bi backend.FileInfo, err error) {
	name := be.Filename(h)
	obj := be.bucket.Object(name)
	info, err := obj.Attrs(ctx)
	if err != nil {
		return backend.FileInfo{}, errors.Wrap(err, "Stat")
	}
	return backend.FileInfo{Size: info.Size, Name: h.Name}, nil
}

// Remove removes the blob with the given name and type.
func (be *b2Backend) Remove(ctx context.Context, h backend.Handle) error {
	// the retry backend will also repeat the remove method up to 10 times
	for i := 0; i < 3; i++ {
		obj := be.bucket.Object(be.Filename(h))

		var err error
		if be.canDelete {
			err = obj.Delete(ctx)
			if err == nil {
				// keep deleting until we are sure that no leftover file versions exist
				continue
			}

			code, _ := base.Code(err)
			if code == 401 { // unauthorized
				// fallback to hide if not allowed to delete files
				be.canDelete = false
				debug.Log("Removing %v failed, falling back to b2_hide_file.", h)
				continue
			}
		} else {
			// hide adds a new file version hiding all older ones, thus retries are not necessary
			err = obj.Hide(ctx)
		}

		// consider a file as removed if b2 informs us that it does not exist
		if b2.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "Delete")
	}

	return errors.New("failed to delete all file versions")
}

// List returns a channel that yields all names of blobs of type t.
func (be *b2Backend) List(ctx context.Context, t backend.FileType, fn func(backend.FileInfo) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	prefix, _ := be.Basedir(t)
	iter := be.bucket.List(ctx, b2.ListPrefix(prefix), b2.ListPageSize(be.listMaxItems))

	for iter.Next() {
		obj := iter.Object()

		attrs, err := obj.Attrs(ctx)
		if err != nil {
			return err
		}

		fi := backend.FileInfo{
			Name: path.Base(obj.Name()),
			Size: attrs.Size,
		}

		if err := fn(fi); err != nil {
			return err
		}
	}
	return iter.Err()
}

// Delete removes all restic keys in the bucket. It will not remove the bucket itself.
func (be *b2Backend) Delete(ctx context.Context) error {
	return util.DefaultDelete(ctx, be)
}

// Close does nothing
func (be *b2Backend) Close() error { return nil }
