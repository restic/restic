package drime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/layout"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/backend/util"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
)

const (
	// files up to this size are uploaded with a single request
	singleUploadLimit = 5 * 1024 * 1024
	// size of the parts of larger files, at most 10000 parts are allowed
	multipartPartSize = 8 * 1024 * 1024
)

// drimeBackend stores the repository in a folder hierarchy on Drime.
type drimeBackend struct {
	cfg    Config
	client *client
	layout.Layout

	mu      sync.Mutex
	folders map[string]string // folder path -> id
	files   map[string]string // file path -> id
}

// ensure statically that *drimeBackend implements backend.Backend.
var _ backend.Backend = &drimeBackend{}

func NewFactory() location.Factory {
	return location.NewHTTPBackendFactory("drime", ParseConfig, location.NoPassword, Create, Open)
}

func newBackend(cfg Config, rt http.RoundTripper) (*drimeBackend, error) {
	if cfg.AccessToken.String() == "" {
		return nil, errors.Fatalf("unable to open Drime backend: access token ($DRIME_ACCESS_TOKEN) is empty")
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = defaultEndpoint
	}
	if cfg.WorkspaceID == "" {
		cfg.WorkspaceID = "0"
	}
	if cfg.Connections == 0 {
		cfg.Connections = 5
	}

	be := &drimeBackend{
		cfg: cfg,
		client: &client{
			hc:        &http.Client{Transport: rt},
			endpoint:  strings.TrimRight(cfg.Endpoint, "/"),
			token:     cfg.AccessToken.Unwrap(),
			workspace: cfg.WorkspaceID,
		},
		Layout:  layout.NewDefaultLayout(cfg.Path, path.Join),
		folders: map[string]string{},
		files:   map[string]string{},
	}
	// the root of the drive or the configured root folder
	be.folders[""] = cfg.RootFolderID
	return be, nil
}

// Open opens the repository, the repository folder must exist.
func Open(ctx context.Context, cfg Config, rt http.RoundTripper, _ func(string, ...any)) (backend.Backend, error) {
	be, err := newBackend(cfg, rt)
	if err != nil {
		return nil, err
	}

	_, err = be.folderID(ctx, cfg.Path, false)
	if errors.Is(err, errNotFound) {
		return nil, backend.ErrNoRepository
	} else if err != nil {
		return nil, err
	}
	return be, nil
}

// Create creates the folders of a new repository.
func Create(ctx context.Context, cfg Config, rt http.RoundTripper, _ func(string, ...any)) (backend.Backend, error) {
	be, err := newBackend(cfg, rt)
	if err != nil {
		return nil, err
	}

	for _, t := range []backend.FileType{backend.PackFile, backend.SnapshotFile, backend.IndexFile, backend.LockFile, backend.KeyFile} {
		dir, _ := be.Basedir(t)
		if _, err := be.folderID(ctx, dir, true); err != nil {
			return nil, err
		}
	}
	return be, nil
}

func (be *drimeBackend) Properties() backend.Properties {
	return backend.Properties{
		Connections: be.cfg.Connections,
	}
}

// Hasher returns SHA-256, which Drime uses to verify the stored data after an upload.
func (be *drimeBackend) Hasher() hash.Hash { return sha256.New() }

func (be *drimeBackend) Close() error { return nil }

func (be *drimeBackend) Warmup(_ context.Context, _ []backend.Handle) ([]backend.Handle, error) {
	return nil, nil
}

func (be *drimeBackend) WarmupWait(_ context.Context, _ []backend.Handle) error { return nil }

func (be *drimeBackend) IsNotExist(err error) bool { return errors.Is(err, errNotFound) }

