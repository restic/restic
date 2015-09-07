package rest

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/restic/restic/backend"
)

const connLimit = 10

// Returns the url of the resource
func restPath(url *url.URL, t backend.Type, name string) string {
	location := url.String()
	if !strings.HasSuffix(location, "/") {
		location += "/"
	}
	if t == backend.Config {
		return location + string(t)
	}
	return location + string(t) + "/" + name
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
		return errors.New("already finalized")
	}

	rb.final = true

	<-rb.b.connChan
	client := *rb.b.client
	resp, err := client.Post(restPath(rb.b.url, t, name), "binary/octet-stream", rb.buf)
	defer resp.Body.Close()
	if resp.StatusCode == 409 {
		err = errors.New("already exists")
	}
	rb.b.connChan <- struct{}{}
	rb.buf.Reset()

	return err
}

// Size returns the number of bytes written to the backend so far.
func (rb *RestBlob) Size() uint {
	return uint(rb.buf.Len())
}

// A simple REST backend.
type Rest struct {
	url      *url.URL
	connChan chan struct{}
	client   *http.Client
}

// Open opens the http backend at the specified url.
func Open(url *url.URL) (*Rest, error) {
	connChan := make(chan struct{}, connLimit)
	for i := 0; i < connLimit; i++ {
		connChan <- struct{}{}
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := http.Client{Transport: tr}
	return &Rest{url: url, connChan: connChan, client: &client}, nil
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
	resp, err := b.client.Get(restPath(b.url, t, name))
	if err == nil && resp.StatusCode != 200 {
		err = errors.New("blob not found")
	}
	return resp.Body, err
}

// GetReader returns an io.ReadCloser for the Blob with the given name of
// type t at offset and length.
func (b *Rest) GetReader(t backend.Type, name string, offset, length uint) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", restPath(b.url, t, name), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length))

	client := *b.client
	resp, errg := client.Do(req)
	if errg != nil {
		return nil, errg
	}

	return backend.LimitReadCloser(resp.Body, int64(length)), nil
}

// Test a boolean value whether a Blob with the name and type exists.
func (b *Rest) Test(t backend.Type, name string) (bool, error) {
	found := false

	req, err := http.NewRequest("HEAD", restPath(b.url, t, name), nil)
	if err != nil {
		return found, err
	}

	client := *b.client
	resp, errh := client.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		found = true
	}
	return found, errh
}

// Remove removes a Blob with type t and name.
func (b *Rest) Remove(t backend.Type, name string) error {
	req, err := http.NewRequest("DELETE", restPath(b.url, t, name), nil)
	if err != nil {
		return err
	}

	client := *b.client
	resp, errd := client.Do(req)
	defer resp.Body.Close()
	return errd
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

	client := *b.client
	resp, err := client.Get(restPath(b.url, t, ""))
	defer resp.Body.Close()
	if err != nil {
		close(ch)
		return ch
	}

	data, errd := ioutil.ReadAll(resp.Body)
	if errd != nil {
		close(ch)
		return ch
	}

	var list []string
	errj := json.Unmarshal(data, &list)
	if errj != nil {
		close(ch)
		return ch
	}

	go func() {
		defer close(ch)
		sort.Strings(list)
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
