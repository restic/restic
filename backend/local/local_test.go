package local_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/backend/local"
	"github.com/restic/restic/backend/test"
)

var tempBackendDir string

//go:generate go run ../test/generate_backend_tests.go

func init() {
	test.CreateFn = func() (backend.Backend, error) {
		if tempBackendDir != "" {
			return nil, errors.New("temporary local backend dir already exists")
		}

		tempdir, err := ioutil.TempDir("", "restic-local-test-")
		if err != nil {
			return nil, err
		}

		fmt.Printf("created new test backend at %v\n", tempdir)
		tempBackendDir = tempdir

		return local.Create(tempdir)
	}

	test.OpenFn = func() (backend.Backend, error) {
		if tempBackendDir == "" {
			return nil, errors.New("repository not initialized")
		}

		return local.Open(tempBackendDir)
	}

	test.CleanupFn = func() error {
		if tempBackendDir == "" {
			return nil
		}

		fmt.Printf("removing test backend at %v\n", tempBackendDir)
		err := os.RemoveAll(tempBackendDir)
		tempBackendDir = ""
		return err
	}
}
