package rest

import (
	"bytes"
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

// make sure the rest backend implements restic.Backend
var _ restic.Writabler = &restBackend{}

type restBackend struct {
	url    *url.URL
	sem    *backend.Semaphore
	client *http.Client
	backend.Layout
}

// Open opens the REST backend with the given config.
func Open(cfg Config, rt http.RoundTripper) (restic.Backend, error) {
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

	resp, err := http.Post(url.String(), "binary/octet-stream", strings.NewReader(""))
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
func (b *restBackend) Save(ctx context.Context, h restic.Handle, rd io.Reader) (err error) {
	if err := h.Valid(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// make sure that client.Post() cannot close the reader by wrapping it
	rd = ioutil.NopCloser(rd)

	b.sem.GetToken()
	resp, err := ctxhttp.Post(ctx, b.client, b.Filename(h), "binary/octet-stream", rd)
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

// Load returns a reader that yields the contents of the file at h at the
// given offset. If length is nonzero, only a portion of the file is
// returned. rd must be closed after use.
func (b *restBackend) Load(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
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
	req.Header.Add("Range", byteRange)
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

	b.sem.GetToken()
	resp, err := ctxhttp.Head(ctx, b.client, b.Filename(h))
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

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (b *restBackend) List(ctx context.Context, t restic.FileType) <-chan string {
	ch := make(chan string)

	url := b.Dirname(restic.Handle{Type: t})
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}

	b.sem.GetToken()
	resp, err := ctxhttp.Get(ctx, b.client, url)
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
		close(ch)
		return ch
	}

	dec := json.NewDecoder(resp.Body)
	var list []string
	if err = dec.Decode(&list); err != nil {
		close(ch)
		return ch
	}

	go func() {
		defer close(ch)
		for _, m := range list {
			select {
			case ch <- m:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch
}

// Close closes all open files.
func (b *restBackend) Close() error {
	// this does not need to do anything, all open files are closed within the
	// same function.
	return nil
}

// Writable implements restic.Writabler for the REST backend and returns
// whether the backend can be written to or must be considered as read-only.
//
// The methods probes the backend by writing to a temporary file and deleting
// it wright away. If that fails for any reason, the backend is considered to
// be read-only.
func (b *restBackend) Writable() bool {
	debug.Log("Writable()")

	const probeFilename = ".resticWriteCheck"
	h := restic.Handle{Type: restic.LockFile, Name: probeFilename}
	f := b.Filename(h)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rd := bytes.NewReader([]byte("restic"))

	b.sem.GetToken()
	resp, err := ctxhttp.Post(ctx, b.client, f, "binary/octet-stream", rd)
	fmt.Println(b.Layout.Paths())
	b.sem.ReleaseToken()
	if err != nil || resp.StatusCode != 200 {
		return false
	}
	if resp.Body.Close() != nil {
		return false
	}
	req, err := http.NewRequest("DELETE", f, nil)
	if err != nil {
		return false
	}
	b.sem.GetToken()
	resp, err = ctxhttp.Do(ctx, b.client, req)
	b.sem.ReleaseToken()

	if err != nil || resp.StatusCode != 200 {
		return false
	}
	if resp.Body.Close() != nil {
		return false
	}
	return true
}
