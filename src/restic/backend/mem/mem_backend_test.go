package mem_test

import (
	"restic"
	"testing"

	"restic/errors"

	"restic/backend/mem"
	"restic/backend/test"
)

type memConfig struct {
	be restic.Backend
}

func TestSuiteBackendMem(t *testing.T) {
	suite := test.Suite{
		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			return &memConfig{}, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(cfg interface{}) (restic.Backend, error) {
			c := cfg.(*memConfig)
			if c.be != nil {
				ok, err := c.be.Test(restic.Handle{Type: restic.ConfigFile})
				if err != nil {
					return nil, err
				}

				if ok {
					return nil, errors.New("config already exists")
				}
			}

			c.be = mem.New()
			return c.be, nil
		},

		// OpenFn is a function that opens a previously created temporary repository.
		Open: func(cfg interface{}) (restic.Backend, error) {
			c := cfg.(*memConfig)
			if c.be == nil {
				c.be = mem.New()
			}
			return c.be, nil
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(cfg interface{}) error {
			// no cleanup needed
			return nil
		},
	}

	suite.RunTests(t)
}
