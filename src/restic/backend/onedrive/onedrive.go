package onedrive

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"golang.org/x/oauth2"

	"restic/backend"
	"restic/debug"
)

const connLimit = 10
const onedriveBase = "https://api.onedrive.com/v1.0/drive/special/approot:/"

type onedrive struct {
	client   *http.Client
	connChan chan struct{}
}

var oauthConf = &oauth2.Config{
	Scopes: []string{
		"wl.signin",          // Allow single sign-on capabilities
		"wl.offline_access",  // Allow receiving a refresh token
		"onedrive.readwrite", // r/w perms to all of a user's OneDrive files
	},
	Endpoint: oauth2.Endpoint{
		AuthURL:  "https://login.live.com/oauth20_authorize.srf",
		TokenURL: "https://login.live.com/oauth20_token.srf",
	},
}

// Items represents a collection of Items
type items struct {
	Collection []*item `json:"value"`
	NextLink   string  `json:"@odata.nextLink"`
}

// The Item resource type represents metadata for an item in OneDrive.
// Since we only need the name and the size, this structure incomplete.
// see: https://dev.onedrive.com/resources/item.htm
type item struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// Action is used for the paramters of an onedrive request
type action string

const (
	list    action = ":/children?select=name"
	content        = ":/content"
	none           = ""
)

func (be *onedrive) odPath(t backend.Type, name string, a action) string {
	if t == backend.Config || name == "" {
		return onedriveBase + string(t) + string(a)
	}
	return onedriveBase + string(t) + "/" + name + string(a)
}

// Open opens the onedrive backend.
func Open(cfg Config) (backend.Backend, error) {
	debug.Log("onedrive.Open", "open, config %#v", cfg)

	oauthConf.ClientID = cfg.ClientID
	oauthConf.ClientSecret = cfg.ClientSecret

	client := oauthConf.Client(oauth2.NoContext, &cfg.Token)

	be := &onedrive{client: client}
	be.createConnections()

	return be, nil
}

func (be *onedrive) createConnections() {
	be.connChan = make(chan struct{}, connLimit)
	for i := 0; i < connLimit; i++ {
		be.connChan <- struct{}{}
	}
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (be *onedrive) List(t backend.Type, done <-chan struct{}) <-chan string {
	debug.Log("onedrive.List", "listing %v", t)
	ch := make(chan string)

	collection := []*item{}

	path := be.odPath(t, "", list)

	for path != "" {
		debug.Log("onedrive.List", "path %v", path)
		resp, err := be.client.Get(path)
		if resp != nil {
			defer resp.Body.Close()
		}
		if err != nil {
			close(ch)
			return ch
		}

		respData, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			close(ch)
			return ch
		}

		var jsonData items
		err = json.Unmarshal([]byte(respData), &jsonData)
		if err != nil {
			close(ch)
			return ch
		}

		collection = append(collection, jsonData.Collection...)

		// If a collection exceeds the default page size (200 items),
		// the @odata.nextLink property is returned and we have to fetch the
		// next page.
		path = jsonData.NextLink
	}

	go func() {
		defer close(ch)
		for _, obj := range collection {
			m := obj.Name

			if m == "" {
				continue
			}

			select {
			case ch <- m:
			case <-done:
				return
			}
		}
	}()

	return ch
}

