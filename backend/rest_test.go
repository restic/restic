package backend_test

import (
	"github.com/restic/restic/backend/rest"
	"net/url"
	"testing"
)

func setupRestBackend(t *testing.T) *rest.Rest {
	url, _ := url.Parse("http://localhost:8000")
	backend, _ := rest.Open(url)
	return backend
}

func TestRestBackend(t *testing.T) {
	s := setupRestBackend(t)
	testBackend(s, t)
}
