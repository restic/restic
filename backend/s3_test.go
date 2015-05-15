package backend_test

import (
	"testing"

	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/s3"
	"github.com/mitchellh/goamz/testutil"

	bes3 "github.com/restic/restic/backend/s3"
	. "github.com/restic/restic/test"
)

var testServer = testutil.NewHTTPServer()

func setupS3Backend(t *testing.T) *bes3.S3 {
	testServer.Start()
	auth := aws.Auth{"abc", "123", ""}
	service := s3.New(auth, aws.Region{Name: "faux-region-1", S3Endpoint: testServer.URL})
	bucket := service.Bucket("testbucket")
	err := bucket.PutBucket("private")
	OK(t, err)

	t.Logf("created s3 backend locally at %s", testServer.URL)

	return bes3.S3{bucket: bucket, path: "testbucket"}
}

func teardownS3Backend(t *testing.T, b *bes3.S3) {
	if !*TestCleanup {
		t.Logf("leaving backend at %s\n", b.Location())
		return
	}

	testServer.Flush()
}

func TestS3Backend(t *testing.T) {
	s := setupS3Backend(t)
	defer teardownS3Backend(t, s)

	testBackend(s, t)
}
