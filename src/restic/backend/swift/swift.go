package swift

import (
	"io"
	"path"
	"restic"
	"restic/backend"
	"restic/debug"
	"restic/errors"
	"strings"
	"time"

	"github.com/ncw/swift"
)

const connLimit = 10

// beSwift is a backend which stores the data on a swift endpoint.
type beSwift struct {
	conn      *swift.Connection
	connChan  chan struct{}
	container string // Container name
	prefix    string // Prefix of object names in the container
}

// Open opens the swift backend at a container in region. The container is
// created if it does not exist yet.
func Open(cfg Config) (restic.Backend, error) {

	be := &beSwift{
		conn: &swift.Connection{
			UserName:       cfg.UserName,
			Domain:         cfg.Domain,
			ApiKey:         cfg.APIKey,
			AuthUrl:        cfg.AuthURL,
			Region:         cfg.Region,
			Tenant:         cfg.Tenant,
			TenantId:       cfg.TenantID,
			TenantDomain:   cfg.TenantDomain,
			TrustId:        cfg.TrustID,
			StorageUrl:     cfg.StorageURL,
			AuthToken:      cfg.AuthToken,
			ConnectTimeout: time.Minute,
			Timeout:        time.Minute,
		},
		container: cfg.Container,
		prefix:    cfg.Prefix,
	}
	be.createConnections()

	// Authenticate if needed
	if !be.conn.Authenticated() {
		if err := be.conn.Authenticate(); err != nil {
			return nil, errors.Wrap(err, "conn.Authenticate")
		}
	}

	// Ensure container exists
	switch _, _, err := be.conn.Container(be.container); err {
	case nil:
		// Container exists

	case swift.ContainerNotFound:
		err = be.createContainer(cfg.DefaultContainerPolicy)
		if err != nil {
			return nil, errors.Wrap(err, "beSwift.createContainer")
		}

	default:
		return nil, errors.Wrap(err, "conn.Container")
	}

	return be, nil
}

func (be *beSwift) swiftpath(h restic.Handle) string {

	var dir string

	switch h.Type {
	case restic.ConfigFile:
		dir = ""
		h.Name = backend.Paths.Config
	case restic.DataFile:
		dir = backend.Paths.Data
	case restic.SnapshotFile:
		dir = backend.Paths.Snapshots
	case restic.IndexFile:
		dir = backend.Paths.Index
	case restic.LockFile:
		dir = backend.Paths.Locks
	case restic.KeyFile:
		dir = backend.Paths.Keys
	default:
		dir = string(h.Type)
	}

	return path.Join(be.prefix, dir, h.Name)
}

func (be *beSwift) createConnections() {
	be.connChan = make(chan struct{}, connLimit)
	for i := 0; i < connLimit; i++ {
		be.connChan <- struct{}{}
	}
}

func (be *beSwift) createContainer(policy string) error {
	var h swift.Headers
	if policy != "" {
		h = swift.Headers{
			"X-Storage-Policy": policy,
		}
	}

	return be.conn.ContainerCreate(be.container, h)
}

// Location returns this backend's location (the container name).
func (be *beSwift) Location() string {
	return be.container
}

// Load returns a reader that yields the contents of the file at h at the
// given offset. If length is nonzero, only a portion of the file is
// returned. rd must be closed after use.
func (be *beSwift) Load(h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
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

	objName := be.swiftpath(h)

	<-be.connChan
	defer func() {
		be.connChan <- struct{}{}
	}()

	obj, _, err := be.conn.ObjectOpen(be.container, objName, false, nil)
	if err != nil {
		debug.Log("  err %v", err)
		return nil, errors.Wrap(err, "conn.ObjectOpen")
	}

	// if we're going to read the whole object, just pass it on.
	if length == 0 {
		debug.Log("Load %v: pass on object", h)
		_, err = obj.Seek(offset, 0)
		if err != nil {
			_ = obj.Close()
			return nil, errors.Wrap(err, "obj.Seek")
		}

		return obj, nil
	}

	// otherwise pass a LimitReader
	size, err := obj.Length()
	if err != nil {
		return nil, errors.Wrap(err, "obj.Length")
	}

	if offset > size {
		_ = obj.Close()
		return nil, errors.Errorf("offset larger than file size")
	}

	_, err = obj.Seek(offset, 0)
	if err != nil {
		_ = obj.Close()
		return nil, errors.Wrap(err, "obj.Seek")
	}

	return backend.LimitReadCloser(obj, int64(length)), nil
}

