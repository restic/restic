package gs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"restic"
	"strings"
	"time"

	"restic/backend"
	"restic/errors"

	"restic/debug"

	storage "google.golang.org/api/storage/v1"
	"io/ioutil"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2"
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

const defaultLayout = "default"

func getStorageService(jsonKeyPath string) (*storage.Service, error) {

	raw, err := ioutil.ReadFile(jsonKeyPath)
	if err != nil {
		return nil, err
	}

	conf, err := google.JWTConfigFromJSON(raw, storage.DevstorageReadWriteScope)
	if err != nil {
		return nil, err
	}

	client := conf.Client(oauth2.NoContext)

	service, err := storage.New(client)
	if err != nil {
		return nil, err
	}

	return  service, nil
}

// Open opens the gs backend at bucket and region. The bucket is created if it
// does not exist yet.
func Open(cfg Config) (restic.Backend, error) {
	debug.Log("open, config %#v", cfg)

	service, err := getStorageService(cfg.JsonKeyPath)
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
	}

	// TODO: CustomTransport in gcs?
	//client.SetCustomTransport(backend.Transport())

	l, err := backend.ParseLayout(be, cfg.Layout, defaultLayout, cfg.Prefix)
	if err != nil {
		return nil, err
	}

	be.Layout = l

	// create bucket if not exists

	if _, err := service.Buckets.Get(be.bucketName).Do(); err!= nil {
		//bucket not exists, creating bucket...

		if _, err := service.Buckets.Insert(be.projectID, &storage.Bucket{Name:be.bucketName}).Do(); err!=nil {
			//error creating bucket
			return nil, errors.Wrap(err, "service.Buckets.Insert")
		}else{
			//bucket created
		}
	}else{
		//bucket exists
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

type fileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
}

func (fi fileInfo) Name() string       { return fi.name }    // base name of the file
func (fi fileInfo) Size() int64        { return fi.size }    // length in bytes for regular files; system-dependent for others
func (fi fileInfo) Mode() os.FileMode  { return fi.mode }    // file mode bits
func (fi fileInfo) ModTime() time.Time { return fi.modTime } // modification time
func (fi fileInfo) IsDir() bool        { return fi.isDir }   // abbreviation for Mode().IsDir()
func (fi fileInfo) Sys() interface{}   { return nil }        // underlying data source (can return nil)

// ReadDir returns the entries for a directory.
func (be *Backend) ReadDir(dir string) (list []os.FileInfo, err error) {
	debug.Log("ReadDir(%v)", dir)

	// make sure dir ends with a slash
	if dir[len(dir)-1] != '/' {
		dir += "/"
	}

	if obj, err := be.service.Objects.List(be.bucketName).Prefix(dir).Delimiter("/").Do(); err != nil {
		return nil, err
	}else {
		for _, item := range obj.Prefixes {
			entry := fileInfo{
				name:    strings.TrimPrefix(item, dir),
				isDir:   true,
				mode:    os.ModeDir | 0755,
			}
			list = append(list, entry)
		}
		for _, item := range obj.Items {
			entry := fileInfo{
				name:    strings.TrimPrefix(item.Name, dir),
				isDir:   false,
				mode:    0644,
				size: 	 int64(item.Size),
				//modTime: item.Updated,
			}
			if entry.name != "" {
				list = append(list, entry)
			}
		}
	}

	return list, nil
}

// Location returns this backend's location (the bucket name).
func (be *Backend) Location() string {
	return be.Join(be.bucketName, be.prefix)
}

// Path returns the path in the bucket that is used for this backend.
func (be *Backend) Path() string {
	return be.prefix
}

// getRemainingSize returns number of bytes remaining. If it is not possible to
// determine the size, panic() is called.
func getRemainingSize(rd io.Reader) (size int64, err error) {
	type Sizer interface {
		Size() int64
	}

	type Lenner interface {
		Len() int
	}

	if r, ok := rd.(Lenner); ok {
		size = int64(r.Len())
	} else if r, ok := rd.(Sizer); ok {
		size = r.Size()
	} else if f, ok := rd.(*os.File); ok {
		fi, err := f.Stat()
		if err != nil {
			return 0, err
		}

		pos, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, err
		}

		size = fi.Size() - pos
	} else {
		panic(fmt.Sprintf("Save() got passed a reader without a method to determine the data size, type is %T", rd))
	}
	return size, nil
}

// preventCloser wraps an io.Reader to run a function instead of the original Close() function.
type preventCloser struct {
	io.Reader
	f func()
}

func (wr preventCloser) Close() error {
	wr.f()
	return nil
}

// Save stores data in the backend at the handle.
func (be *Backend) Save(ctx context.Context, h restic.Handle, rd io.Reader) (err error) {

	if err := h.Valid(); err != nil {
		return err
	}

	objName := be.Filename(h)

	// TODO: use of size?
	size, err := getRemainingSize(rd)
	if err != nil {
		return err
	}

	debug.Log("Save %v at %v", h, objName)

	// Check key does not already exist
	if _, err := be.service.Objects.Get(be.bucketName, objName).Do(); err == nil {
		debug.Log("%v already exists", h)
		return errors.New("key already exists")
	}

	be.sem.GetToken()

	// wrap the reader so that net/http client cannot close the reader, return
	// the token instead.
	rd = preventCloser{
		Reader: rd,
		f: func() {
			debug.Log("Close()")
		},
	}

	debug.Log("InsertObject(%v, %v)", be.bucketName, objName)

	info, err := be.service.Objects.Insert(be.bucketName,
		&storage.Object{
			Name:objName,
			Size: uint64(size),
		}).Media(rd).Do()

	be.sem.ReleaseToken()
	debug.Log("%v -> %v bytes, err %#v", objName, info.Size, err)

	return errors.Wrap(err, "service.Objects.Insert")
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
		byteRange = fmt.Sprintf("bytes=%d-%d", offset, offset+int64(length))
	} else {
		byteRange = fmt.Sprintf("bytes=%d-", offset)
	}

	req := be.service.Objects.Get(be.bucketName, objName)
	// https://cloud.google.com/storage/docs/json_api/v1/parameters#range
	req.Header().Set("Range", byteRange)
	res, err := req.Download()
	if err != nil {
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

// Rename moves a file based on the new layout l.
func (be *Backend) Rename(h restic.Handle, l backend.Layout) error {
	debug.Log("Rename %v to %v", h, l)

	oldName := be.Filename(h)
	newName := l.Filename(h)

	debug.Log("  %v -> %v", oldName, newName)

	_, err := be.service.Objects.Copy(be.bucketName, oldName, be.bucketName, newName, &storage.Object{}).Do()

	if err != nil {
		debug.Log("copy failed: %v", err)
		return err
	}

	return be.service.Objects.Delete(be.bucketName, oldName).Do()
}

