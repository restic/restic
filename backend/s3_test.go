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
	be, err := s3.Create("127.0.0.1:9000", "restictestbucket")
	OK(t, err)

	testBackend(be, t)

	del := be.(deleter)
	OK(t, del.Delete())
}
