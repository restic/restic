package local_test

import (
	"fmt"
	"io/ioutil"
	"os"

	"restic/backend"
	"restic/backend/local"
	"restic/backend/test"
)

var tempBackendDir string

//go:generate go run ../test/generate_backend_tests.go

func createTempdir() error {
	if tempBackendDir != "" {
		return nil
	}

	tempdir, err := ioutil.TempDir("", "restic-local-test-")
	if err != nil {
		return err
	}

	fmt.Printf("created new test backend at %v\n", tempdir)
	tempBackendDir = tempdir
	return nil
}

func init() {
	test.CreateFn = func() (backend.Backend, error) {
		err := createTempdir()
		if err != nil {
			return nil, err
		}
		return local.Create(tempBackendDir)
	}

	test.OpenFn = func() (backend.Backend, error) {
		err := createTempdir()
		if err != nil {
			return nil, err
		}
		return local.Open(tempBackendDir)
	}

	test.CleanupFn = func() error {
		if tempBackendDir == "" {
			return nil
		}

		fmt.Printf("removing test backend at %v\n", tempBackendDir)
		err := patchedos.RemoveAll(tempBackendDir)
		tempBackendDir = ""
		return err
	}
}
