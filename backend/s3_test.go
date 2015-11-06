package backend_test

import (
	"testing"

	bes3 "github.com/restic/restic/backend/s3"
	. "github.com/restic/restic/test"
)

func setupS3Backend(t *testing.T) *bes3.S3Backend {
	return bes3.Open("play.minio.io:9000", "restictestbucket")
}

func TestS3Backend(t *testing.T) {
	s := setupS3Backend(t)

	testBackend(s, t)
}
