package oss

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// Backend is backend for Alibaba Cloud Object Storage Service
type Backend struct {
	cfg    Config
	client *oss.Client
	bucket *oss.Bucket
	sem    *backend.Semaphore
	backend.Layout
}

var _ restic.Backend = &Backend{}

const defaultLayout = "default"

func open(cfg Config) (*Backend, error) {
	sem, err := backend.NewSemaphore(cfg.Connections)
	if err != nil {
		return nil, err
	}

	client, err := oss.New(cfg.Host, cfg.AccessID, cfg.AccessKey, ossProxyOption)
	if err != nil {
		return nil, err
	}

	bucket, err := client.Bucket(cfg.Bucket)
	if err != nil {
		return nil, err
	}
	be := &Backend{
		cfg:    cfg,
		client: client,
		bucket: bucket,
		sem:    sem,
		Layout: &backend.DefaultLayout{
			Join: path.Join,
			Path: cfg.Prefix,
		},
	}
	return be, nil
}

// Open is open repo
func Open(cfg Config) (restic.Backend, error) {
	return open(cfg)
}

// Create is create rop
func Create(cfg Config) (restic.Backend, error) {
	return open(cfg)
}

// Stat returns information about a blob.
func (be *Backend) Stat(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
	debug.Log("Stat(%v)", h)

	objName := be.Filename(h)
	// debug.Log("Stat: %v", objName)
	meta, err := be.bucket.GetObjectMeta(objName)

	if err != nil {
		return restic.FileInfo{}, errors.Wrap(err, "bucket.GetObjectMeta")
	}

	length, err := strconv.ParseInt(meta["Content-Length"][0], 0, 64)
	return restic.FileInfo{Size: length}, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (be *Backend) Test(ctx context.Context, h restic.Handle) (bool, error) {
	found := false
	objName := be.Filename(h)
	_, err := be.bucket.GetObjectMeta(objName)
	if err == nil {
		found = true
	}

	// If error, then not found
	return found, nil
}

// IsNotExist reports whether a given error indicates that an object or bucket
// does not exist.
func (be *Backend) IsNotExist(err error) bool {
	debug.Log("IsNotExist(%T, %#v)", err, err)

	if os.IsNotExist(errors.Cause(err)) {
		return true
	}

	if e, ok := errors.Cause(err).(oss.ServiceError); ok && e.Code == "NoSuchKey" {
		return true
	}
	return false
}

// List - List Object with given type
func (be *Backend) List(ctx context.Context, t restic.FileType) <-chan string {
	debug.Log("List %v", t)
	ch := make(chan string)

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer close(ch)
		defer cancel()

		prefix := be.Dirname(restic.Handle{Type: t})
		marker := oss.Marker("")
		for {
			lor, err := be.bucket.ListObjects(oss.Prefix(prefix), oss.MaxKeys(888), marker)
			if err != nil {
				return
			}

			marker = oss.Marker(lor.NextMarker)
			for _, object := range lor.Objects {
				m := strings.TrimPrefix(object.Key, prefix)
				select {
				case ch <- path.Base(m):
				case <-ctx.Done():
					return
				}
			}

			if !lor.IsTruncated {
				break
			}
		}
	}()

	return ch
}

// Remove - remove object in OSS
func (be *Backend) Remove(ctx context.Context, h restic.Handle) error {
	debug.Log("Remove %v", h)
	return errors.Wrap(be.bucket.DeleteObject(be.Filename(h)), "bucket.DeleteObject")
}

// RemoveKeys
func (be *Backend) removeKeys(ctx context.Context, t restic.FileType) error {
	debug.Log("removeKeys %v", t)
	for key := range be.List(ctx, t) {
		err := be.Remove(ctx, restic.Handle{Type: t, Name: key})
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
	err := be.Remove(ctx, restic.Handle{Type: restic.ConfigFile})
	if err != nil && be.IsNotExist(err) {
		err = nil
	}
	return err
}

// Close does nothing
func (be *Backend) Close() error { return nil }

// Location returns a string that describes the type and location of the
// repository.
func (be *Backend) Location() string {
	return be.cfg.Bucket + ":" + be.cfg.Prefix
}

// Save stores the data in the backend under the given handle.
func (be *Backend) Save(ctx context.Context, h restic.Handle, rd io.Reader) error {
	debug.Log("Save %v", h)

	if err := h.Valid(); err != nil {
		return err
	}

	objName := be.Filename(h)
	// debug.Log("Save key:%v", objName)
	_, err := be.bucket.GetObjectMeta(objName)

	if err == nil {
		debug.Log("%v already exists", h)
		return errors.New("key already exists")
	}

	be.sem.GetToken()
	err = be.bucket.PutObject(objName, rd)
	be.sem.ReleaseToken()
	return errors.Wrap(err, "bucket.PutObject")
}

// Load returns a reader that yields the contents of the file at h at the
// given offset. If length is larger than zero, only a portion of the file
// is returned. rd must be closed after use. If an error is returned, the
// ReadCloser must be nil.
func (be *Backend) Load(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
	debug.Log("Load %v, length %v, offset %v", h, length, offset)
	if offset < 0 {
		return nil, errors.New("offset is negative")
	}

	if length < 0 {
		return nil, errors.Errorf("invalid length %d", length)
	}

	objName := be.Filename(h)
	byteRange := fmt.Sprintf("%d-", offset)
	if length > 0 {
		byteRange = fmt.Sprintf("%d-%d", offset, offset+int64(length)-1)
	}
	be.sem.GetToken()
	body, err := be.bucket.GetObject(objName, oss.NormalizedRange(byteRange))
	be.sem.ReleaseToken()
	return body, err
}

func ossProxyOption(client *oss.Client) {
	proxy := os.Getenv("RESTIC_OSS_PROXY")
	if proxy != "" {
		client.Config.IsUseProxy = true
		client.Config.IsAuthProxy = false
		client.Config.ProxyHost = proxy
	}
}
