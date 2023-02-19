package mem_test

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/backend/test"
)

type memConfig struct {
	be restic.Backend
}

func newTestSuite() *test.Suite {
	return &test.Suite{
		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			return &memConfig{}, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(cfg interface{}) (restic.Backend, error) {
			c := cfg.(*memConfig)
			if c.be != nil {
				_, err := c.be.Stat(context.TODO(), restic.Handle{Type: restic.ConfigFile})
				if err != nil && !c.be.IsNotExist(err) {
					return nil, err
				}

				if err == nil {
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
}

func TestSuiteBackendMem(t *testing.T) {
	newTestSuite().RunTests(t)
}

func BenchmarkSuiteBackendMem(t *testing.B) {
	newTestSuite().RunBenchmarks(t)
}
