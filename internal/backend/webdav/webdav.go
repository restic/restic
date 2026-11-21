package webdav

import (
	"context"
	"encoding/xml"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/layout"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/backend/util"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/feature"
)

// make sure the WebDAV backend implements backend.Backend
var _ backend.Backend = &Backend{}

// Backend uses the WebDAV protocol to access data stored on a server.
type Backend struct {
	url         *url.URL
	connections uint
	client      http.Client
	layout.Layout
}

// davError is returned whenever the server returns a non-successful HTTP status.
type davError struct {
	backend.Handle
	StatusCode int
	Status     string
}

func (e *davError) Error() string {
	if e.StatusCode == http.StatusNotFound && e.Handle.Type.String() != "invalid" {
		return fmt.Sprintf("%v does not exist", e.Handle)
	}
	return fmt.Sprintf("unexpected HTTP response (%v): %v", e.StatusCode, e.Status)
}

func NewFactory() location.Factory {
	return location.NewHTTPBackendFactory("webdav", ParseConfig, StripPassword, Create, Open)
}

// Open opens the WebDAV backend with the given config.
func Open(_ context.Context, cfg Config, rt http.RoundTripper, _ func(string, ...interface{})) (*Backend, error) {
	// use url without trailing slash for layout
	url := cfg.URL.String()
	if url[len(url)-1] == '/' {
		url = url[:len(url)-1]
	}

	be := &Backend{
		url:    cfg.URL,
		client: http.Client{Transport: rt},
		Layout: layout.NewDefaultLayout(url, func(parts ...string) string {
			p := make([]string, len(parts))
			copy(p, parts)
			p[0] = "/"
			return strings.TrimRight(parts[0], "/") + path.Join(p...)
		}),
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

func createPath(ctx context.Context, be *Backend, url string) error {
	req, err := http.NewRequestWithContext(ctx, "MKCOL", url, nil)
	if err != nil {
		return errors.WithStack(err)
	}

	resp, err := be.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "Create")
	}

	if err := drainAndClose(resp); err != nil {
		return err
	}

	if resp.StatusCode != http.StatusCreated {
		return &davError{backend.Handle{}, resp.StatusCode, resp.Status}
	}

	return nil
}

// Create creates a new WebDAV on server configured in config.
// TODO: Change this to MKCOL /{path}; expect 201 Created
func Create(ctx context.Context, cfg Config, rt http.RoundTripper, errorLog func(string, ...interface{})) (*Backend, error) {
	be, err := Open(ctx, cfg, rt, errorLog)
	if err != nil {
		return nil, err
	}

	_, err = be.Stat(ctx, backend.Handle{Type: backend.ConfigFile})
	if err == nil {
		return nil, errors.New("config file already exists")
	}

	// MKCOL isn't recursive so create the root of the repo first
	if err := createPath(ctx, be, cfg.URL.String()); err != nil {
		return nil, err
	}

	for _, url := range be.Layout.Paths() {
		if err := createPath(ctx, be, url); err != nil {
			return nil, err
		}
	}

	return be, nil
}

func (b *Backend) Properties() backend.Properties {
	return backend.Properties{
		Connections:      b.connections,
		HasAtomicReplace: false, // WebDAV server can prevent overwriting
	}
}

// Hasher may return a hash function for calculating a content hash for the backend
func (b *Backend) Hasher() hash.Hash {
	return nil
}

// Save stores data in the backend at the handle.
func (b *Backend) Save(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// make sure that client.Post() cannot close the reader by wrapping it
	req, err := http.NewRequestWithContext(ctx,
		http.MethodPut, b.Filename(h), io.NopCloser(rd))
	if err != nil {
		return errors.WithStack(err)
	}
	req.GetBody = func() (io.ReadCloser, error) {
		if err := rd.Rewind(); err != nil {
			return nil, err
		}
		return io.NopCloser(rd), nil
	}
	req.Header.Set("Content-Type", "application/octet-stream")

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

	if !slices.Contains(
		[]int{http.StatusOK, http.StatusCreated, http.StatusAccepted},
		resp.StatusCode,
	) {
		return &davError{h, resp.StatusCode, resp.Status}
	}

	return nil
}

// IsNotExist returns true if the error was caused by a non-existing file.
func (b *Backend) IsNotExist(err error) bool {
	var e *davError
	return errors.As(err, &e) && e.StatusCode == http.StatusNotFound
}

