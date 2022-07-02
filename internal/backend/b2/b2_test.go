package b2_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/b2"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/options"
	"github.com/restic/restic/internal/restic"

	rtest "github.com/restic/restic/internal/test"
)

func newB2TestSuite(t testing.TB) *test.Suite {
	tr, err := backend.Transport(backend.TransportOptions{})
	if err != nil {
		t.Fatalf("cannot create transport for tests: %v", err)
	}

	return &test.Suite{
		// do not use excessive data
		MinimalData: true,

		// wait for at most 10 seconds for removed files to disappear
		WaitForDelayedRemoval: 10 * time.Second,

		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			b2cfg, err := b2.ParseConfig(os.Getenv("RESTIC_TEST_B2_REPOSITORY"))
			if err != nil {
				return nil, err
			}

			cfg := b2cfg.(b2.Config)
			cfg.AccountID = os.Getenv("RESTIC_TEST_B2_ACCOUNT_ID")
			cfg.Key = options.NewSecretString(os.Getenv("RESTIC_TEST_B2_ACCOUNT_KEY"))
			cfg.Prefix = fmt.Sprintf("test-%d", time.Now().UnixNano())
			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			cfg := config.(b2.Config)
			return b2.Create(context.Background(), cfg, tr)
		},

		// OpenFn is a function that opens a previously created temporary repository.
		Open: func(config interface{}) (restic.Backend, error) {
			cfg := config.(b2.Config)
			return b2.Open(context.Background(), cfg, tr)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			cfg := config.(b2.Config)
			be, err := b2.Open(context.Background(), cfg, tr)
			if err != nil {
				return err
			}

			return be.Delete(context.TODO())
		},
	}
}

func testVars(t testing.TB) {
	vars := []string{
		"RESTIC_TEST_B2_ACCOUNT_ID",
		"RESTIC_TEST_B2_ACCOUNT_KEY",
		"RESTIC_TEST_B2_REPOSITORY",
	}

	for _, v := range vars {
		if os.Getenv(v) == "" {
			t.Skipf("environment variable %v not set", v)
			return
		}
	}
}

func TestBackendB2(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/b2.TestBackendB2")
		}
	}()

	testVars(t)
	newB2TestSuite(t).RunTests(t)
}

func BenchmarkBackendb2(t *testing.B) {
	testVars(t)
	newB2TestSuite(t).RunBenchmarks(t)
}
