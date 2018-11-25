// Package gs provides a restic backend for Google Cloud Storage.
package gs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/pkg/errors"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	storage "google.golang.org/api/storage/v1"
)

// Backend stores data in a GCS bucket.
//
// The service account used to access the bucket must have these permissions:
//  * storage.objects.create
//  * storage.objects.delete
//  * storage.objects.get
//  * storage.objects.list
type Backend struct {
	service      *storage.Service
	projectID    string
	sem          *backend.Semaphore
	bucketName   string
	prefix       string
	listMaxItems int
	backend.Layout
}

// Ensure that *Backend implements restic.Backend.
var _ restic.Backend = &Backend{}

func getStorageService(rt http.RoundTripper) (*storage.Service, error) {
	// create a new HTTP client
	httpClient := &http.Client{
		Transport: rt,
	}

	// create a now context with the HTTP client stored at the oauth2.HTTPClient key
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)

	// use this context
	client, err := google.DefaultClient(ctx, storage.DevstorageReadWriteScope)
	if err != nil {
		return nil, err
	}

	service, err := storage.New(client)
	if err != nil {
		return nil, err
	}

	return service, nil
}

const defaultListMaxItems = 1000

func open(cfg Config, rt http.RoundTripper) (*Backend, error) {
	debug.Log("open, config %#v", cfg)

	service, err := getStorageService(rt)
	if err != nil {
		return nil, errors.Wrap(err, "getStorageService")
	}

	sem, err := backend.NewSemaphore(cfg.Connections)
	if err != nil {
		return nil, err
	}

	be := &Backend{
		service:    service,
		projectID:  cfg.ProjectID,
		sem:        sem,
		bucketName: cfg.Bucket,
		prefix:     cfg.Prefix,
		Layout: &backend.DefaultLayout{
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
	//
	// A Get call has three typical error cases:
	//
	// * nil: Bucket exists and we have access to the metadata (returned).
	//
	// * 403: Bucket exists and we do not have access to the metadata. We
	// don't have storage.buckets.get permission to the bucket, but we may
	// still be able to access objects in the bucket.
	//
	// * 404: Bucket doesn't exist.
	//
	// Determining if the bucket is accessible is best-effort because the
	// 403 case is ambiguous.
	if _, err := be.service.Buckets.Get(be.bucketName).Do(); err != nil {
		gerr, ok := err.(*googleapi.Error)
		if !ok {
			// Don't know what to do with this error.
			return nil, errors.Wrap(err, "service.Buckets.Get")
		}

		switch gerr.Code {
		case 403:
			// Bucket exists, but we don't know if it is
			// accessible. Optimistically assume it is; if not,
			// future Backend calls will fail.
			debug.Log("Unable to determine if bucket %s is accessible (err %v). Continuing as if it is.", be.bucketName, err)
		case 404:
			// Bucket doesn't exist, try to create it.
			bucket := &storage.Bucket{
				Name: be.bucketName,
			}

			if _, err := be.service.Buckets.Insert(be.projectID, bucket).Do(); err != nil {
				// Always an error, as the bucket definitely
				// doesn't exist.
				return nil, errors.Wrap(err, "service.Buckets.Insert")
			}
		default:
			// Don't know what to do with this error.
			return nil, errors.Wrap(err, "service.Buckets.Get")
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

	if os.IsNotExist(err) {
		return true
	}

	if er, ok := err.(*googleapi.Error); ok {
		if er.Code == 404 {
			return true
		}
	}

	return false
}

// Join combines path components with slashes.
func (be *Backend) Join(p ...string) string {
	return path.Join(p...)
}

// Location returns this backend's location (the bucket name).
func (be *Backend) Location() string {
	return be.Join(be.bucketName, be.prefix)
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
	cs := googleapi.ChunkSize(0)

	info, err := be.service.Objects.Insert(be.bucketName,
		&storage.Object{
			Name: objName,
			Size: uint64(rd.Length()),
		}).Media(rd, cs).Do()

	be.sem.ReleaseToken()

	if err != nil {
		debug.Log("%v: err %#v: %v", objName, err, err)
		return errors.Wrap(err, "service.Objects.Insert")
	}

	debug.Log("%v -> %v bytes", objName, info.Size)
	return nil
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

	be.sem.GetToken()

	var byteRange string
	if length > 0 {
		byteRange = fmt.Sprintf("bytes=%d-%d", offset, offset+int64(length-1))
	} else {
		byteRange = fmt.Sprintf("bytes=%d-", offset)
	}

	req := be.service.Objects.Get(be.bucketName, objName)
	// https://cloud.google.com/storage/docs/json_api/v1/parameters#range
	req.Header().Set("Range", byteRange)
	res, err := req.Download()
	if err != nil {
		be.sem.ReleaseToken()
		return nil, err
	}

	closeRd := wrapReader{
		ReadCloser: res.Body,
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

	be.sem.GetToken()
	obj, err := be.service.Objects.Get(be.bucketName, objName).Do()
	be.sem.ReleaseToken()

	if err != nil {
		debug.Log("GetObject() err %v", err)
		return restic.FileInfo{}, errors.Wrap(err, "service.Objects.Get")
	}

	return restic.FileInfo{Size: int64(obj.Size), Name: h.Name}, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (be *Backend) Test(ctx context.Context, h restic.Handle) (bool, error) {
	found := false
	objName := be.Filename(h)

	be.sem.GetToken()
	_, err := be.service.Objects.Get(be.bucketName, objName).Do()
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
	err := be.service.Objects.Delete(be.bucketName, objName).Do()
	be.sem.ReleaseToken()

	if er, ok := err.(*googleapi.Error); ok {
		if er.Code == 404 {
			err = nil
		}
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

	listReq := be.service.Objects.List(be.bucketName).Context(ctx).Prefix(prefix).MaxResults(int64(be.listMaxItems))
	for {
		be.sem.GetToken()
		obj, err := listReq.Do()
		be.sem.ReleaseToken()

		if err != nil {
			return err
		}

		debug.Log("returned %v items", len(obj.Items))

		for _, item := range obj.Items {
			m := strings.TrimPrefix(item.Name, prefix)
			if m == "" {
				continue
			}

			if ctx.Err() != nil {
				return ctx.Err()
			}

			fi := restic.FileInfo{
				Name: path.Base(m),
				Size: int64(item.Size),
			}

			err := fn(fi)
			if err != nil {
				return err
			}

			if ctx.Err() != nil {
				return ctx.Err()
			}
		}

		if obj.NextPageToken == "" {
			break
		}
		listReq.PageToken(obj.NextPageToken)
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

// Close does nothing.
func (be *Backend) Close() error { return nil }
