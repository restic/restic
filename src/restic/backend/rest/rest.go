package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"restic"
	"strings"

	"restic/debug"
	"restic/errors"

	"restic/backend"
)

const connLimit = 40

// make sure the rest backend implements restic.Backend
var _ restic.Backend = &restBackend{}

type restBackend struct {
	url      *url.URL
	connChan chan struct{}
	client   http.Client
	backend.Layout
}

// Open opens the REST backend with the given config.
func Open(cfg Config) (restic.Backend, error) {
	connChan := make(chan struct{}, connLimit)
	for i := 0; i < connLimit; i++ {
		connChan <- struct{}{}
	}

	client := http.Client{Transport: backend.Transport()}

	// use url without trailing slash for layout
	url := cfg.URL.String()
	if url[len(url)-1] == '/' {
		url = url[:len(url)-1]
	}

	be := &restBackend{
		url:      cfg.URL,
		connChan: connChan,
		client:   client,
		Layout:   &backend.CloudLayout{URL: url, Join: path.Join},
	}

	return be, nil
}

// Create creates a new REST on server configured in config.
func Create(cfg Config) (restic.Backend, error) {
	be, err := Open(cfg)
	if err != nil {
		return nil, err
	}

	_, err = be.Stat(restic.Handle{Type: restic.ConfigFile})
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
func (b *restBackend) Save(h restic.Handle, rd io.Reader) (err error) {
	if err := h.Valid(); err != nil {
		return err
	}

	// make sure that client.Post() cannot close the reader by wrapping it in
	// backend.Closer, which has a noop method.
	rd = backend.Closer{Reader: rd}

	<-b.connChan
	resp, err := b.client.Post(b.Filename(h), "binary/octet-stream", rd)
	b.connChan <- struct{}{}

	if resp != nil {
		defer func() {
			io.Copy(ioutil.Discard, resp.Body)
			e := resp.Body.Close()

			if err == nil {
				err = errors.Wrap(e, "Close")
			}
		}()
	}

	if err != nil {
		return errors.Wrap(err, "client.Post")
	}

	// fmt.Printf("status is %v (%v)\n", resp.Status, resp.StatusCode)
	if resp.StatusCode != 200 {
		return errors.Errorf("unexpected HTTP response code %v", resp.StatusCode)
	}

	return nil
}

// Load returns a reader that yields the contents of the file at h at the
// given offset. If length is nonzero, only a portion of the file is
// returned. rd must be closed after use.
func (b *restBackend) Load(h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
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

	<-b.connChan
	resp, err := b.client.Do(req)
	b.connChan <- struct{}{}

	if err != nil {
		if resp != nil {
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		}
		return nil, errors.Wrap(err, "client.Do")
	}

	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
		return nil, errors.Errorf("unexpected HTTP response code %v", resp.StatusCode)
	}

	return resp.Body, nil
}

// Stat returns information about a blob.
func (b *restBackend) Stat(h restic.Handle) (restic.FileInfo, error) {
	if err := h.Valid(); err != nil {
		return restic.FileInfo{}, err
	}

	<-b.connChan
	resp, err := b.client.Head(b.Filename(h))
	b.connChan <- struct{}{}
	if err != nil {
		return restic.FileInfo{}, errors.Wrap(err, "client.Head")
	}

	io.Copy(ioutil.Discard, resp.Body)
	if err = resp.Body.Close(); err != nil {
		return restic.FileInfo{}, errors.Wrap(err, "Close")
	}

	if resp.StatusCode != 200 {
		return restic.FileInfo{}, errors.Errorf("unexpected HTTP response code %v", resp.StatusCode)
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
func (b *restBackend) Test(h restic.Handle) (bool, error) {
	_, err := b.Stat(h)
	if err != nil {
		return false, nil
	}

	return true, nil
}

// Remove removes the blob with the given name and type.
func (b *restBackend) Remove(h restic.Handle) error {
	if err := h.Valid(); err != nil {
		return err
	}

	req, err := http.NewRequest("DELETE", b.Filename(h), nil)
	if err != nil {
		return errors.Wrap(err, "http.NewRequest")
	}
	<-b.connChan
	resp, err := b.client.Do(req)
	b.connChan <- struct{}{}

	if err != nil {
		return errors.Wrap(err, "client.Do")
	}

	if resp.StatusCode != 200 {
		return errors.Errorf("blob not removed, server response: %v (%v)", resp.Status, resp.StatusCode)
	}

	io.Copy(ioutil.Discard, resp.Body)
	return resp.Body.Close()
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (b *restBackend) List(t restic.FileType, done <-chan struct{}) <-chan string {
	ch := make(chan string)

	url := b.Dirname(restic.Handle{Type: t})
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}

	<-b.connChan
	resp, err := b.client.Get(url)
	b.connChan <- struct{}{}

	if resp != nil {
		defer func() {
			io.Copy(ioutil.Discard, resp.Body)
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
			case <-done:
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
