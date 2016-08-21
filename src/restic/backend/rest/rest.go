package rest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/pkg/errors"

	"restic/backend"
)

const connLimit = 10

// restPath returns the path to the given resource.
func restPath(url *url.URL, h backend.Handle) string {
	u := *url

	var dir string

	switch h.Type {
	case backend.Config:
		dir = ""
		h.Name = "config"
	case backend.Data:
		dir = backend.Paths.Data
	case backend.Snapshot:
		dir = backend.Paths.Snapshots
	case backend.Index:
		dir = backend.Paths.Index
	case backend.Lock:
		dir = backend.Paths.Locks
	case backend.Key:
		dir = backend.Paths.Keys
	default:
		dir = string(h.Type)
	}

	u.Path = path.Join(url.Path, dir, h.Name)

	return u.String()
}

type restBackend struct {
	url      *url.URL
	connChan chan struct{}
	client   http.Client
}

// Open opens the REST backend with the given config.
func Open(cfg Config) (backend.Backend, error) {
	connChan := make(chan struct{}, connLimit)
	for i := 0; i < connLimit; i++ {
		connChan <- struct{}{}
	}
	tr := &http.Transport{}
	client := http.Client{Transport: tr}

	return &restBackend{url: cfg.URL, connChan: connChan, client: client}, nil
}

// Location returns this backend's location (the server's URL).
func (b *restBackend) Location() string {
	return b.url.String()
}

// Load returns the data stored in the backend for h at the given offset
// and saves it in p. Load has the same semantics as io.ReaderAt.
func (b *restBackend) Load(h backend.Handle, p []byte, off int64) (n int, err error) {
	if err := h.Valid(); err != nil {
		return 0, err
	}

	// invert offset
	if off < 0 {
		info, err := b.Stat(h)
		if err != nil {
			return 0, err
		}

		if -off > info.Size {
			off = 0
		} else {
			off = info.Size + off
		}
	}

	req, err := http.NewRequest("GET", restPath(b.url, h), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", off, off+int64(len(p))))
	<-b.connChan
	resp, err := b.client.Do(req)
	b.connChan <- struct{}{}

	if resp != nil {
		defer func() {
			e := resp.Body.Close()

			if err == nil {
				err = e
			}
		}()
	}

	if err != nil {
		return 0, err
	}
	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return 0, fmt.Errorf("unexpected HTTP response code %v", resp.StatusCode)
	}

	return io.ReadFull(resp.Body, p)
}

// Save stores data in the backend at the handle.
func (b *restBackend) Save(h backend.Handle, p []byte) (err error) {
	if err := h.Valid(); err != nil {
		return err
	}

	<-b.connChan
	resp, err := b.client.Post(restPath(b.url, h), "binary/octet-stream", bytes.NewReader(p))
	b.connChan <- struct{}{}

	if resp != nil {
		defer func() {
			e := resp.Body.Close()

			if err == nil {
				err = e
			}
		}()
	}

	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("unexpected HTTP response code %v", resp.StatusCode)
	}

	return nil
}

// Stat returns information about a blob.
func (b *restBackend) Stat(h backend.Handle) (backend.BlobInfo, error) {
	if err := h.Valid(); err != nil {
		return backend.BlobInfo{}, err
	}

	<-b.connChan
	resp, err := b.client.Head(restPath(b.url, h))
	b.connChan <- struct{}{}
	if err != nil {
		return backend.BlobInfo{}, err
	}

	if err = resp.Body.Close(); err != nil {
		return backend.BlobInfo{}, err
	}

	if resp.StatusCode != 200 {
		return backend.BlobInfo{}, fmt.Errorf("unexpected HTTP response code %v", resp.StatusCode)
	}

	if resp.ContentLength < 0 {
		return backend.BlobInfo{}, errors.New("negative content length")
	}

	bi := backend.BlobInfo{
		Size: resp.ContentLength,
	}

	return bi, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (b *restBackend) Test(t backend.Type, name string) (bool, error) {
	_, err := b.Stat(backend.Handle{Type: t, Name: name})
	if err != nil {
		return false, nil
	}

	return true, nil
}

// Remove removes the blob with the given name and type.
func (b *restBackend) Remove(t backend.Type, name string) error {
	h := backend.Handle{Type: t, Name: name}
	if err := h.Valid(); err != nil {
		return err
	}

	req, err := http.NewRequest("DELETE", restPath(b.url, h), nil)
	if err != nil {
		return err
	}
	<-b.connChan
	resp, err := b.client.Do(req)
	b.connChan <- struct{}{}

	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return errors.New("blob not removed")
	}

	return resp.Body.Close()
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (b *restBackend) List(t backend.Type, done <-chan struct{}) <-chan string {
	ch := make(chan string)

	url := restPath(b.url, backend.Handle{Type: t})
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}

	<-b.connChan
	resp, err := b.client.Get(url)
	b.connChan <- struct{}{}

	if resp != nil {
		defer resp.Body.Close()
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