// Load returns the data stored in the backend for h at the given offset
// and saves it in p. Load has the same semantics as io.ReaderAt.
func (be *onedrive) Load(h backend.Handle, p []byte, off int64) (n int, err error) {
	debug.Log("onedrive.Load", "load offset %v length %v", off, len(p))
	path := be.odPath(h.Type, h.Name, content)

	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		debug.Log("onedrive.Load", "  err %v", err)
		return 0, err
	}

	transport := be.client.Transport
	resp, err := transport.RoundTrip(req)
	if err != nil {
		debug.Log("onedrive.Load", "  err %v", err)
		return 0, err
	}
	if resp.StatusCode != 302 {
		debug.Log("onedrive.Load()", "no redirect - resp %v", resp)
		return 0, errors.New("no redirect to content location")
	}

	req, err = http.NewRequest("GET", resp.Header.Get("Location"), nil)
	if err != nil {
		debug.Log("onedrive.Load", "  err %v", err)
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+int64(len(p)-1)))

	resp, err = be.client.Do(req)
	if err != nil {
		debug.Log("onedrive.Load", "  err %v", err)
		return 0, err
	}

	<-be.connChan
	defer func() {
		be.connChan <- struct{}{}
	}()

	n, err = io.ReadFull(resp.Body, p)
	if err != nil {
		debug.Log("onedrive.Load", "GetObject() err %v", err)
		return 0, err
	}

	return n, nil
}

// Save stores data in the backend at the handle.
func (be onedrive) Save(h backend.Handle, p []byte) (err error) {
	debug.Log("onedrive.Save", "name %v", h.Name)
	path := be.odPath(h.Type, h.Name, content)

	// Check file does not already exist
	resp, err := be.client.Get(path)
	if err == nil && resp.StatusCode == 200 {
		debug.Log("onedrive.Save", "%v already exists", h)
		return errors.New("key already exists")
	}

	<-be.connChan
	defer func() {
		be.connChan <- struct{}{}
	}()

	debug.Log("onedrive.Save", "PutObject(%v, %v)",
		path, int64(len(p)))

	req, err := http.NewRequest("PUT", path, bytes.NewReader(p))
	if err != nil {
		debug.Log("onedrive.Save", "  err %v", err)
		return err
	}

	resp, err = be.client.Do(req)
	if err != nil {
		debug.Log("onedrive.Save", "  err %v ", err)
		return err
	}

	if resp.StatusCode >= 400 {
		debug.Log("onedrive.Save", "  resp %v ", resp)
		return errors.New("Invalid response code")
	}

	debug.Log("onedrive.Save", "%v -> %v bytes, err %#v", path, resp.ContentLength, err)

	return err
}

// Stat returns information about a blob.
func (be *onedrive) Stat(h backend.Handle) (backend.BlobInfo, error) {
	debug.Log("onedrive.Stat", "name %v", h.Name)
	path := be.odPath(h.Type, h.Name, none)

	resp, err := be.client.Get(path)
	if err != nil {
		debug.Log("onedrive.Stat", "GetObject() err %v", err)
		return backend.BlobInfo{}, err
	}

	respData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		debug.Log("onedrive.Stat", "GetObject() err %v", err)
		return backend.BlobInfo{}, err
	}

	var jsonData item
	err = json.Unmarshal([]byte(respData), &jsonData)
	if err != nil {
		debug.Log("onedrive.Stat", "GetObject() err %v", err)
		return backend.BlobInfo{}, err
	}

	return backend.BlobInfo{Size: jsonData.Size}, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (be *onedrive) Test(t backend.Type, name string) (bool, error) {
	debug.Log("onedrive.Test", "name %v", name)
	found := false
	path := be.odPath(t, name, none)
	resp, err := be.client.Get(path)
	if err == nil && resp.StatusCode == 200 {
		found = true
	}

	// If error, then not found
	return found, nil
}

// Remove removes the blob with the given name and type.
func (be *onedrive) Remove(t backend.Type, name string) error {
	debug.Log("onedrive.Remove", "name %v", name)
	path := be.odPath(t, name, none)
	req, err := http.NewRequest("DELETE", path, nil)
	if err != nil {
		debug.Log("onedrive.Remove", "  err %v", err)
		return err
	}

	_, err = be.client.Do(req)
	if err != nil {
		debug.Log("onedrive.Remove", "  err %v", err)
		return err
	}
	return err
}

// Location returns this backend's location (the bucket name).
// TODO: return path?
func (be *onedrive) Location() string {
	return "restic"
}

// Close does nothing
func (be *onedrive) Close() error { return nil }
