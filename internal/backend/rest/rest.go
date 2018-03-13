package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"

	"golang.org/x/net/context/ctxhttp"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/backend"
)

// make sure the rest backend implements restic.Backend
var _ restic.Backend = &restBackend{}

type restBackend struct {
	url    *url.URL
	sem    *backend.Semaphore
	client *http.Client
	backend.Layout
}

// the REST API protocol version is decided by HTTP request headers, these are the constants.
const (
	ContentTypeV1 = "application/vnd.x.restic.rest.v1"
	ContentTypeV2 = "application/vnd.x.restic.rest.v2"
)

// Open opens the REST backend with the given config.
func Open(cfg Config, rt http.RoundTripper) (*restBackend, error) {
	client := &http.Client{Transport: rt}

	sem, err := backend.NewSemaphore(cfg.Connections)
	if err != nil {
		return nil, err
	}

	// use url without trailing slash for layout
	url := cfg.URL.String()
	if url[len(url)-1] == '/' {
		url = url[:len(url)-1]
	}

	be := &restBackend{
		url:    cfg.URL,
		client: client,
		Layout: &backend.RESTLayout{URL: url, Join: path.Join},
		sem:    sem,
	}

	return be, nil
}

// Create creates a new REST on server configured in config.
func Create(cfg Config, rt http.RoundTripper) (restic.Backend, error) {
	be, err := Open(cfg, rt)
	if err != nil {
		return nil, err
	}

	_, err = be.Stat(context.TODO(), restic.Handle{Type: restic.ConfigFile})
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

	_, err = io.Copy(ioutil.Discard, resp.Body)
	if err != nil {
		return nil, err
	}

	err = resp.Body.Close()
	if err != nil {
		return nil, err
	}

	return be, nil
}

// Location returns this backend's location (the server's URL).
func (b *restBackend) Location() string {
	return b.url.String()
}