func (b *Backend) IsPermanentError(err error) bool {
	if b.IsNotExist(err) {
		return true
	}

	var rerr *davError
	if errors.As(err, &rerr) {
		if rerr.StatusCode == http.StatusRequestedRangeNotSatisfiable || rerr.StatusCode == http.StatusUnauthorized || rerr.StatusCode == http.StatusForbidden || rerr.StatusCode == http.StatusInsufficientStorage {
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.Filename(h), nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	byteRange := fmt.Sprintf("bytes=%d-", offset)
	if length > 0 {
		byteRange = fmt.Sprintf("bytes=%d-%d", offset, offset+int64(length)-1)
	}
	req.Header.Set("Range", byteRange)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "client.Do")
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		_ = drainAndClose(resp)
		return nil, &davError{h, resp.StatusCode, resp.Status}
	}

	if feature.Flag.Enabled(feature.BackendErrorRedesign) && length > 0 && resp.ContentLength != int64(length) {
		return nil, &davError{h, http.StatusRequestedRangeNotSatisfiable, "partial out of bounds read"}
	}

	return resp.Body, nil
}

// Stat returns information about a blob.
func (b *Backend) Stat(ctx context.Context, h backend.Handle) (backend.FileInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, b.Filename(h), nil)
	if err != nil {
		return backend.FileInfo{}, errors.WithStack(err)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return backend.FileInfo{}, errors.WithStack(err)
	}

	if err = drainAndClose(resp); err != nil {
		return backend.FileInfo{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return backend.FileInfo{}, &davError{h, resp.StatusCode, resp.Status}
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
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, b.Filename(h), nil)
	if err != nil {
		return errors.WithStack(err)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "client.Do")
	}

	if err = drainAndClose(resp); err != nil {
		return err
	}

	if !slices.Contains(
		[]int{http.StatusOK, http.StatusAccepted, http.StatusNoContent},
		resp.StatusCode,
	) {
		return &davError{h, resp.StatusCode, resp.Status}
	}

	return nil
}

type props struct {
	Status string   `xml:"DAV: status"`
	Name   string   `xml:"DAV: prop>displayname,omitempty"`
	Type   xml.Name `xml:"DAV: prop>resourcetype>collection,omitempty"`
	Size   string   `xml:"DAV: prop>getcontentlength,omitempty"`
}

type propfindresponse struct {
	Href  string  `xml:"DAV: href"`
	Props []props `xml:"DAV: propstat"`
}

func parsePropfind(data io.Reader, parse func(resp *propfindresponse) error) error {
	decoder := xml.NewDecoder(data)
	for t, _ := decoder.Token(); t != nil; t, _ = decoder.Token() {
		switch se := t.(type) {
		case xml.StartElement:
			if se.Name.Local == "response" {
				var response propfindresponse
				if e := decoder.DecodeElement(&response, &se); e == nil {
					if err := parse(&response); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func getProps(r *propfindresponse, status string) *props {
	for _, prop := range r.Props {
		if strings.Contains(prop.Status, status) {
			return &prop
		}
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

	payload := strings.NewReader(`<d:propfind xmlns:d='DAV:'>
			<d:prop>
				<d:displayname/>
				<d:resourcetype/>
				<d:getcontentlength/>
			</d:prop>
		</d:propfind>`)

	req, err := http.NewRequestWithContext(ctx, "PROPFIND", url, payload)
	if err != nil {
		return errors.WithStack(err)
	}
	req.Header.Set("Accept", "application/xml,text/xml")
	req.Header.Set("Accept-Charset", "utf-8")
	req.Header.Set("Content-Type", "application/xml;charset=UTF-8")
	req.Header.Set("Accept-Encoding", "") // Don't allow compressed response

	resp, err := b.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "List")
	}

	// ignore missing directories
	if resp.StatusCode == http.StatusNotFound {
		return drainAndClose(resp)
	}

	if resp.StatusCode != http.StatusMultiStatus {
		_ = drainAndClose(resp)
		return &davError{backend.Handle{Type: t}, resp.StatusCode, resp.Status}
	}

	err = parsePropfind(resp.Body, func(r *propfindresponse) error {
		// Tests expect context cancellation to cause failure in a very
		// specific way, but often this backend will have read the entire
		// response and be in the middle of decoding it at that time. Check
		// context cancellation and return the correct error if the context
		// was cancelled.
		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != nil {
				return err
			} else {
				return context.Canceled
			}
		default:
		}

		if p := getProps(r, "200 OK"); p != nil {
			// Skip folders
			if p.Type.Local == "collection" {
				return nil
			}
			size, err := strconv.ParseInt(p.Size, 10, 64)
			if err != nil {
				return err
			}
			file := backend.FileInfo{
				Name: p.Name,
				Size: size,
			}
			return fn(file)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if cerr := drainAndClose(resp); cerr != nil && err == nil {
		err = cerr
	}
	return err
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

// Warmup not implemented
func (b *Backend) Warmup(_ context.Context, _ []backend.Handle) ([]backend.Handle, error) {
	return []backend.Handle{}, nil
}
func (b *Backend) WarmupWait(_ context.Context, _ []backend.Handle) error { return nil }
