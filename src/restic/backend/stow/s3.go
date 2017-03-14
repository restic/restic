package stow

import (
	"github.com/graymeta/stow"
	"io"
	"os"
	"path"
	"reflect"
	"restic"
	"restic/debug"
	"restic/errors"
	"runtime"
	"strings"
)

const connLimit = 40

// s3 is a backend which stores the data on an S3 endpoint.
type s3 struct {
	location   stow.Location
	container  stow.Container
	connChan   chan struct{}
	bucketname string
	prefix     string
}

// Open opens the S3 backend at bucket and region. The bucket is created if it
// does not exist yet.
func Open(cfg Config) (restic.Backend, error) {
	debug.Log("open, config %#v", cfg)

	loc, err := stow.Dial(cfg.Kind, cfg.ConfigMap)
	if err != nil {
		return nil, errors.Wrap(err, "minio.New")
	}

	be := &s3{location: loc, bucketname: cfg.Bucket, prefix: cfg.Prefix}

	be.container, err = loc.Container(cfg.Bucket)
	if err != nil {
		be.container, err = loc.CreateContainer(cfg.Bucket)
		if err != nil {
			return nil, errors.Wrap(err, "location.CreateContainer")
		}
	}

	//tr := &http.Transport{MaxIdleConnsPerHost: connLimit}
	//client.SetCustomTransport(tr)
	//
	//be.createConnections()

	return be, nil
}

func (be *s3) s3path(h restic.Handle) string {
	if h.Type == restic.ConfigFile {
		return path.Join(be.prefix, string(h.Type))
	}
	return path.Join(be.prefix, string(h.Type), h.Name)
}

func (be *s3) createConnections() {
	be.connChan = make(chan struct{}, connLimit)
	for i := 0; i < connLimit; i++ {
		be.connChan <- struct{}{}
	}
}

// Location returns this backend's location (the bucket name).
func (be *s3) Location() string {
	return be.bucketname
}

// Save stores data in the backend at the handle.
func (be *s3) Save(h restic.Handle, rd io.Reader) (err error) {
	if err := h.Valid(); err != nil {
		return err
	}

	debug.Log("Save %v", h)

	objName := be.s3path(h)

	// Check key does not already exist
	_, err = be.container.Item(objName)
	if err == nil {
		debug.Log("%v already exists", h)
		return errors.New("key already exists")
	}

	<-be.connChan
	defer func() {
		be.connChan <- struct{}{}
	}()

	debug.Log("PutObject(%v, %v)", be.bucketname, objName)
	sz, err := getReaderSize(rd)
	if err != nil {
		debug.Log("reader size %v bytes, err %#v", sz, err)
		return errors.Wrap(err, "getReaderSize")
	}

	_, err = be.container.Put(objName, rd, sz, map[string]interface{}{
		"ContentLength": "binary/octet-stream",
	})
	debug.Log("%v -> %v bytes, err %#v", objName, sz, err)
	return errors.Wrap(err, "client.PutObject")
}

// getReaderSize - Determine the size of Reader if available.
func getReaderSize(reader io.Reader) (size int64, err error) {
	size = -1
	if reader == nil {
		return -1, nil
	}
	// Verify if there is a method by name 'Size'.
	sizeFn := reflect.ValueOf(reader).MethodByName("Size")
	// Verify if there is a method by name 'Len'.
	lenFn := reflect.ValueOf(reader).MethodByName("Len")
	if sizeFn.IsValid() {
		if sizeFn.Kind() == reflect.Func {
			// Call the 'Size' function and save its return value.
			result := sizeFn.Call([]reflect.Value{})
			if len(result) == 1 {
				size = toInt(result[0])
			}
		}
	} else if lenFn.IsValid() {
		if lenFn.Kind() == reflect.Func {
			// Call the 'Len' function and save its return value.
			result := lenFn.Call([]reflect.Value{})
			if len(result) == 1 {
				size = toInt(result[0])
			}
		}
	} else {
		// Fallback to Stat() method, two possible Stat() structs exist.
		switch v := reader.(type) {
		case *os.File:
			var st os.FileInfo
			st, err = v.Stat()
			if err != nil {
				// Handle this case specially for "windows",
				// certain files for example 'Stdin', 'Stdout' and
				// 'Stderr' it is not allowed to fetch file information.
				if runtime.GOOS == "windows" {
					if strings.Contains(err.Error(), "GetFileInformationByHandle") {
						return -1, nil
					}
				}
				return
			}
			// Ignore if input is a directory, throw an error.
			if st.Mode().IsDir() {
				// TODO(tamal): Error type
				return -1, errors.Fatal("Input file cannot be a directory.")
			}
			// Ignore 'Stdin', 'Stdout' and 'Stderr', since they
			// represent *os.File type but internally do not
			// implement Seekable calls. Ignore them and treat
			// them like a stream with unknown length.
			switch st.Name() {
			case "stdin":
				fallthrough
			case "stdout":
				fallthrough
			case "stderr":
				return
			}
			size = st.Size()
		default:
			// TODO(tamal): Skipped minio Object type
			err = errors.Fatal("Unknown reader type")
			return
		}
	}
	// Returns the size here.
	return size, err
}