// Save stores data in the backend at the handle.
func (be *beSwift) Save(h restic.Handle, rd io.Reader) (err error) {
	if err = h.Valid(); err != nil {
		return err
	}

	debug.Log("Save %v", h)

	objName := be.swiftpath(h)

	// Check key does not already exist
	switch _, _, err = be.conn.Object(be.container, objName); err {
	case nil:
		debug.Log("%v already exists", h)
		return errors.New("key already exists")

	case swift.ObjectNotFound:
		// Ok, that's what we want

	default:
		return errors.Wrap(err, "conn.Object")
	}

	<-be.connChan
	defer func() {
		be.connChan <- struct{}{}
	}()

	encoding := "binary/octet-stream"

	debug.Log("PutObject(%v, %v, %v)",
		be.container, objName, encoding)
	//err = be.conn.ObjectPutBytes(be.container, objName, p, encoding)
	_, err = be.conn.ObjectPut(be.container, objName, rd, true, "", encoding, nil)
	debug.Log("%v, err %#v", objName, err)

	return errors.Wrap(err, "client.PutObject")
}

// Stat returns information about a blob.
func (be *beSwift) Stat(h restic.Handle) (bi restic.FileInfo, err error) {
	debug.Log("%v", h)

	objName := be.swiftpath(h)

	obj, _, err := be.conn.Object(be.container, objName)
	if err != nil {
		debug.Log("Object() err %v", err)
		return restic.FileInfo{}, errors.Wrap(err, "conn.Object")
	}

	return restic.FileInfo{Size: obj.Bytes}, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (be *beSwift) Test(h restic.Handle) (bool, error) {
	objName := be.swiftpath(h)
	switch _, _, err := be.conn.Object(be.container, objName); err {
	case nil:
		return true, nil

	case swift.ObjectNotFound:
		return false, nil

	default:
		return false, errors.Wrap(err, "conn.Object")
	}
}

// Remove removes the blob with the given name and type.
func (be *beSwift) Remove(h restic.Handle) error {
	objName := be.swiftpath(h)
	err := be.conn.ObjectDelete(be.container, objName)
	debug.Log("Remove(%v) -> err %v", h, err)
	return errors.Wrap(err, "conn.ObjectDelete")
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (be *beSwift) List(t restic.FileType, done <-chan struct{}) <-chan string {
	debug.Log("listing %v", t)
	ch := make(chan string)

	prefix := be.swiftpath(restic.Handle{Type: t}) + "/"

	go func() {
		defer close(ch)

		be.conn.ObjectsWalk(be.container, &swift.ObjectsOpts{Prefix: prefix},
			func(opts *swift.ObjectsOpts) (interface{}, error) {
				newObjects, err := be.conn.ObjectNames(be.container, opts)
				if err != nil {
					return nil, errors.Wrap(err, "conn.ObjectNames")
				}
				for _, obj := range newObjects {
					m := strings.TrimPrefix(obj, prefix)
					if m == "" {
						continue
					}

					select {
					case ch <- m:
					case <-done:
						return nil, io.EOF
					}
				}
				return newObjects, nil
			})
	}()

	return ch
}

// Remove keys for a specified backend type.
func (be *beSwift) removeKeys(t restic.FileType) error {
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

// Delete removes all restic objects in the container.
// It will not remove the container itself.
func (be *beSwift) Delete() error {
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
func (be *beSwift) Close() error { return nil }
