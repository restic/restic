package backend_test

import (
	"testing"

	bes3 "github.com/restic/restic/backend/s3"
)

func TestS3Backend(t *testing.T) {
	s, err := bes3.Open("play.minio.io:9000", "restictestbucket")

	if err != nil {
		t.Fatal(err)
	}

	testBackend(s, t)
}
