package rest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"restic"
	"strings"

	"restic/errors"

	"restic/backend"
)

const connLimit = 10

// restPath returns the path to the given resource.
func restPath(url *url.URL, h restic.Handle) string {
	u := *url

	var dir string

	switch h.Type {
	case restic.ConfigFile:
		dir = ""
		h.Name = "config"
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

	u.Path = path.Join(url.Path, dir, h.Name)

	return u.String()
}

type restBackend struct {
	url      *url.URL
	connChan chan struct{}
	client   http.Client
}

// Open opens the REST backend with the given config.
func Open(cfg Config) (restic.Backend, error) {
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
func (b *restBackend) Load(h restic.Handle, p []byte, off int64) (n int, err error) {
	if err := h.Valid(); err != nil {
		return 0, err
	}

	// invert offset
	if off < 0 {
		info, err := b.Stat(h)
		if err != nil {
			return 0, errors.Wrap(err, "Stat")
		}

		if -off > info.Size {
			off = 0
		} else {
			off = info.Size + off
		}
	}

	req, err := http.NewRequest("GET", restPath(b.url, h), nil)
	if err != nil {
		return 0, errors.Wrap(err, "http.NewRequest")
	}
	req.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", off, off+int64(len(p))))
	<-b.connChan
	resp, err := b.client.Do(req)
	b.connChan <- struct{}{}

	if resp != nil {
		defer func() {
			e := resp.Body.Close()

			if err == nil {
				err = errors.Wrap(e, "Close")
			}
		}()
	}

	if err != nil {
		return 0, errors.Wrap(err, "client.Do")
	}
	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return 0, errors.Errorf("unexpected HTTP response code %v", resp.StatusCode)
	}

	return io.ReadFull(resp.Body, p)
}

// Save stores data in the backend at the handle.
func (b *restBackend) Save(h restic.Handle, p []byte) (err error) {
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
				err = errors.Wrap(e, "Close")
			}
		}()
	}

	if err != nil {
		return errors.Wrap(err, "client.Post")
	}

	if resp.StatusCode != 200 {
		return errors.Errorf("unexpected HTTP response code %v", resp.StatusCode)
	}

	return nil
}

// Stat returns information about a blob.
func (b *restBackend) Stat(h restic.Handle) (restic.FileInfo, error) {
	if err := h.Valid(); err != nil {
		return restic.FileInfo{}, err
	}

	<-b.connChan
	resp, err := b.client.Head(restPath(b.url, h))
	b.connChan <- struct{}{}
	if err != nil {
		return restic.FileInfo{}, errors.Wrap(err, "client.Head")
	}

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
func (b *restBackend) Test(t restic.FileType, name string) (bool, error) {
	_, err := b.Stat(restic.Handle{Type: t, Name: name})
	if err != nil {
		return false, nil
	}

	return true, nil
}

// Remove removes the blob with the given name and type.
func (b *restBackend) Remove(t restic.FileType, name string) error {
	h := restic.Handle{Type: t, Name: name}
	if err := h.Valid(); err != nil {
		return err
	}

	req, err := http.NewRequest("DELETE", restPath(b.url, h), nil)
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
		return errors.New("blob not removed")
	}

	return resp.Body.Close()
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (b *restBackend) List(t restic.FileType, done <-chan struct{}) <-chan string {
	ch := make(chan string)

	url := restPath(b.url, restic.Handle{Type: t})
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