func (be *drimeBackend) IsPermanentError(err error) bool {
	return be.IsNotExist(err) || errors.Is(err, errTooShort) || errors.Is(err, errRange) || errors.Is(err, errCorrupted)
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// folderID returns the id of the folder at the (slash separated) path p. The
// folder and its parents are created if create is set.
func (be *drimeBackend) folderID(ctx context.Context, p string, create bool) (string, error) {
	p = strings.Trim(p, "/")

	be.mu.Lock()
	id, ok := be.folders[p]
	be.mu.Unlock()
	if ok {
		return id, nil
	}

	cur := ""
	be.mu.Lock()
	parentID := be.folders[""]
	be.mu.Unlock()
	for _, seg := range splitPath(p) {
		parentPath := cur
		cur = path.Join(cur, seg)

		be.mu.Lock()
		id, ok := be.folders[cur]
		be.mu.Unlock()
		if !ok {
			var err error
			id, err = be.childFolder(ctx, parentPath, cur, parentID, seg, create)
			if err != nil {
				return "", err
			}
		}
		parentID = id
	}
	return parentID, nil
}

// childFolder looks up the folder name in the folder with the given id and
// remembers all folders found while doing so.
func (be *drimeBackend) childFolder(ctx context.Context, parentPath, p, parentID, name string, create bool) (string, error) {
	find := func() (string, error) {
		found := ""
		err := be.client.list(ctx, parentID, "", false, func(it item) error {
			if it.Type != folderType {
				return nil
			}
			be.mu.Lock()
			be.folders[path.Join(parentPath, it.Name)] = it.ID.String()
			be.mu.Unlock()
			if it.Name == name {
				found = it.ID.String()
			}
			return nil
		})
		return found, err
	}

	id, err := find()
	if err != nil || id != "" {
		return id, err
	}
	if !create {
		return "", errNotFound
	}

	id, err = be.client.mkdir(ctx, parentID, name)
	var ae *apiError
	if errors.As(err, &ae) && ae.Status == http.StatusUnprocessableEntity {
		// created concurrently
		if id, err = find(); err == nil && id == "" {
			err = errors.New("drime: folder " + name + " could not be created")
		}
	}
	if err != nil {
		return "", err
	}

	be.mu.Lock()
	be.folders[p] = id
	be.mu.Unlock()
	return id, nil
}

// fileID returns the id and size of a file, it is cached when the file is saved or listed.
func (be *drimeBackend) fileID(ctx context.Context, name string) (id string, size int64, err error) {
	dirID, err := be.folderID(ctx, path.Dir(name), false)
	if err != nil {
		return "", 0, err
	}

	base := path.Base(name)
	found := false
	err = be.client.list(ctx, dirID, base, false, func(it item) error {
		if it.Type != folderType && it.Name == base {
			id, size, found = it.ID.String(), it.FileSize, true
		}
		return nil
	})
	if err != nil {
		return "", 0, err
	}
	if !found {
		return "", 0, errNotFound
	}

	be.mu.Lock()
	be.files[name] = id
	be.mu.Unlock()
	return id, size, nil
}

func (be *drimeBackend) cachedFileID(name string) (string, bool) {
	be.mu.Lock()
	defer be.mu.Unlock()
	id, ok := be.files[name]
	return id, ok
}

func (be *drimeBackend) forgetFile(name string) {
	be.mu.Lock()
	delete(be.files, name)
	be.mu.Unlock()
}

// Save stores data in the backend at the handle.
func (be *drimeBackend) Save(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
	name := be.Filename(h)
	dirID, err := be.folderID(ctx, path.Dir(name), true)
	if err != nil {
		return err
	}
	base := path.Base(name)
	size := rd.Length()

	// uploading to an existing name replaces the file
	var it item
	if size <= singleUploadLimit {
		data, err := io.ReadAll(io.LimitReader(rd, size+1))
		if err != nil {
			return errors.Wrap(err, "Save")
		}
		if int64(len(data)) != size {
			return errors.Errorf("wrote %d bytes instead of the expected %d bytes", len(data), size)
		}
		it, err = be.client.upload(ctx, dirID, base, data)
		if err != nil {
			return err
		}
	} else {
		it, err = be.saveMultipart(ctx, dirID, base, size, rd)
		if err != nil {
			return err
		}
	}

	be.mu.Lock()
	be.files[name] = it.ID.String()
	be.mu.Unlock()

	return be.verifyUpload(ctx, name, it.ID.String(), rd.Hash())
}

// verifyUpload asks Drime to hash the stored data and compare it to sum. A
// mismatch removes the file. Failing to verify is not an error, the upload
// itself succeeded.
func (be *drimeBackend) verifyUpload(ctx context.Context, name, id string, sum []byte) error {
	if len(sum) == 0 {
		return nil
	}
	want := hex.EncodeToString(sum)

	verified, serverHash, err := be.client.verify(ctx, id, want)
	if err != nil {
		debug.Log("could not verify upload of %v: %v", name, err)
		return nil
	}
	if verified {
		return nil
	}

	be.forgetFile(name)
	if rmErr := be.client.remove(context.WithoutCancel(ctx), id); rmErr != nil {
		debug.Log("could not remove corrupted upload %v: %v", name, rmErr)
	}
	return fmt.Errorf("%w: %v: SHA-256 is %v, expected %v", errCorrupted, name, serverHash, want)
}

func (be *drimeBackend) saveMultipart(ctx context.Context, dirID, name string, size int64, rd io.Reader) (item, error) {
	up, err := be.client.multipartCreate(ctx, dirID, name, size)
	if err != nil {
		return item{}, err
	}

	it, err := func() (item, error) {
		var parts []completedPart
		buf := make([]byte, multipartPartSize)
		var total int64
		for number := 1; ; number++ {
			n, err := io.ReadFull(rd, buf)
			if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
				return item{}, errors.Wrap(err, "Save")
			}
			if n > 0 {
				etag, perr := be.client.multipartPart(ctx, up, number, buf[:n])
				if perr != nil {
					return item{}, perr
				}
				parts = append(parts, completedPart{ETag: etag, PartNumber: number})
				total += int64(n)
			}
			if err != nil {
				break
			}
		}
		if total != size {
			return item{}, errors.Errorf("wrote %d bytes instead of the expected %d bytes", total, size)
		}
		return be.client.multipartComplete(ctx, dirID, name, up, size, parts)
	}()
	if err != nil {
		// do not leave the incomplete upload behind, even if ctx was canceled
		be.client.multipartAbort(context.WithoutCancel(ctx), up)
		return item{}, err
	}
	return it, nil
}

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (be *drimeBackend) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	return util.DefaultLoad(ctx, h, length, offset, be.openReader, func(rd io.Reader) error {
		if length == 0 {
			return fn(rd)
		}

		// use a LimitedReader to detect whether the file is too short
		limrd := &io.LimitedReader{R: rd, N: int64(length)}
		err := fn(limrd)

		_, rderr := rd.Read([]byte{0})
		if rderr == io.EOF && limrd.N != 0 {
			return fmt.Errorf("%w: %v", errTooShort, err)
		}
		return err
	})
}

