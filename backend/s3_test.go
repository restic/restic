package backend_test

import (
	"testing"

	"gopkg.in/amz.v3/aws"
	"gopkg.in/amz.v3/s3"
	"gopkg.in/amz.v3/s3/s3test"

	bes3 "github.com/restic/restic/backend/s3"
	. "github.com/restic/restic/test"
)

type LocalServer struct {
	auth   aws.Auth
	region aws.Region
	srv    *s3test.Server
	config *s3test.Config
}

var s LocalServer

func setupS3Backend(t *testing.T) *bes3.S3Backend {
	s.config = &s3test.Config{
		Send409Conflict: true,
	}
	srv, err := s3test.NewServer(s.config)
	OK(t, err)
	s.srv = srv

	s.region = aws.Region{
		Name:                 "faux-region-1",
		S3Endpoint:           srv.URL(),
		S3LocationConstraint: true, // s3test server requires a LocationConstraint
	}

	s.auth = aws.Auth{"abc", "123"}

	service := s3.New(s.auth, s.region)
	bucket, berr := service.Bucket("testbucket")
	OK(t, err)
	err = bucket.PutBucket("private")
	OK(t, err)

	t.Logf("created s3 backend locally")

	return bes3.OpenS3Bucket(bucket, "testbucket")
}

func TestS3Backend(t *testing.T) {
	s := setupS3Backend(t)

	testBackend(s, t)
}
