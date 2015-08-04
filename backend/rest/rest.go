package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/restic/restic/backend"
)

const connLimit = 10

// Returns the url of the resource
func restPath(url *url.URL, t backend.Type, name string) string {
	if t == backend.Config {
		return url.String() + "/" + string(t)
	}

	return url.String() + "/" + string(t) + "/" + name
}

type RestBlob struct {
	b     *Rest
	buf   *bytes.Buffer
	final bool
}

func (rb *RestBlob) Write(p []byte) (int, error) {
	if rb.final {
		return 0, errors.New("blob already closed")
	}
	n, err := rb.buf.Write(p)
	return n, err
}

func (rb *RestBlob) Read(p []byte) (int, error) {
	return rb.buf.Read(p)
}

func (rb *RestBlob) Close() error {
	rb.final = true
	rb.buf.Reset()
	return nil
}

// Finalize moves the data blob to the final location for type and name.
func (rb *RestBlob) Finalize(t backend.Type, name string) error {
	if rb.final {
		return errors.New("Already finalized")
	}

	rb.final = true

	// Check key does not already exist
	resp, err := http.Get(restPath(rb.b.url, t, name))
	if err == nil && resp.StatusCode == 200 {
		return errors.New("Key already exists")
	}

	<-rb.b.connChan
	_, err2 := http.Post(restPath(rb.b.url, t, name), "binary/octet-stream", rb.buf)
	rb.b.connChan <- struct{}{}
	rb.buf.Reset()
	return err2
}

// Size returns the number of bytes written to the backend so far.
func (rb *RestBlob) Size() uint {
	return uint(rb.buf.Len())
}

// A simple REST backend.
type Rest struct {
	url      *url.URL
	connChan chan struct{}
}

// Open opens the http backend at the specified url.
func Open(url *url.URL) (*Rest, error) {
	connChan := make(chan struct{}, connLimit)
	for i := 0; i < connLimit; i++ {
		connChan <- struct{}{}
	}
	return &Rest{url: url, connChan: connChan}, nil
}

// Location returns a string that specifies the location of the repository, like a URL.
func (b *Rest) Location() string {
	return b.url.Host
}

// Create creates a new Blob. The data is available only after Finalize()
// has been called on the returned Blob.
func (b *Rest) Create() (backend.Blob, error) {
	blob := RestBlob{
		b:   b,
		buf: &bytes.Buffer{},
	}
	return &blob, nil
}

// Get returns an io.ReadCloser for the Blob with the given name of type t.
func (b *Rest) Get(t backend.Type, name string) (io.ReadCloser, error) {
	resp, err := http.Get(restPath(b.url, t, name))
	if err == nil && resp.StatusCode != 200 {
		err = errors.New("Not found")
	}
	return resp.Body, err
}

// GetReader returns an io.ReadCloser for the Blob with the given name of
// type t at offset and length.
func (b *Rest) GetReader(t backend.Type, name string, offset, length uint) (io.ReadCloser, error) {
	rc, err := b.Get(t, name)
	if err != nil {
		return nil, err
	}

	n, errc := io.CopyN(ioutil.Discard, rc, int64(offset))
	if errc != nil {
		return nil, errc
	} else if n != int64(offset) {
		return nil, fmt.Errorf("less bytes read than expected, read: %d, expected: %d", n, offset)
	}

	if length == 0 {
		return rc, nil
	}

	return backend.LimitReadCloser(rc, int64(length)), nil
}

// Test a boolean value whether a Blob with the name and type exists.
func (b *Rest) Test(t backend.Type, name string) (bool, error) {
	found := false

	req, e1 := http.NewRequest("HEAD", restPath(b.url, t, name), nil)
	if e1 != nil {
		return found, e1
	}

	resp, err := http.DefaultClient.Do(req)
	if resp.StatusCode == 200 {
		found = true
	}
	return found, err
}

// Remove removes a Blob with type t and name.
func (b *Rest) Remove(t backend.Type, name string) error {
	req, err := http.NewRequest("DELETE", restPath(b.url, t, name), nil)
	if err != nil {
		return err
	}

	_, err2 := http.DefaultClient.Do(req)
	return err2
}

// Close the backend
func (b *Rest) Close() error {
	return nil
}

// List returns a channel that yields all names of blobs of type t in
// lexicographic order. A goroutine is started for this. If the channel
// done is closed, sending stops.
func (b *Rest) List(t backend.Type, done <-chan struct{}) <-chan string {
	ch := make(chan string)

	resp, e1 := http.Get(restPath(b.url, t, ""))
	if e1 != nil {
		close(ch)
		return ch
	}

	data, e2 := ioutil.ReadAll(resp.Body)
	if e2 != nil {
		close(ch)
		return ch
	}

	var list []string
	e3 := json.Unmarshal(data, &list)
	if e3 != nil {
		close(ch)
		return ch
	}

	go func() {
		defer close(ch)
		for _, m := range list {
			fmt.Println(m)
			select {
			case ch <- m:
			case <-done:
				return
			}
		}
	}()

	return ch
}
