package b2_test

import (
	"os"
	"restic"

	"restic/errors"

	"restic/backend/b2"
	"restic/backend/test"
)

//go:generate go run ../test/generate_backend_tests.go

func init() {
	if os.Getenv("B2_ACCOUNT_ID") == "" || os.Getenv("B2_ACCOUNT_KEY") == "" {
		SkipMessage = "No B2 credentials found. Skipping B2 backend tests."
		return
	}

	cfg := b2.Config{
		AccountID: os.Getenv("B2_ACCOUNT_ID"),
		Key:       os.Getenv("B2_ACCOUNT_KEY"),
		Bucket:    "restic-test",
		Prefix:    "test",
	}

	test.CreateFn = func() (restic.Backend, error) {
		be, err := b2.Open(cfg)
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
		return b2.Open(cfg)
	}

	// test.CleanupFn = func() error {
	// 	if tempBackendDir == "" {
	// 		return nil
	// 	}
	//
	// 	fmt.Printf("removing test backend at %v\n", tempBackendDir)
	// 	err := os.RemoveAll(tempBackendDir)
	// 	tempBackendDir = ""
	// 	return err
	// }
}
