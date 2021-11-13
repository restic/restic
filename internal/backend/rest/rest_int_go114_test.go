//go:build go1.14 && !go1.18
// +build go1.14,!go1.18

// missing eof error is fixed in golang >= 1.17.3 or >= 1.16.10
// remove the workaround from rest.go when the minimum golang version
// supported by restic reaches 1.18.

package rest_test

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/restic/restic/internal/backend/rest"
	"github.com/restic/restic/internal/restic"
)

func TestZeroLengthRead(t *testing.T) {
	// Test workaround for https://github.com/golang/go/issues/46071. Can be removed once this is fixed in Go
	// and the minimum golang version supported by restic includes the fix.
	numRequests := 0
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		numRequests++
		t.Logf("req %v %v", req.Method, req.URL.Path)
		if req.Method == "GET" {
			res.Header().Set("Content-Length", "42")
			// Now the handler fails for some reason and is unable to send data
			return
		}

		t.Errorf("unhandled request %v %v", req.Method, req.URL.Path)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	srvURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	cfg := rest.Config{
		Connections: 5,
		URL:         srvURL,
	}
	be, err := rest.Open(cfg, srv.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = be.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()

	err = be.Load(context.TODO(), restic.Handle{Type: restic.ConfigFile}, 0, 0, func(rd io.Reader) error {
		_, err := ioutil.ReadAll(rd)
		if err == nil {
			t.Fatal("ReadAll should have returned an 'Unexpected EOF' error")
		}
		return nil
	})
	if err == nil {
		t.Fatal("Got no unexpected EOF error")
	}
}
