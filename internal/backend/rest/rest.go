package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/restic/restic/internal/backend/layout"
	"github.com/restic/restic/internal/backend/sema"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/cenkalti/backoff/v4"
)

// make sure the rest backend implements restic.Backend
var _ restic.Backend = &Backend{}

// Backend uses the REST protocol to access data stored on a server.
type Backend struct {
	url         *url.URL
	connections uint
	sem         sema.Semaphore
	client      http.Client
	layout.Layout
}

// the REST API protocol version is decided by HTTP request headers, these are the constants.
const (
	ContentTypeV1 = "application/vnd.x.restic.rest.v1"
	ContentTypeV2 = "application/vnd.x.restic.rest.v2"
)

// Open opens the REST backend with the given config.
func Open(cfg Config, rt http.RoundTripper) (*Backend, error) {
	sem, err := sema.New(cfg.Connections)
	if err != nil {
		return nil, err
	}

	// use url without trailing slash for layout
	url := cfg.URL.String()
	if url[len(url)-1] == '/' {
		url = url[:len(url)-1]
	}

	be := &Backend{
		url:         cfg.URL,
		client:      http.Client{Transport: rt},
		Layout:      &layout.RESTLayout{URL: url, Join: path.Join},
		connections: cfg.Connections,
		sem:         sem,
	}

	return be, nil
}

// Create creates a new REST on server configured in config.
func Create(ctx context.Context, cfg Config, rt http.RoundTripper) (*Backend, error) {
	be, err := Open(cfg, rt)
	if err != nil {
		return nil, err
	}

	_, err = be.Stat(ctx, restic.Handle{Type: restic.ConfigFile})
	if err == nil {
		return nil, errors.Fatal("config file already exists")
	}

	url := *cfg.URL
	values := url.Query()
	values.Set("create", "true")
	url.RawQuery = values.Encode()

	resp, err := be.client.Post(url.String(), "binary/octet-stream", strings.NewReader(""))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Fatalf("server response unexpected: %v (%v)", resp.Status, resp.StatusCode)
	}

	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return nil, err
	}

	err = resp.Body.Close()
	if err != nil {
		return nil, err
	}

	return be, nil
}

func (b *Backend) Connections() uint {
	return b.connections
}

// Location returns this backend's location (the server's URL).
func (b *Backend) Location() string {
	return b.url.String()
}

// Hasher may return a hash function for calculating a content hash for the backend
func (b *Backend) Hasher() hash.Hash {
	return nil
}

// HasAtomicReplace returns whether Save() can atomically replace files
func (b *Backend) HasAtomicReplace() bool {
	// rest-server prevents overwriting
	return false
}

// Save stores data in the backend at the handle.
func (b *Backend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	if err := h.Valid(); err != nil {
		return backoff.Permanent(err)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// make sure that client.Post() cannot close the reader by wrapping it
	req, err := http.NewRequestWithContext(ctx,
		http.MethodPost, b.Filename(h), io.NopCloser(rd))
	if err != nil {
		return errors.WithStack(err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Accept", ContentTypeV2)

	// explicitly set the content length, this prevents chunked encoding and
	// let's the server know what's coming.
	req.ContentLength = rd.Length()

	b.sem.GetToken()
	resp, err := b.client.Do(req)
	b.sem.ReleaseToken()

	var cerr error
	if resp != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		cerr = resp.Body.Close()
	}

	if err != nil {
		return errors.WithStack(err)
	}

	if resp.StatusCode != 200 {
		return errors.Errorf("server response unexpected: %v (%v)", resp.Status, resp.StatusCode)
	}

	return errors.Wrap(cerr, "Close")
}

// notExistError is returned whenever the requested file does not exist on the
// server.
type notExistError struct {
	restic.Handle
}

func (e *notExistError) Error() string {
	return fmt.Sprintf("%v does not exist", e.Handle)
}

// IsNotExist returns true if the error was caused by a non-existing file.
func (b *Backend) IsNotExist(err error) bool {
	var e *notExistError
	return errors.As(err, &e)
}

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (b *Backend) Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	r, err := b.openReader(ctx, h, length, offset)
	if err != nil {
		return err
	}
	err = fn(r)
	if err != nil {
		_ = r.Close() // ignore error here
		return err
	}

	// Note: readerat.ReadAt() (the fn) uses io.ReadFull() that doesn't
	// wait for EOF after reading body. Due to HTTP/2 stream multiplexing
	// and goroutine timings the EOF frame arrives from server (eg. rclone)
	// with a delay after reading body. Immediate close might trigger
	// HTTP/2 stream reset resulting in the *stream closed* error on server,
	// so we wait for EOF before closing body.
	var buf [1]byte
	_, err = r.Read(buf[:])
	if err == io.EOF {
		err = nil
	}

	if e := r.Close(); err == nil {
		err = e
	}
	return err
}

func (b *Backend) openReader(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
	debug.Log("Load %v, length %v, offset %v", h, length, offset)
	if err := h.Valid(); err != nil {
		return nil, backoff.Permanent(err)
	}

	if offset < 0 {
		return nil, errors.New("offset is negative")
	}

	if length < 0 {
		return nil, errors.Errorf("invalid length %d", length)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", b.Filename(h), nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	byteRange := fmt.Sprintf("bytes=%d-", offset)
	if length > 0 {
		byteRange = fmt.Sprintf("bytes=%d-%d", offset, offset+int64(length)-1)
	}
	req.Header.Set("Range", byteRange)
	req.Header.Set("Accept", ContentTypeV2)
	debug.Log("Load(%v) send range %v", h, byteRange)

	b.sem.GetToken()
	resp, err := b.client.Do(req)
	b.sem.ReleaseToken()

	if err != nil {
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		return nil, errors.Wrap(err, "client.Do")
	}

	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return nil, &notExistError{h}
	}

	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		_ = resp.Body.Close()
		return nil, errors.Errorf("unexpected HTTP response (%v): %v", resp.StatusCode, resp.Status)
	}

	return resp.Body, nil
}

