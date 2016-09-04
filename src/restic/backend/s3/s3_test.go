package s3_test

import (
	"fmt"
	"net/url"
	"os"
	"restic"

	"restic/errors"

	"restic/backend/s3"
	"restic/backend/test"
	. "restic/test"
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

	cfg := s3.Config{
		Endpoint: url.Host,
		Bucket:   "restictestbucket",
		KeyID:    os.Getenv("AWS_ACCESS_KEY_ID"),
		Secret:   os.Getenv("AWS_SECRET_ACCESS_KEY"),
	}

	if url.Scheme == "http" {
		cfg.UseHTTP = true
	}

	test.CreateFn = func() (restic.Backend, error) {
		be, err := s3.Open(cfg)
		if err != nil {
			return nil, err
		}

		exists, err := be.Test(restic.ConfigFile, "")
		if err != nil {
			return nil, err
		}

		if exists {
			return nil, errors.New("config already exists")
		}

		return be, nil
	}

	test.OpenFn = func() (restic.Backend, error) {
		return s3.Open(cfg)
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
