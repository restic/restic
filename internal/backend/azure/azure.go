package azure

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/cenkalti/backoff/v4"
)

// Backend stores data on an azure endpoint.
type Backend struct {
	accountName  string
	container    *storage.Container
	sem          *backend.Semaphore
	prefix       string
	listMaxItems int
	backend.Layout
}

const defaultListMaxItems = 5000

// make sure that *Backend implements backend.Backend
var _ restic.Backend = &Backend{}

func open(cfg Config, rt http.RoundTripper) (*Backend, error) {
	debug.Log("open, config %#v", cfg)

	client, err := storage.NewBasicClient(cfg.AccountName, cfg.AccountKey)
	if err != nil {
		return nil, errors.Wrap(err, "NewBasicClient")
	}

	client.HTTPClient = &http.Client{Transport: rt}

	service := client.GetBlobService()

	sem, err := backend.NewSemaphore(cfg.Connections)
	if err != nil {
		return nil, err
	}

	be := &Backend{
		container:   service.GetContainerReference(cfg.Container),
		accountName: cfg.AccountName,
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

	options := storage.CreateContainerOptions{
		Access: storage.ContainerAccessTypePrivate,
	}

	_, err = be.container.CreateIfNotExists(&options)
	if err != nil {
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
	return be.Join(be.container.Name, be.prefix)
}

// Path returns the path in the bucket that is used for this backend.
func (be *Backend) Path() string {
	return be.prefix
}

type azureAdapter struct {
	restic.RewindReader
}

func (azureAdapter) Close() error { return nil }

func (a azureAdapter) Len() int {
	return int(a.Length())
}

// Save stores data in the backend at the handle.
func (be *Backend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	if err := h.Valid(); err != nil {
		return backoff.Permanent(err)
	}

	objName := be.Filename(h)

	debug.Log("Save %v at %v", h, objName)

	be.sem.GetToken()

	debug.Log("InsertObject(%v, %v)", be.container.Name, objName)

	var err error
	if rd.Length() < 256*1024*1024 {
		// wrap the reader so that net/http client cannot close the reader
		// CreateBlockBlobFromReader reads length from `Len()``
		dataReader := azureAdapter{rd}

		// if it's smaller than 256miB, then just create the file directly from the reader
		err = be.container.GetBlobReference(objName).CreateBlockBlobFromReader(dataReader, nil)
	} else {
		// otherwise use the more complicated method
		err = be.saveLarge(ctx, objName, rd)

	}

	be.sem.ReleaseToken()
	debug.Log("%v, err %#v", objName, err)

	return errors.Wrap(err, "CreateBlockBlobFromReader")
}

func (be *Backend) saveLarge(ctx context.Context, objName string, rd restic.RewindReader) error {
	// create the file on the server
	file := be.container.GetBlobReference(objName)
	err := file.CreateBlockBlob(nil)
	if err != nil {
		return errors.Wrap(err, "CreateBlockBlob")
	}

	// read the data, in 100 MiB chunks
	buf := make([]byte, 100*1024*1024)
	var blocks []storage.Block
	uploadedBytes := 0

	for {
		n, err := io.ReadFull(rd, buf)
		if err == io.ErrUnexpectedEOF {
			err = nil
		}
		if err == io.EOF {
			// end of file reached, no bytes have been read at all
			break
		}

		if err != nil {
			return errors.Wrap(err, "ReadFull")
		}

		buf = buf[:n]
		uploadedBytes += n

		// upload it as a new "block", use the base64 hash for the ID
		h := restic.Hash(buf)
		id := base64.StdEncoding.EncodeToString(h[:])
		debug.Log("PutBlock %v with %d bytes", id, len(buf))
		err = file.PutBlock(id, buf, nil)
		if err != nil {
			return errors.Wrap(err, "PutBlock")
		}

		blocks = append(blocks, storage.Block{
			ID:     id,
			Status: "Uncommitted",
		})
	}

	// sanity check
	if uploadedBytes != int(rd.Length()) {
		return errors.Errorf("wrote %d bytes instead of the expected %d bytes", uploadedBytes, rd.Length())
	}

	debug.Log("uploaded %d parts: %v", len(blocks), blocks)
	err = file.PutBlockList(blocks, nil)
	debug.Log("PutBlockList returned %v", err)
	return errors.Wrap(err, "PutBlockList")
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
	blob := be.container.GetBlobReference(objName)

	start := uint64(offset)
	var end uint64

	if length > 0 {
		end = uint64(offset + int64(length) - 1)
	} else {
		end = 0
	}

	be.sem.GetToken()

	rd, err := blob.GetRange(&storage.GetBlobRangeOptions{Range: &storage.BlobRange{Start: start, End: end}})
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
func (be *Backend) Stat(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
	debug.Log("%v", h)

	objName := be.Filename(h)
	blob := be.container.GetBlobReference(objName)

	be.sem.GetToken()
	err := blob.GetProperties(nil)
	be.sem.ReleaseToken()

	if err != nil {
		debug.Log("blob.GetProperties err %v", err)
		return restic.FileInfo{}, errors.Wrap(err, "blob.GetProperties")
	}

	fi := restic.FileInfo{
		Size: int64(blob.Properties.ContentLength),
		Name: h.Name,
	}
	return fi, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (be *Backend) Test(ctx context.Context, h restic.Handle) (bool, error) {
	objName := be.Filename(h)

	be.sem.GetToken()
	found, err := be.container.GetBlobReference(objName).Exists()
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
	_, err := be.container.GetBlobReference(objName).DeleteIfExists(nil)
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

	params := storage.ListBlobsParameters{
		MaxResults: uint(be.listMaxItems),
		Prefix:     prefix,
	}

	for {
		be.sem.GetToken()
		obj, err := be.container.ListBlobs(params)
		be.sem.ReleaseToken()

		if err != nil {
			return err
		}

		debug.Log("got %v objects", len(obj.Blobs))

		for _, item := range obj.Blobs {
			m := strings.TrimPrefix(item.Name, prefix)
			if m == "" {
				continue
			}

			fi := restic.FileInfo{
				Name: path.Base(m),
				Size: item.Properties.ContentLength,
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

		if obj.NextMarker == "" {
			break
		}
		params.Marker = obj.NextMarker
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