// Stat returns information about a blob.
func (b *Backend) Stat(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
	if err := h.Valid(); err != nil {
		return restic.FileInfo{}, backoff.Permanent(err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, b.Filename(h), nil)
	if err != nil {
		return restic.FileInfo{}, errors.WithStack(err)
	}
	req.Header.Set("Accept", ContentTypeV2)

	b.sem.GetToken()
	resp, err := b.client.Do(req)
	b.sem.ReleaseToken()
	if err != nil {
		return restic.FileInfo{}, errors.WithStack(err)
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	if err = resp.Body.Close(); err != nil {
		return restic.FileInfo{}, errors.Wrap(err, "Close")
	}

	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return restic.FileInfo{}, &notExistError{h}
	}

	if resp.StatusCode != 200 {
		return restic.FileInfo{}, errors.Errorf("unexpected HTTP response (%v): %v", resp.StatusCode, resp.Status)
	}

	if resp.ContentLength < 0 {
		return restic.FileInfo{}, errors.New("negative content length")
	}

	bi := restic.FileInfo{
		Size: resp.ContentLength,
		Name: h.Name,
	}

	return bi, nil
}

// Remove removes the blob with the given name and type.
func (b *Backend) Remove(ctx context.Context, h restic.Handle) error {
	if err := h.Valid(); err != nil {
		return backoff.Permanent(err)
	}

	req, err := http.NewRequestWithContext(ctx, "DELETE", b.Filename(h), nil)
	if err != nil {
		return errors.WithStack(err)
	}
	req.Header.Set("Accept", ContentTypeV2)

	b.sem.GetToken()
	resp, err := b.client.Do(req)
	b.sem.ReleaseToken()

	if err != nil {
		return errors.Wrap(err, "client.Do")
	}

	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return &notExistError{h}
	}

	if resp.StatusCode != 200 {
		return errors.Errorf("blob not removed, server response: %v (%v)", resp.Status, resp.StatusCode)
	}

	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return errors.Wrap(err, "Copy")
	}

	return errors.Wrap(resp.Body.Close(), "Close")
}

// List runs fn for each file in the backend which has the type t. When an
// error occurs (or fn returns an error), List stops and returns it.
func (b *Backend) List(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error {
	url := b.Dirname(restic.Handle{Type: t})
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return errors.WithStack(err)
	}
	req.Header.Set("Accept", ContentTypeV2)

	b.sem.GetToken()
	resp, err := b.client.Do(req)
	b.sem.ReleaseToken()

	if err != nil {
		return errors.Wrap(err, "List")
	}

	if resp.StatusCode != 200 {
		return errors.Errorf("List failed, server response: %v (%v)", resp.Status, resp.StatusCode)
	}

	if resp.Header.Get("Content-Type") == ContentTypeV2 {
		return b.listv2(ctx, t, resp, fn)
	}

	return b.listv1(ctx, t, resp, fn)
}

// listv1 uses the REST protocol v1, where a list HTTP request (e.g. `GET
// /data/`) only returns the names of the files, so we need to issue an HTTP
// HEAD request for each file.
func (b *Backend) listv1(ctx context.Context, t restic.FileType, resp *http.Response, fn func(restic.FileInfo) error) error {
	debug.Log("parsing API v1 response")
	dec := json.NewDecoder(resp.Body)
	var list []string
	if err := dec.Decode(&list); err != nil {
		return errors.Wrap(err, "Decode")
	}

	for _, m := range list {
		fi, err := b.Stat(ctx, restic.Handle{Name: m, Type: t})
		if err != nil {
			return err
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		fi.Name = m
		err = fn(fi)
		if err != nil {
			return err
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return ctx.Err()
}

// listv2 uses the REST protocol v2, where a list HTTP request (e.g. `GET
// /data/`) returns the names and sizes of all files.
func (b *Backend) listv2(ctx context.Context, t restic.FileType, resp *http.Response, fn func(restic.FileInfo) error) error {
	debug.Log("parsing API v2 response")
	dec := json.NewDecoder(resp.Body)

	var list []struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	if err := dec.Decode(&list); err != nil {
		return errors.Wrap(err, "Decode")
	}

	for _, item := range list {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		fi := restic.FileInfo{
			Name: item.Name,
			Size: item.Size,
		}

		err := fn(fi)
		if err != nil {
			return err
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return ctx.Err()
}

// Close closes all open files.
func (b *Backend) Close() error {
	// this does not need to do anything, all open files are closed within the
	// same function.
	return nil
}

// Remove keys for a specified backend type.
func (b *Backend) removeKeys(ctx context.Context, t restic.FileType) error {
	return b.List(ctx, t, func(fi restic.FileInfo) error {
		return b.Remove(ctx, restic.Handle{Type: t, Name: fi.Name})
	})
}

// Delete removes all data in the backend.
func (b *Backend) Delete(ctx context.Context) error {
	alltypes := []restic.FileType{
		restic.PackFile,
		restic.KeyFile,
		restic.LockFile,
		restic.SnapshotFile,
		restic.IndexFile}

	for _, t := range alltypes {
		err := b.removeKeys(ctx, t)
		if err != nil {
			return nil
		}
	}

	err := b.Remove(ctx, restic.Handle{Type: restic.ConfigFile})
	if err != nil && b.IsNotExist(err) {
		return nil
	}
	return err
}