// Save stores data in the backend at the handle.
func (b *restBackend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	if err := h.Valid(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// make sure that client.Post() cannot close the reader by wrapping it
	req, err := http.NewRequest(http.MethodPost, b.Filename(h), ioutil.NopCloser(rd))
	if err != nil {
		return errors.Wrap(err, "NewRequest")
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Accept", ContentTypeV2)

	// explicitly set the content length, this prevents chunked encoding and
	// let's the server know what's coming.
	req.ContentLength = rd.Length()

	b.sem.GetToken()
	resp, err := ctxhttp.Do(ctx, b.client, req)
	b.sem.ReleaseToken()

	if resp != nil {
		defer func() {
			_, _ = io.Copy(ioutil.Discard, resp.Body)
			e := resp.Body.Close()

			if err == nil {
				err = errors.Wrap(e, "Close")
			}
		}()
	}

	if err != nil {
		return errors.Wrap(err, "client.Post")
	}

	if resp.StatusCode != 200 {
		return errors.Errorf("server response unexpected: %v (%v)", resp.Status, resp.StatusCode)
	}

	return nil
}

// ErrIsNotExist is returned whenever the requested file does not exist on the
// server.
type ErrIsNotExist struct {
	restic.Handle
}

func (e ErrIsNotExist) Error() string {
	return fmt.Sprintf("%v does not exist", e.Handle)
}

// IsNotExist returns true if the error was caused by a non-existing file.
func (b *restBackend) IsNotExist(err error) bool {
	err = errors.Cause(err)
	_, ok := err.(ErrIsNotExist)
	return ok
}

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (b *restBackend) Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	return backend.DefaultLoad(ctx, h, length, offset, b.openReader, fn)
}

func (b *restBackend) openReader(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
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

	req, err := http.NewRequest("GET", b.Filename(h), nil)
	if err != nil {
		return nil, errors.Wrap(err, "http.NewRequest")
	}

	byteRange := fmt.Sprintf("bytes=%d-", offset)
	if length > 0 {
		byteRange = fmt.Sprintf("bytes=%d-%d", offset, offset+int64(length)-1)
	}
	req.Header.Set("Range", byteRange)
	req.Header.Set("Accept", ContentTypeV2)
	debug.Log("Load(%v) send range %v", h, byteRange)

	b.sem.GetToken()
	resp, err := ctxhttp.Do(ctx, b.client, req)
	b.sem.ReleaseToken()

	if err != nil {
		if resp != nil {
			_, _ = io.Copy(ioutil.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		return nil, errors.Wrap(err, "client.Do")
	}

	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return nil, ErrIsNotExist{h}
	}

	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		_ = resp.Body.Close()
		return nil, errors.Errorf("unexpected HTTP response (%v): %v", resp.StatusCode, resp.Status)
	}

	return resp.Body, nil
}

// Stat returns information about a blob.
func (b *restBackend) Stat(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
	if err := h.Valid(); err != nil {
		return restic.FileInfo{}, err
	}

	req, err := http.NewRequest(http.MethodHead, b.Filename(h), nil)
	if err != nil {
		return restic.FileInfo{}, errors.Wrap(err, "NewRequest")
	}
	req.Header.Set("Accept", ContentTypeV2)

	b.sem.GetToken()
	resp, err := ctxhttp.Do(ctx, b.client, req)
	b.sem.ReleaseToken()
	if err != nil {
		return restic.FileInfo{}, errors.Wrap(err, "client.Head")
	}

	_, _ = io.Copy(ioutil.Discard, resp.Body)
	if err = resp.Body.Close(); err != nil {
		return restic.FileInfo{}, errors.Wrap(err, "Close")
	}

	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return restic.FileInfo{}, ErrIsNotExist{h}
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

// Test returns true if a blob of the given type and name exists in the backend.
func (b *restBackend) Test(ctx context.Context, h restic.Handle) (bool, error) {
	_, err := b.Stat(ctx, h)
	if err != nil {
		return false, nil
	}

	return true, nil
}

// Remove removes the blob with the given name and type.
func (b *restBackend) Remove(ctx context.Context, h restic.Handle) error {
	if err := h.Valid(); err != nil {
		return err
	}

	req, err := http.NewRequest("DELETE", b.Filename(h), nil)
	if err != nil {
		return errors.Wrap(err, "http.NewRequest")
	}
	req.Header.Set("Accept", ContentTypeV2)

	b.sem.GetToken()
	resp, err := ctxhttp.Do(ctx, b.client, req)
	b.sem.ReleaseToken()

	if err != nil {
		return errors.Wrap(err, "client.Do")
	}

	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return ErrIsNotExist{h}
	}

	if resp.StatusCode != 200 {
		return errors.Errorf("blob not removed, server response: %v (%v)", resp.Status, resp.StatusCode)
	}

	_, err = io.Copy(ioutil.Discard, resp.Body)
	if err != nil {
		return errors.Wrap(err, "Copy")
	}

	return errors.Wrap(resp.Body.Close(), "Close")
}

// List runs fn for each file in the backend which has the type t. When an
// error occurs (or fn returns an error), List stops and returns it.
func (b *restBackend) List(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error {
	url := b.Dirname(restic.Handle{Type: t})
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return errors.Wrap(err, "NewRequest")
	}
	req.Header.Set("Accept", ContentTypeV2)

	b.sem.GetToken()
	resp, err := ctxhttp.Do(ctx, b.client, req)
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
func (b *restBackend) listv1(ctx context.Context, t restic.FileType, resp *http.Response, fn func(restic.FileInfo) error) error {
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
func (b *restBackend) listv2(ctx context.Context, t restic.FileType, resp *http.Response, fn func(restic.FileInfo) error) error {
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
func (b *restBackend) Close() error {
	// this does not need to do anything, all open files are closed within the
	// same function.
	return nil
}

// Remove keys for a specified backend type.
func (b *restBackend) removeKeys(ctx context.Context, t restic.FileType) error {
	return b.List(ctx, t, func(fi restic.FileInfo) error {
		return b.Remove(ctx, restic.Handle{Type: t, Name: fi.Name})
	})
}

// Delete removes all data in the backend.
func (b *restBackend) Delete(ctx context.Context) error {
	alltypes := []restic.FileType{
		restic.DataFile,
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
