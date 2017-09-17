package gs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/pkg/errors"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"io/ioutil"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	storage "google.golang.org/api/storage/v1"
)

// Backend stores data on an gs endpoint.
type Backend struct {
	service    *storage.Service
	projectID  string
	sem        *backend.Semaphore
	bucketName string
	prefix     string
	backend.Layout
}

// make sure that *Backend implements backend.Backend
var _ restic.Backend = &Backend{}

func getStorageService(jsonKeyPath string) (*storage.Service, error) {

	raw, err := ioutil.ReadFile(jsonKeyPath)
	if err != nil {
		return nil, errors.Wrap(err, "ReadFile")
	}

	conf, err := google.JWTConfigFromJSON(raw, storage.DevstorageReadWriteScope)
	if err != nil {
		return nil, err
	}

	client := conf.Client(context.TODO())

	service, err := storage.New(client)
	if err != nil {
		return nil, err
	}

	return service, nil
}

func open(cfg Config) (*Backend, error) {
	debug.Log("open, config %#v", cfg)

	service, err := getStorageService(cfg.JSONKeyPath)
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
	}

	return be, nil
}

// Open opens the gs backend at bucket and region.
func Open(cfg Config) (restic.Backend, error) {
	return open(cfg)
}

// Create opens the S3 backend at bucket and region and creates the bucket if
// it does not exist yet.
func Create(cfg Config) (restic.Backend, error) {
	be, err := open(cfg)

	if err != nil {
		return nil, errors.Wrap(err, "open")
	}

	// Create bucket if not exists
	if _, err := be.service.Buckets.Get(be.bucketName).Do(); err != nil {
		bucket := &storage.Bucket{
			Name: be.bucketName,
		}

		if _, err := be.service.Buckets.Insert(be.projectID, bucket).Do(); err != nil {
			return nil, errors.Wrap(err, "service.Buckets.Insert")
		}
	}

	return be, nil
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
func (be *Backend) Save(ctx context.Context, h restic.Handle, rd io.Reader) (err error) {
	if err := h.Valid(); err != nil {
		return err
	}

	objName := be.Filename(h)

	debug.Log("Save %v at %v", h, objName)

	// Check key does not already exist
	if _, err := be.service.Objects.Get(be.bucketName, objName).Do(); err == nil {
		debug.Log("%v already exists", h)
		return errors.New("key already exists")
	}

	be.sem.GetToken()

	debug.Log("InsertObject(%v, %v)", be.bucketName, objName)

	info, err := be.service.Objects.Insert(be.bucketName,
		&storage.Object{
			Name: objName,
		}).Media(rd).Do()

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

// Load returns a reader that yields the contents of the file at h at the
// given offset. If length is nonzero, only a portion of the file is
// returned. rd must be closed after use.
func (be *Backend) Load(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
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

	obj, err := be.service.Objects.Get(be.bucketName, objName).Do()
	if err != nil {
		debug.Log("GetObject() err %v", err)
		return restic.FileInfo{}, errors.Wrap(err, "service.Objects.Get")
	}

	return restic.FileInfo{Size: int64(obj.Size)}, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (be *Backend) Test(ctx context.Context, h restic.Handle) (bool, error) {
	found := false
	objName := be.Filename(h)
	_, err := be.service.Objects.Get(be.bucketName, objName).Do()
	if err == nil {
		found = true
	}
	// If error, then not found
	return found, nil
}

// Remove removes the blob with the given name and type.
func (be *Backend) Remove(ctx context.Context, h restic.Handle) error {
	objName := be.Filename(h)

	err := be.service.Objects.Delete(be.bucketName, objName).Do()
	if er, ok := err.(*googleapi.Error); ok {
		if er.Code == 404 {
			err = nil
		}
	}

	debug.Log("Remove(%v) at %v -> err %v", h, objName, err)
	return errors.Wrap(err, "client.RemoveObject")
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (be *Backend) List(ctx context.Context, t restic.FileType) <-chan string {
	debug.Log("listing %v", t)
	ch := make(chan string)

	prefix := be.Dirname(restic.Handle{Type: t})

	// make sure prefix ends with a slash
	if prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}

	go func() {
		defer close(ch)

		obj, err := be.service.Objects.List(be.bucketName).Prefix(prefix).Do()
		if err != nil {
			return
		}

		for _, item := range obj.Items {
			m := strings.TrimPrefix(item.Name, prefix)
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
func (be *Backend) removeKeys(ctx context.Context, t restic.FileType) error {
	for key := range be.List(ctx, restic.DataFile) {
		err := be.Remove(ctx, restic.Handle{Type: restic.DataFile, Name: key})
		if err != nil {
			return err
		}
	}

	return nil
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
