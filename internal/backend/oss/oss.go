package oss

import (
	"context"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/pkg/errors"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
)

// Backend stores data on an OSS endpoint.
type Backend struct {
	client *oss.Client
	sem    *backend.Semaphore
	cfg    Config
	backend.Layout
}

// make sure that *Backend implements backend.Backend
var _ restic.Backend = &Backend{}

func open(cfg Config, rt http.RoundTripper) (*Backend, error) {
	debug.Log("open, config %#v", cfg)

	client, err := oss.New(cfg.Endpoint, cfg.AccessKeyID, cfg.AccessKeySecret)
	if err != nil {
		return nil, errors.Wrap(err, "oss.NewWithAK")
	}
	sem, err := backend.NewSemaphore(cfg.Connections)
	be := &Backend{
		client: client,
		sem:    sem,
		cfg:    cfg,
		Layout: &backend.DefaultLayout{
			Path: cfg.Prefix,
			Join: path.Join,
		},
	}

	return be, nil
}

// Open opens the OSS backend at bucket and region. The bucket is created if it
// does not exist yet.
func Open(cfg Config, rt http.RoundTripper) (restic.Backend, error) {
	return open(cfg, rt)
}

// Create opens the OSS backend at bucket and region and creates the bucket if
// it does not exist yet.
func Create(cfg Config, rt http.RoundTripper) (restic.Backend, error) {
	be, err := open(cfg, rt)
	if err != nil {
		return nil, errors.Wrap(err, "open")
	}
	found, err := be.client.IsBucketExist(cfg.Bucket)
	if err != nil {
		debug.Log("BucketExists(%v) returned err %v", cfg.Bucket, err)
		return nil, errors.Wrap(err, "client.IsBucketExists")
	}

	if !found {
		// create new bucket with default ACL in default region
		err = be.client.CreateBucket(cfg.Bucket)
		if err != nil {
			return nil, errors.Wrap(err, "client.CreateBucket")
		}
	}

	return be, nil
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

// Location returns this backend's location (the bucket name).
func (be *Backend) Location() string {
	return be.Join(be.cfg.Bucket, be.cfg.Prefix)
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

	bucketObj, err := be.client.Bucket(be.cfg.Bucket)
	if err != nil {
		debug.Log("PutObject(%v, %v, %v)", be.cfg.Bucket, objName, rd.Length())
		return errors.Wrap(err, "client.PutObject")
	}
	dataReader := ioutil.NopCloser(rd)
	err = bucketObj.PutObject(objName, dataReader)

	debug.Log("PutObject(%v, %v, %v)", be.cfg.Bucket, objName, rd.Length())

	return errors.Wrap(err, "client.PutObject")
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

	bucketObj, err := be.client.Bucket(be.cfg.Bucket)
	if err != nil {
		return nil, err
	}

	var end int64

	if length > 0 {
		end = offset + int64(length) - 1
	} else {
		end = -1
	}

	be.sem.GetToken()

	rd, err := bucketObj.GetObject(objName, oss.Range(offset, end))

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

	be.sem.GetToken()
	bucketObj, err := be.client.Bucket(be.cfg.Bucket)
	if err != nil {
		return restic.FileInfo{}, errors.Wrap(err, "service.Objects.Get")
	}
	obj, err := bucketObj.GetObjectDetailedMeta(objName)
	be.sem.ReleaseToken()

	if err != nil {
		debug.Log("GetObject() err %v", err)
		return restic.FileInfo{}, errors.Wrap(err, "service.Objects.Get")
	}

	size, err := strconv.ParseInt(obj.Get("Content-Length"), 10, 64)
	if err != nil {
		return restic.FileInfo{}, errors.Wrap(err, "service.Objects.Get")
	}

	return restic.FileInfo{Size: size, Name: h.Name}, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (be *Backend) Test(ctx context.Context, h restic.Handle) (bool, error) {
	objName := be.Filename(h)

	be.sem.GetToken()
	bucketObj, err := be.client.Bucket(be.cfg.Bucket)
	if err != nil {
		return false, err
	}
	found, err := bucketObj.IsObjectExist(objName)
	be.sem.ReleaseToken()

	if err != nil {
		return false, err
	}
	return found, nil
}

// Remove removes the blob with the given name and type.
func (be *Backend) Remove(ctx context.Context, h restic.Handle) error {
	objName := be.Filename(h)

	be.sem.GetToken()
	bucketObj, err := be.client.Bucket(be.cfg.Bucket)
	if err != nil {
		debug.Log("Remove(%v) at %v -> err %v", h, objName, err)
		return errors.Wrap(err, "client.RemoveObject")
	}
	err = bucketObj.DeleteObject(objName)
	be.sem.ReleaseToken()

	debug.Log("Remove(%v) at %v -> err %v", h, objName, err)
	return errors.Wrap(err, "client.RemoveObject")
}

// Close does nothing
func (be *Backend) Close() error { return nil }

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

	bucketObj, err := be.client.Bucket(be.cfg.Bucket)
	if err != nil {
		return err
	}

	for {
		be.sem.GetToken()
		marker := oss.Marker("")
		obj, err := bucketObj.ListObjects(oss.Prefix(prefix), oss.MaxKeys(50), marker)
		be.sem.ReleaseToken()

		if err != nil {
			return err
		}

		debug.Log("got %v objects", len(obj.Objects))

		for _, item := range obj.Objects {
			m := strings.TrimPrefix(item.Key, prefix)
			if m == "" {
				continue
			}

			fi := restic.FileInfo{
				Name: path.Base(m),
				Size: item.Size,
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

		if obj.IsTruncated {
			marker = oss.Marker(obj.NextMarker)
		} else {
			break
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
