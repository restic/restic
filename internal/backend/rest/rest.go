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

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/layout"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/backend/util"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/feature"
)

// make sure the rest backend implements backend.Backend
var _ backend.Backend = &Backend{}

// Backend uses the REST protocol to access data stored on a server.
type Backend struct {
	url         *url.URL
	connections uint
	client      http.Client
	layout.Layout
}

// restError is returned whenever the server returns a non-successful HTTP status.
type restError struct {
	backend.Handle
	StatusCode int
	Status     string
}

func (e *restError) Error() string {
	if e.StatusCode == http.StatusNotFound && e.Handle.Type.String() != "invalid" {
		return fmt.Sprintf("%v does not exist", e.Handle)
	}
	return fmt.Sprintf("unexpected HTTP response (%v): %v", e.StatusCode, e.Status)
}

func NewFactory() location.Factory {
	return location.NewHTTPBackendFactory("rest", ParseConfig, StripPassword, Create, Open)
}

// the REST API protocol version is decided by HTTP request headers, these are the constants.
const (
	ContentTypeV1 = "application/vnd.x.restic.rest.v1"
	ContentTypeV2 = "application/vnd.x.restic.rest.v2"
)

// Open opens the REST backend with the given config.
func Open(_ context.Context, cfg Config, rt http.RoundTripper) (*Backend, error) {
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
	}

	return be, nil
}

func drainAndClose(resp *http.Response) error {
	_, err := io.Copy(io.Discard, resp.Body)
	cerr := resp.Body.Close()

	// return first error
	if err != nil {
		return errors.Errorf("drain: %w", err)
	}
	return cerr
}

// Create creates a new REST on server configured in config.
func Create(ctx context.Context, cfg Config, rt http.RoundTripper) (*Backend, error) {
	be, err := Open(ctx, cfg, rt)
	if err != nil {
		return nil, err
	}

	_, err = be.Stat(ctx, backend.Handle{Type: backend.ConfigFile})
	if err == nil {
		return nil, errors.New("config file already exists")
	}

	url := *cfg.URL
	values := url.Query()
	values.Set("create", "true")
	url.RawQuery = values.Encode()

	resp, err := be.client.Post(url.String(), "binary/octet-stream", strings.NewReader(""))
	if err != nil {
		return nil, err
	}

	if err := drainAndClose(resp); err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &restError{backend.Handle{}, resp.StatusCode, resp.Status}
	}

	return be, nil
}

func (b *Backend) Connections() uint {
	return b.connections
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
func (b *Backend) Save(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
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

	resp, err := b.client.Do(req)
	if err != nil {
		return errors.WithStack(err)
	}

	if err := drainAndClose(resp); err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return &restError{h, resp.StatusCode, resp.Status}
	}

	return nil
}

// IsNotExist returns true if the error was caused by a non-existing file.
func (b *Backend) IsNotExist(err error) bool {
	var e *restError
	return errors.As(err, &e) && e.StatusCode == http.StatusNotFound
}

func (b *Backend) IsPermanentError(err error) bool {
	if b.IsNotExist(err) {
		return true
	}

	var rerr *restError
	if errors.As(err, &rerr) {
		if rerr.StatusCode == http.StatusRequestedRangeNotSatisfiable || rerr.StatusCode == http.StatusUnauthorized || rerr.StatusCode == http.StatusForbidden {
			return true
		}
	}

	return false
}

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (b *Backend) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
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

func (b *Backend) openReader(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
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

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "client.Do")
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		_ = drainAndClose(resp)
		return nil, &restError{h, resp.StatusCode, resp.Status}
	}

	if feature.Flag.Enabled(feature.BackendErrorRedesign) && length > 0 && resp.ContentLength != int64(length) {
		return nil, &restError{h, http.StatusRequestedRangeNotSatisfiable, "partial out of bounds read"}
	}

	return resp.Body, nil
}

// Stat returns information about a blob.
func (b *Backend) Stat(ctx context.Context, h backend.Handle) (backend.FileInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, b.Filename(h), nil)
	if err != nil {
		return backend.FileInfo{}, errors.WithStack(err)
	}
	req.Header.Set("Accept", ContentTypeV2)

	resp, err := b.client.Do(req)
	if err != nil {
		return backend.FileInfo{}, errors.WithStack(err)
	}

	if err = drainAndClose(resp); err != nil {
		return backend.FileInfo{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return backend.FileInfo{}, &restError{h, resp.StatusCode, resp.Status}
	}

	if resp.ContentLength < 0 {
		return backend.FileInfo{}, errors.New("negative content length")
	}

	bi := backend.FileInfo{
		Size: resp.ContentLength,
		Name: h.Name,
	}

	return bi, nil
}

// Remove removes the blob with the given name and type.
func (b *Backend) Remove(ctx context.Context, h backend.Handle) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", b.Filename(h), nil)
	if err != nil {
		return errors.WithStack(err)
	}
	req.Header.Set("Accept", ContentTypeV2)

	resp, err := b.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "client.Do")
	}

	if err = drainAndClose(resp); err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return &restError{h, resp.StatusCode, resp.Status}
	}

	return nil
}

// List runs fn for each file in the backend which has the type t. When an
// error occurs (or fn returns an error), List stops and returns it.
func (b *Backend) List(ctx context.Context, t backend.FileType, fn func(backend.FileInfo) error) error {
	url := b.Dirname(backend.Handle{Type: t})
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return errors.WithStack(err)
	}
	req.Header.Set("Accept", ContentTypeV2)

	resp, err := b.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "List")
	}

	if resp.StatusCode == http.StatusNotFound {
		if !strings.HasPrefix(resp.Header.Get("Server"), "rclone/") {
			// ignore missing directories, unless the server is rclone. rclone
			// already ignores missing directories, but misuses "not found" to
			// report certain internal errors, see
			// https://github.com/rclone/rclone/pull/7550 for details.
			return drainAndClose(resp)
		}
	}

	if resp.StatusCode != http.StatusOK {
		_ = drainAndClose(resp)
		return &restError{backend.Handle{Type: t}, resp.StatusCode, resp.Status}
	}

	if resp.Header.Get("Content-Type") == ContentTypeV2 {
		err = b.listv2(ctx, resp, fn)
	} else {
		err = b.listv1(ctx, t, resp, fn)
	}

	if cerr := drainAndClose(resp); cerr != nil && err == nil {
		err = cerr
	}
	return err
}

// listv1 uses the REST protocol v1, where a list HTTP request (e.g. `GET
// /data/`) only returns the names of the files, so we need to issue an HTTP
// HEAD request for each file.
func (b *Backend) listv1(ctx context.Context, t backend.FileType, resp *http.Response, fn func(backend.FileInfo) error) error {
	debug.Log("parsing API v1 response")
	dec := json.NewDecoder(resp.Body)
	var list []string
	if err := dec.Decode(&list); err != nil {
		return errors.Wrap(err, "Decode")
	}

	for _, m := range list {
		fi, err := b.Stat(ctx, backend.Handle{Name: m, Type: t})
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
func (b *Backend) listv2(ctx context.Context, resp *http.Response, fn func(backend.FileInfo) error) error {
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

		fi := backend.FileInfo{
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

// Delete removes all data in the backend.
func (b *Backend) Delete(ctx context.Context) error {
	return util.DefaultDelete(ctx, b)
}