func (be *drimeBackend) openReader(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
	name := be.Filename(h)

	id, ok := be.cachedFileID(name)
	if ok {
		rd, err := be.client.download(ctx, id, offset, length)
		if !errors.Is(err, errNotFound) {
			return rd, err
		}
		// the file was replaced or removed by someone else
		be.forgetFile(name)
	}

	id, _, err := be.fileID(ctx, name)
	if err != nil {
		return nil, err
	}
	return be.client.download(ctx, id, offset, length)
}

// Stat returns information about a file.
func (be *drimeBackend) Stat(ctx context.Context, h backend.Handle) (backend.FileInfo, error) {
	_, size, err := be.fileID(ctx, be.Filename(h))
	if err != nil {
		return backend.FileInfo{}, errors.Wrap(err, "Stat")
	}
	return backend.FileInfo{Size: size, Name: h.Name}, nil
}

// Remove removes the file with the given handle.
func (be *drimeBackend) Remove(ctx context.Context, h backend.Handle) error {
	name := be.Filename(h)

	id, ok := be.cachedFileID(name)
	if !ok {
		var err error
		if id, _, err = be.fileID(ctx, name); err != nil {
			return err
		}
	}

	err := be.client.remove(ctx, id)
	if errors.Is(err, errNotFound) && ok {
		// the cached id was stale, check again
		be.forgetFile(name)
		if id, _, err = be.fileID(ctx, name); err != nil {
			return err
		}
		err = be.client.remove(ctx, id)
	}
	if err == nil || errors.Is(err, errNotFound) {
		be.forgetFile(name)
	}
	return err
}

// List runs fn for each file of type t.
func (be *drimeBackend) List(ctx context.Context, t backend.FileType, fn func(backend.FileInfo) error) error {
	dir, subdirs := be.Basedir(t)

	dirID, err := be.folderID(ctx, dir, false)
	if errors.Is(err, errNotFound) {
		return nil
	} else if err != nil {
		return err
	}

	dirs := []string{dir}
	if subdirs {
		dirs = dirs[:0]
		err := be.client.list(ctx, dirID, "", false, func(it item) error {
			if it.Type == folderType {
				sub := path.Join(dir, it.Name)
				be.mu.Lock()
				be.folders[sub] = it.ID.String()
				be.mu.Unlock()
				dirs = append(dirs, sub)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		infos []backend.FileInfo
		err   error
	}
	results := make(chan result)
	work := make(chan string)

	var wg sync.WaitGroup
	for range min(int(be.cfg.Connections), max(len(dirs), 1)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for d := range work {
				infos, err := be.listDir(ctx, d)
				select {
				case results <- result{infos, err}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(work)
		for _, d := range dirs {
			select {
			case work <- d:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	// fn has to be called from this goroutine
	var firstErr error
	for res := range results {
		if firstErr != nil {
			continue
		}
		if res.err != nil {
			firstErr = res.err
			cancel()
			continue
		}
		for _, fi := range res.infos {
			if err := ctx.Err(); err != nil {
				firstErr = err
				break
			}
			if err := fn(fi); err != nil {
				firstErr = err
				cancel()
				break
			}
		}
	}
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

func (be *drimeBackend) listDir(ctx context.Context, dir string) ([]backend.FileInfo, error) {
	dirID, err := be.folderID(ctx, dir, false)
	if errors.Is(err, errNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	var infos []backend.FileInfo
	err = be.client.list(ctx, dirID, "", false, func(it item) error {
		if it.Type == folderType {
			return nil
		}
		be.mu.Lock()
		be.files[path.Join(dir, it.Name)] = it.ID.String()
		be.mu.Unlock()
		infos = append(infos, backend.FileInfo{Name: it.Name, Size: it.FileSize})
		return nil
	})
	return infos, err
}

// Delete removes all files of the repository. The folders are kept, like other
// backends keep their bucket.
func (be *drimeBackend) Delete(ctx context.Context) error {
	return util.DefaultDelete(ctx, be)
}
