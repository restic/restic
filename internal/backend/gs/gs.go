// Package gs provides a restic backend for Google Cloud Storage.
package gs

import (
	"context"
	"crypto/md5"
	"hash"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/pkg/errors"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/layout"
	"github.com/restic/restic/internal/backend/sema"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// Backend stores data in a GCS bucket.
//
// The service account used to access the bucket must have these permissions:
//   - storage.objects.create
//   - storage.objects.delete
//   - storage.objects.get
//   - storage.objects.list
type Backend struct {
	gcsClient    *storage.Client
	projectID    string
	connections  uint
	sem          sema.Semaphore
	bucketName   string
	bucket       *storage.BucketHandle
	prefix       string
	listMaxItems int
	layout.Layout
}

// Ensure that *Backend implements restic.Backend.
var _ restic.Backend = &Backend{}

func getStorageClient(rt http.RoundTripper) (*storage.Client, error) {
	// create a new HTTP client
	httpClient := &http.Client{
		Transport: rt,
	}

	// create a new context with the HTTP client stored at the oauth2.HTTPClient key
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)

	var ts oauth2.TokenSource
	if token := os.Getenv("GOOGLE_ACCESS_TOKEN"); token != "" {
		ts = oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: token,
			TokenType:   "Bearer",
		})
	} else {
		var err error
		ts, err = google.DefaultTokenSource(ctx, storage.ScopeReadWrite)
		if err != nil {
			return nil, err
		}
	}

	oauthClient := oauth2.NewClient(ctx, ts)

	gcsClient, err := storage.NewClient(ctx, option.WithHTTPClient(oauthClient))
	if err != nil {
		return nil, err
	}

	return gcsClient, nil
}

func (be *Backend) bucketExists(ctx context.Context, bucket *storage.BucketHandle) (bool, error) {
	_, err := bucket.Attrs(ctx)
	if err == storage.ErrBucketNotExist {
		return false, nil
	}
	return err == nil, err
}

const defaultListMaxItems = 1000

func open(cfg Config, rt http.RoundTripper) (*Backend, error) {
	debug.Log("open, config %#v", cfg)

	gcsClient, err := getStorageClient(rt)
	if err != nil {
		return nil, errors.Wrap(err, "getStorageClient")
	}

	sem, err := sema.New(cfg.Connections)
	if err != nil {
		return nil, err
	}

	be := &Backend{
		gcsClient:   gcsClient,
		projectID:   cfg.ProjectID,
		connections: cfg.Connections,
		sem:         sem,
		bucketName:  cfg.Bucket,
		bucket:      gcsClient.Bucket(cfg.Bucket),
		prefix:      cfg.Prefix,
		Layout: &layout.DefaultLayout{
			Path: cfg.Prefix,
			Join: path.Join,
		},
		listMaxItems: defaultListMaxItems,
	}

	return be, nil
}

// Open opens the gs backend at the specified bucket.
func Open(cfg Config, rt http.RoundTripper) (restic.Backend, error) {
	return open(cfg, rt)
}

// Create opens the gs backend at the specified bucket and attempts to creates
// the bucket if it does not exist yet.
//
// The service account must have the "storage.buckets.create" permission to
// create a bucket the does not yet exist.
func Create(cfg Config, rt http.RoundTripper) (restic.Backend, error) {
	be, err := open(cfg, rt)
	if err != nil {
		return nil, errors.Wrap(err, "open")
	}

	// Try to determine if the bucket exists. If it does not, try to create it.
	ctx := context.Background()
	exists, err := be.bucketExists(ctx, be.bucket)
	if err != nil {
		if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusForbidden {
			// the bucket might exist!
			// however, the client doesn't have storage.bucket.get permission
			return be, nil
		}
		return nil, errors.Wrap(err, "service.Buckets.Get")
	}

	if !exists {
		// Bucket doesn't exist, try to create it.
		if err := be.bucket.Create(ctx, be.projectID, nil); err != nil {
			// Always an error, as the bucket definitely doesn't exist.
			return nil, errors.Wrap(err, "service.Buckets.Insert")
		}

	}

	return be, nil
}

// SetListMaxItems sets the number of list items to load per request.
func (be *Backend) SetListMaxItems(i int) {
	be.listMaxItems = i
}

// IsNotExist returns true if the error is caused by a not existing file.
func (be *Backend) IsNotExist(err error) bool {
	debug.Log("IsNotExist(%T, %#v)", err, err)
	return errors.Is(err, storage.ErrObjectNotExist)
}

// Join combines path components with slashes.
func (be *Backend) Join(p ...string) string {
	return path.Join(p...)
}

func (be *Backend) Connections() uint {
	return be.connections
}

// Location returns this backend's location (the bucket name).
func (be *Backend) Location() string {
	return be.Join(be.bucketName, be.prefix)
}

// Hasher may return a hash function for calculating a content hash for the backend
func (be *Backend) Hasher() hash.Hash {
	return md5.New()
}

// HasAtomicReplace returns whether Save() can atomically replace files
func (be *Backend) HasAtomicReplace() bool {
	return true
}

