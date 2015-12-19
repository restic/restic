package backend_test

import (
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

	be, err := s3.Open(TestS3Server, "restictestbucket")
	OK(t, err)

	testBackend(be, t)

	del := be.(deleter)
	OK(t, del.Delete())
}
