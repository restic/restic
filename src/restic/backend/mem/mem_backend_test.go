package mem_test

import (
	"restic"

	"restic/errors"

	"restic/backend/mem"
	"restic/backend/test"
)

var be restic.Backend

//go:generate go run ../test/generate_backend_tests.go

func init() {
	test.CreateFn = func() (restic.Backend, error) {
		if be != nil {
			return nil, errors.New("temporary memory backend dir already exists")
		}

		be = mem.New()

		return be, nil
	}

	test.OpenFn = func() (restic.Backend, error) {
		if be == nil {
			return nil, errors.New("repository not initialized")
		}

		return be, nil
	}

	test.CleanupFn = func() error {
		be = nil
		return nil
	}
}