// Path returns the path in the bucket that is used for this backend.
func (be *Backend) Path() string {
	return be.prefix
}

// Save stores data in the backend at the handle.
func (be *Backend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	if err := h.Valid(); err != nil {
		return err
	}

	objName := be.Filename(h)

	debug.Log("Save %v at %v", h, objName)

	be.sem.GetToken()

	debug.Log("InsertObject(%v, %v)", be.bucketName, objName)

	// Set chunk size to zero to disable resumable uploads.
	//
	// With a non-zero chunk size (the default is
	// googleapi.DefaultUploadChunkSize, 8MB), Insert will buffer data from
	// rd in chunks of this size so it can upload these chunks in
	// individual requests.
	//
	// This chunking allows the library to automatically handle network
	// interruptions and re-upload only the last chunk rather than the full
	// file.
	//
	// Unfortunately, this buffering doesn't play nicely with
	// --limit-upload, which applies a rate limit to rd. This rate limit
	// ends up only limiting the read from rd into the buffer rather than
	// the network traffic itself. This results in poor network rate limit
	// behavior, where individual chunks are written to the network at full
	// bandwidth for several seconds, followed by several seconds of no
	// network traffic as the next chunk is read through the rate limiter.
	//
	// By disabling chunking, rd is passed further down the request stack,
	// where there is less (but some) buffering, which ultimately results
	// in better rate limiting behavior.
	//
	// restic typically writes small blobs (4MB-30MB), so the resumable
	// uploads are not providing significant benefit anyways.
	w := be.bucket.Object(objName).NewWriter(ctx)
	w.ChunkSize = 0
	w.MD5 = rd.Hash()
	wbytes, err := io.Copy(w, rd)
	cerr := w.Close()
	if err == nil {
		err = cerr
	}

	be.sem.ReleaseToken()

	if err != nil {
		debug.Log("%v: err %#v: %v", objName, err, err)
		return errors.Wrap(err, "service.Objects.Insert")
	}

	debug.Log("%v -> %v bytes", objName, wbytes)
	// sanity check
	if wbytes != rd.Length() {
		return errors.Errorf("wrote %d bytes instead of the expected %d bytes", wbytes, rd.Length())
	}
	return nil
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
	if length == 0 {
		// negative length indicates read till end to GCS lib
		length = -1
	}

	objName := be.Filename(h)

	be.sem.GetToken()

	ctx, cancel := context.WithCancel(ctx)

	r, err := be.bucket.Object(objName).NewRangeReader(ctx, offset, int64(length))
	if err != nil {
		cancel()
		be.sem.ReleaseToken()
		return nil, err
	}

	return be.sem.ReleaseTokenOnClose(r, cancel), err
}

// Stat returns information about a blob.
func (be *Backend) Stat(ctx context.Context, h restic.Handle) (bi restic.FileInfo, err error) {
	debug.Log("%v", h)

	objName := be.Filename(h)

	be.sem.GetToken()
	attr, err := be.bucket.Object(objName).Attrs(ctx)
	be.sem.ReleaseToken()

	if err != nil {
		debug.Log("GetObjectAttributes() err %v", err)
		return restic.FileInfo{}, errors.Wrap(err, "service.Objects.Get")
	}

	return restic.FileInfo{Size: attr.Size, Name: h.Name}, nil
}

// Remove removes the blob with the given name and type.
func (be *Backend) Remove(ctx context.Context, h restic.Handle) error {
	objName := be.Filename(h)

	be.sem.GetToken()
	err := be.bucket.Object(objName).Delete(ctx)
	be.sem.ReleaseToken()

	if err == storage.ErrObjectNotExist {
		err = nil
	}

	debug.Log("Remove(%v) at %v -> err %v", h, objName, err)
	return errors.Wrap(err, "client.RemoveObject")
}

// List runs fn for each file in the backend which has the type t. When an
// error occurs (or fn returns an error), List stops and returns it.
func (be *Backend) List(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error {
	debug.Log("listing %v", t)

	prefix, _ := be.Basedir(t)

	// make sure prefix ends with a slash
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	itr := be.bucket.Objects(ctx, &storage.Query{Prefix: prefix})

	for {
		be.sem.GetToken()
		attrs, err := itr.Next()
		be.sem.ReleaseToken()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		m := strings.TrimPrefix(attrs.Name, prefix)
		if m == "" {
			continue
		}

		fi := restic.FileInfo{
			Name: path.Base(m),
			Size: int64(attrs.Size),
		}

		err = fn(fi)
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
	return be.List(ctx, t, func(fi restic.FileInfo) error {
		return be.Remove(ctx, restic.Handle{Type: t, Name: fi.Name})
	})
}

// Delete removes all restic keys in the bucket. It will not remove the bucket itself.
func (be *Backend) Delete(ctx context.Context) error {
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

	return be.Remove(ctx, restic.Handle{Type: restic.ConfigFile})
}

// Close does nothing.
func (be *Backend) Close() error { return nil }
