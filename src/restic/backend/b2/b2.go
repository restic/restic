package b2

import (
	"io"
	"path"
	"restic"
	"strings"

	"restic/debug"
	"restic/errors"

	blazerb2 "github.com/kurin/blazer/b2"

	"golang.org/x/net/context"
)

// b2 is a backend which stores the data on a B2 endpoint.
type b2 struct {
	client     *blazerb2.Client
	bucket     *blazerb2.Bucket
	bucketName string
	prefix     string
	context    context.Context
}

// Open opens the B2 backend. The bucket is created if it does not exist yet.
func Open(cfg Config) (restic.Backend, error) {
	debug.Log("open, config %#v", cfg)

	ctx := context.Background()

	client, err := blazerb2.NewClient(ctx, cfg.AccountID, cfg.Key)
	if err != nil {
		return nil, errors.Wrap(err, "blazerb2.NewClient")
	}

	bucket, err := client.NewBucket(ctx, cfg.Bucket, blazerb2.Private)
	if err != nil {
		return nil, errors.Wrap(err, "blazerb2.NewBucket")
	}

	be := &b2{client: client, bucket: bucket, bucketName: cfg.Bucket, prefix: cfg.Prefix, context: ctx}

	return be, nil
}

// Resolve the backend path for based on file type ane name.
func (be *b2) b2path(t restic.FileType, name string) string {
	if t == "key" && name == "config" {
		t = restic.ConfigFile
	}
	if string(t) == restic.ConfigFile || name == "" {
		return path.Join(be.prefix, string(t))
	}
	return path.Join(be.prefix, string(t), name)
}

// Location returns this backend's location (the bucket name).
func (be *b2) Location() string {
	return be.bucketName
}

// Load returns the data stored in the backend for h at the given offset
// and saves it in p. Load has the same semantics as io.ReaderAt.
func (be b2) Load(h restic.Handle, p []byte, off int64) (n int, err error) {
	debug.Log("Load: %v, offset %v, len %v", h, off, len(p))
	objName := be.b2path(h.Type, h.Name)

	obj := be.bucket.Object(objName)
	if err != nil {
		debug.Log("  err %v", err)
		return 0, errors.Wrap(err, "bucket.Object")
	}

	info, err := obj.Attrs(be.context)
	if err != nil {
		return 0, errors.Wrap(err, "obj.Attrs")
	}

	// handle negative offsets
	if off < 0 {
		// if the negative offset is larger than the object itself, read from
		// the beginning.
		if -off > info.Size {
			off = 0
		} else {
			// otherwise compute the offset from the end of the file.
			off = info.Size + off
		}
	}

	// return an error if the offset is beyond the end of the file
	if off > info.Size {
		return 0, errors.Wrap(io.EOF, "")
	}

	var nextError error

	// manually create an io.ErrUnexpectedEOF
	if off+int64(len(p)) > info.Size {
		newlen := info.Size - off
		p = p[:newlen]

		nextError = io.ErrUnexpectedEOF

		debug.Log("  capped buffer to %v byte", len(p))
	}

	r := obj.NewRangeReader(be.context, off, int64(len(p)))
	defer r.Close()

	bufsize := int64(len(p))
	var pos int64 = 0

	// loop to read chunks until the buffer is full or EOF is reached
	for {
		n, err = r.Read(p[pos:])
		pos += int64(n)
		if pos == info.Size-off || pos == bufsize || err != nil {
			if errors.Cause(err) == io.EOF {
				err = nil
			}
			break
		}
	}

	if err == nil {
		err = nextError
	}

	return int(pos), err
}

// Save stores data in the backend at the handle.
func (be b2) Save(h restic.Handle, p []byte) (err error) {
	debug.Log("Save: %v with len %d", h, len(p))
	if err := h.Valid(); err != nil {
		return err
	}

	objName := be.b2path(h.Type, h.Name)

	obj := be.bucket.Object(objName)
	if err != nil {
		debug.Log("  err %v", err)
		return errors.Wrap(err, "bucket.Object")
	}

	_, err = obj.Attrs(be.context)
	if err == nil {
		debug.Log("  %v already exists", h)
		return errors.New("key already exists")
	}

	debug.Log("  Writing to %s", objName)
	w := obj.NewWriter(be.context)
	n, err := w.Write(p)

	if err != nil {
		debug.Log("  Write error %v -> %v bytes, err %#v", objName, n, err)
		w.Close()
		return err
	}

	debug.Log("  Write successful %v -> %v bytes", objName, n)
	w.Close()
	return nil
}

// Stat returns information about a blob.
func (be b2) Stat(h restic.Handle) (bi restic.FileInfo, err error) {
	debug.Log("Stat: %v", h)
	objName := be.b2path(h.Type, h.Name)
	obj := be.bucket.Object(objName)
	info, err := obj.Attrs(be.context)
	if err != nil {
		debug.Log("Attrs() err %v", err)
		return restic.FileInfo{}, errors.Wrap(err, "Stat")
	}
	return restic.FileInfo{Size: info.Size}, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (be *b2) Test(t restic.FileType, name string) (bool, error) {
	found := false
	objName := be.b2path(t, name)
	obj := be.bucket.Object(objName)
	_, err := obj.Attrs(be.context)
	if err == nil {
		found = true
	}
	return found, nil
}

// Remove removes the blob with the given name and type.
func (be *b2) Remove(t restic.FileType, name string) error {
	objName := be.b2path(t, name)

	obj := be.bucket.Object(objName)

	err := obj.Delete(be.context)
	debug.Log("%v %v -> err %v", t, name, err)
	return errors.Wrap(err, "object.Delete")
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (be *b2) List(t restic.FileType, done <-chan struct{}) <-chan string {
	debug.Log("List: %v", t)
	ch := make(chan string)

	go func() {
		defer close(ch)
		prefix := path.Join(be.prefix, string(t)) + "/"
		cur := &blazerb2.Cursor{Name: prefix}

		for {
			objs, c, err := be.bucket.ListCurrentObjects(be.context, 1000, cur)
			if err != nil && err != io.EOF {
				return
			}
			for _, obj := range objs {
				info, err := obj.Attrs(be.context)
				if err != nil {
					continue
				}

				// Skip objects returned that do not have the specified prefix.
				if !strings.HasPrefix(info.Name, prefix) {
					continue
				}

				// Remove the prefix from returned names.
				m := strings.TrimPrefix(info.Name, prefix)
				if m == "" {
					continue
				}

				select {
				case ch <- m:
				case <-done:
					return
				}
			}
			if err == io.EOF {
				return
			}
			cur = c
		}
	}()

	return ch
}

// Remove keys for a specified backend type.
func (be *b2) removeKeys(t restic.FileType) error {
	done := make(chan struct{})
	defer close(done)
	for key := range be.List(restic.DataFile, done) {
		err := be.Remove(restic.DataFile, key)
		if err != nil {
			return err
		}
	}
	return nil
}

// Delete removes all restic keys in the bucket. It will not remove the bucket itself.
func (be *b2) Delete() error {
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
	return be.Remove(restic.ConfigFile, "")
}

// Close does nothing
func (be *b2) Close() error { return nil }
