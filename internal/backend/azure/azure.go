package azure

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	storage "github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/cenkalti/backoff/v4"
)

// Backend stores data on an azure endpoint.
type Backend struct {
	accountName  string
	container    storage.ContainerURL
	sem          *backend.Semaphore
	prefix       string
	listMaxItems int
	backend.Layout
}

const defaultListMaxItems = 5000

// make sure that *Backend implements backend.Backend
var _ restic.Backend = &Backend{}

func getCredential(cfg Config) (*storage.SharedKeyCredential, error) {
	if cfg.AccountKey != "" {
		cred, err := storage.NewSharedKeyCredential(cfg.AccountName, cfg.AccountKey)
		if err != nil {
			return nil, errors.Wrap(err, "Error creating SharedKeyCredential")
		}

		return cred, err
	}

	return nil, errors.New("Not supported")
}

func getBSU(cfg Config) (storage.ServiceURL, error) {
	credential, err := getCredential(cfg)
	if err != nil {
		return storage.ServiceURL{}, err
	}

	pipeline := storage.NewPipeline(credential, storage.PipelineOptions{})
	blobPrimaryURL, err := url.Parse("https://" + credential.AccountName() + ".blob.core.windows.net/")

	if err != nil {
		return storage.ServiceURL{}, err
	}

	return storage.NewServiceURL(*blobPrimaryURL, pipeline), nil
}

func open(cfg Config, rt http.RoundTripper) (*Backend, error) {
	debug.Log("open, config %#v", cfg)

	sem, err := backend.NewSemaphore(cfg.Connections)
	if err != nil {
		return nil, err
	}

	bsu, err := getBSU(cfg)
	if err != nil {
		return nil, err
	}

	be := &Backend{
		accountName: cfg.AccountName,
		container:   bsu.NewContainerURL(cfg.Container),
		sem:         sem,
		prefix:      cfg.Prefix,
		Layout: &backend.DefaultLayout{
			Path: cfg.Prefix,
			Join: path.Join,
		},
		listMaxItems: defaultListMaxItems,
	}

	return be, nil
}

// Open opens the Azure backend at specified container.
func Open(cfg Config, rt http.RoundTripper) (*Backend, error) {
	return open(cfg, rt)
}

// Create opens the Azure backend at specified container and creates the container if
// it does not exist yet.
func Create(cfg Config, rt http.RoundTripper) (*Backend, error) {
	be, err := open(cfg, rt)

	if err != nil {
		return nil, errors.Wrap(err, "open")
	}

	_, err = be.container.Create(context.TODO(), storage.Metadata{}, storage.PublicAccessNone)
	if err != nil {
		if stgErr, ok := err.(storage.StorageError); ok {
			if stgErr.ServiceCode() == storage.ServiceCodeContainerAlreadyExists {
				return be, nil
			}
		}

		return nil, errors.Wrap(err, "container.CreateIfNotExists")
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
	return os.IsNotExist(err)
}

// Join combines path components with slashes.
func (be *Backend) Join(p ...string) string {
	return path.Join(p...)
}

// Location returns this backend's location (the container name).
func (be *Backend) Location() string {
	return be.Join(be.container.String(), be.prefix)
}

// Path returns the path in the bucket that is used for this backend.
func (be *Backend) Path() string {
	return be.prefix
}

// Save stores data in the backend at the handle.
func (be *Backend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	if err := h.Valid(); err != nil {
		return backoff.Permanent(err)
	}

	objName := be.Filename(h)

	debug.Log("Save %v at %v", h, objName)

	be.sem.GetToken()

	debug.Log("InsertObject(%v, %v)", be.container, objName)

	// if it's smaller than 256miB, then just create the file directly from the reader
	blob := be.container.NewBlockBlobURL(objName)

	_, err := storage.UploadStreamToBlockBlob(context.TODO(), rd, blob, storage.UploadStreamToBlockBlobOptions{})

	be.sem.ReleaseToken()
	debug.Log("%v, err %#v", objName, err)

	return errors.Wrap(err, "CreateBlockBlobFromReader")
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
		return nil, backoff.Permanent(err)
	}

	if offset < 0 {
		return nil, errors.New("offset is negative")
	}

	if length < 0 {
		return nil, errors.Errorf("invalid length %d", length)
	}

	objName := be.Filename(h)
	blob := be.container.NewBlockBlobURL(objName)

	be.sem.GetToken()

	resp, err := blob.Download(context.TODO(), offset, int64(length), storage.BlobAccessConditions{}, false, storage.ClientProvidedKeyOptions{})
	if err != nil {
		be.sem.ReleaseToken()
		return nil, err
	}
	closeRd := wrapReader{
		ReadCloser: resp.Body(storage.RetryReaderOptions{}),
		f: func() {
			debug.Log("Close()")
			be.sem.ReleaseToken()
		},
	}

	return closeRd, err
}

// Stat returns information about a blob.
func (be *Backend) Stat(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
	debug.Log("%v", h)

	objName := be.Filename(h)
	blob := be.container.NewBlockBlobURL(objName)
	be.sem.GetToken()
	props, err := blob.GetProperties(context.TODO(), storage.BlobAccessConditions{}, storage.ClientProvidedKeyOptions{})
	be.sem.ReleaseToken()

	if err != nil {
		debug.Log("blob.GetProperties err %v", err)
		return restic.FileInfo{}, errors.Wrap(err, "blob.GetProperties")
	}

	fi := restic.FileInfo{
		Size: int64(props.ContentLength()),
		Name: h.Name,
	}
	return fi, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (be *Backend) Test(ctx context.Context, h restic.Handle) (bool, error) {
	objName := be.Filename(h)

	be.sem.GetToken()
	blob := be.container.NewBlobURL(objName)
	_, err := blob.GetProperties(context.TODO(), storage.BlobAccessConditions{}, storage.ClientProvidedKeyOptions{})
	if err != nil {
		if stgErr, ok := err.(storage.StorageError); ok {
			if stgErr.ServiceCode() == storage.ServiceCodeBlobNotFound {
				return false, nil
			}
		}

		return false, err
	}

	be.sem.ReleaseToken()

	return true, nil
}

// Remove removes the blob with the given name and type.
func (be *Backend) Remove(ctx context.Context, h restic.Handle) error {
	objName := be.Filename(h)

	be.sem.GetToken()
	_, err := be.container.NewBlobURL(objName).Delete(context.TODO(), storage.DeleteSnapshotsOptionNone, storage.BlobAccessConditions{})
	be.sem.ReleaseToken()

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

	listOpts := storage.ListBlobsSegmentOptions{
		MaxResults: int32(be.listMaxItems),
		Prefix:     prefix,
	}

	for marker := (storage.Marker{}); marker.NotDone(); {
		be.sem.GetToken()
		listBlob, err := be.container.ListBlobsFlatSegment(ctx, marker, listOpts)
		be.sem.ReleaseToken()

		if err != nil {
			return err
		}

		debug.Log("got %v objects", len(listBlob.Segment.BlobItems))

		marker = listBlob.NextMarker

		for _, item := range listBlob.Segment.BlobItems {
			m := strings.TrimPrefix(item.Name, prefix)
			if m == "" {
				continue
			}

			fi := restic.FileInfo{
				Name: path.Base(m),
				Size: *item.Properties.ContentLength,
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

// Close does nothing
func (be *Backend) Close() error { return nil }