// toInt - converts go value to its integer representation based
// on the value kind if it is an integer.
func toInt(value reflect.Value) (size int64) {
	size = -1
	if value.IsValid() {
		switch value.Kind() {
		case reflect.Int:
			fallthrough
		case reflect.Int8:
			fallthrough
		case reflect.Int16:
			fallthrough
		case reflect.Int32:
			fallthrough
		case reflect.Int64:
			size = value.Int()
		}
	}
	return size
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
func (be *s3) Load(h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
	debug.Log("Load %v, length %v, offset %v", h, length, offset)
	if err := h.Valid(); err != nil {
		return nil, err
	}

	if offset < 0 {
		return nil, errors.New("offset is negative")
	}

	if length < 0 {
		return nil, errors.Errorf("invalid length %d", length)
	}

	objName := be.s3path(h)

	// get token for connection
	<-be.connChan

	obj, err := be.container.Item(objName)
	if err != nil {
		debug.Log("  err %v", err)

		// return token
		be.connChan <- struct{}{}

		return nil, errors.Wrap(err, "client.GetObject")
	}

	defer func() {
		// return token
		be.connChan <- struct{}{}
	}()

	return obj.Partial(int64(length), offset)
}

// Stat returns information about a blob.
func (be *s3) Stat(h restic.Handle) (bi restic.FileInfo, err error) {
	debug.Log("%v", h)

	objName := be.s3path(h)
	item, err := be.container.Item(objName)
	if err != nil {
		debug.Log("GetObject() err %v", err)
		return restic.FileInfo{}, errors.Wrap(err, "client.GetObject")
	}

	sz, err := item.Size()
	if err != nil {
		debug.Log("Stat() err %v", err)
		return restic.FileInfo{}, errors.Wrap(err, "Stat")
	}

	return restic.FileInfo{Size: sz}, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (be *s3) Test(h restic.Handle) (bool, error) {
	found := false
	objName := be.s3path(h)
	_, err := be.container.Item(objName)
	if err == nil {
		found = true
	}

	// If error, then not found
	return found, nil
}

// Remove removes the blob with the given name and type.
func (be *s3) Remove(h restic.Handle) error {
	objName := be.s3path(h)
	err := be.container.RemoveItem(objName)
	debug.Log("Remove(%v) -> err %v", h, err)
	return errors.Wrap(err, "container.RemoveItem")
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (be *s3) List(t restic.FileType, done <-chan struct{}) <-chan string {
	debug.Log("listing %v", t)
	ch := make(chan string)

	prefix := be.s3path(restic.Handle{Type: t}) + "/"

	go func() {
		defer close(ch)
		cursor := stow.CursorStart
		for {
			items, next, err := be.container.Items(prefix, cursor, 50)
			if err != nil {
				debug.Log("Items(%v, %v) -> err %v", prefix, cursor, err)
				return
			}
			for _, item := range items {
				m := strings.TrimPrefix(item.ID(), prefix)
				if m == "" {
					continue
				}
				select {
				case ch <- m:
				case <-done:
					return
				}
			}
			cursor = next
			if stow.IsCursorEnd(cursor) {
				return
			}
		}
	}()

	return ch
}

// Remove keys for a specified backend type.
func (be *s3) removeKeys(t restic.FileType) error {
	done := make(chan struct{})
	defer close(done)
	for key := range be.List(restic.DataFile, done) {
		err := be.Remove(restic.Handle{Type: restic.DataFile, Name: key})
		if err != nil {
			return err
		}
	}

	return nil
}

// Delete removes all restic keys in the bucket. It will not remove the bucket itself.
func (be *s3) Delete() error {
	alltypes := []restic.FileType{
		restic.DataFile,
		restic.KeyFile,
		restic.LockFile,
		restic.SnapshotFile,
		restic.IndexFile}

	for _, t := range alltypes {
		err := be.removeKeys(t)
		if err != nil {
			return nil
		}
	}

	return be.Remove(restic.Handle{Type: restic.ConfigFile})
}

// Close does nothing
func (be *s3) Close() error { return nil }
