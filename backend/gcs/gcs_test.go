package gcs_test

import (
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/backend/gcs"
	"github.com/restic/restic/backend/test"
	. "github.com/restic/restic/test"
)

//go:generate go run ../test/generate_backend_tests.go

func init() {
	if TestS3Server == "" {
		SkipMessage = "s3 test server not available"
		return
	}

	url, err := url.Parse(TestS3Server)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid url: %v\n", err)
		return
	}

	cfg := gcs.Config{
		Endpoint: url.Host,
		Bucket:   "restictestbucket",
		KeyID:    os.Getenv("S3_ACCESS_KEY_ID"),
		Secret:   os.Getenv("S3_SECRET_ACCESS_KEY"),
	}

	if url.Scheme == "http" {
		cfg.UseHTTP = true
	}

	test.CreateFn = func() (backend.Backend, error) {
		be, err := gcs.Open(cfg)
		if err != nil {
			return nil, err
		}

		exists, err := be.Test(backend.Config, "")
		if err != nil {
			return nil, err
		}

		if exists {
			return nil, errors.New("config already exists")
		}

		return be, nil
	}

	test.OpenFn = func() (backend.Backend, error) {
		return gcs.Open(cfg)
	}

	// test.CleanupFn = func() error {
	// 	if tempBackendDir == "" {
	// 		return nil
	// 	}

	// 	fmt.Printf("removing test backend at %v\n", tempBackendDir)
	// 	err := os.RemoveAll(tempBackendDir)
	// 	tempBackendDir = ""
	// 	return err
	// }
}
