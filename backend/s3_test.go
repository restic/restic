package backend_test

import (
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

	be, err := s3.Open(s3.Config{
		URL:    TestS3Server,
		Bucket: "restictestbucket",
		KeyID:  os.Getenv("AWS_ACCESS_KEY_ID"),
		Secret: os.Getenv("AWS_SECRET_ACCESS_KEY"),
	})
	OK(t, err)

	testBackend(be, t)

	del := be.(deleter)
	OK(t, del.Delete())
}
