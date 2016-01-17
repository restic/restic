package backend_test

import (
	"net/url"
	"os"
	"testing"

	"github.com/restic/restic/backend/s3"
	. "github.com/restic/restic/test"
)

type deleter interface {
	Delete() error
}

func TestS3Backend(t *testing.T) {
	if TestS3Server == "" {
		t.Skip("s3 test server not available")
	}

	url, err := url.Parse(TestS3Server)
	OK(t, err)

	cfg := s3.Config{
		Endpoint: url.Host,
		Bucket:   "restictestbucket",
		KeyID:    os.Getenv("AWS_ACCESS_KEY_ID"),
		Secret:   os.Getenv("AWS_SECRET_ACCESS_KEY"),
	}

	if url.Scheme == "http" {
		cfg.UseHTTP = true
	}

	be, err := s3.Open(cfg)
	OK(t, err)

	testBackend(be, t)

	del := be.(deleter)
	OK(t, del.Delete())
}
