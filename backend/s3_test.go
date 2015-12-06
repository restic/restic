package backend_test

import (
	"os"
	"testing"

	"github.com/minio/minio-go"
	bes3 "github.com/restic/restic/backend/s3"
	. "github.com/restic/restic/test"
)

func TestS3Backend(t *testing.T) {
	config := minio.Config{
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		Endpoint:        "http://localhost:9000",
	}
	s3Client, err := minio.New(config)
	if err != nil {
		t.Fatal(err)
	}

	bucketname := "restictestbucket"

	err = s3Client.MakeBucket(bucketname, "")
	if err != nil {
		t.Fatal(err)
	}

	s, err := bes3.Open("127.0.0.1:9000", bucketname)
	OK(t, err)

	testBackend(s, t)
}
