package azure_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/azure"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func newAzureTestSuite(t testing.TB) *test.Suite {
	tr, err := backend.Transport(nil)
	if err != nil {
		t.Fatalf("cannot create transport for tests: %v", err)
	}

	return &test.Suite{
		// do not use excessive data
		MinimalData: true,

		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			azcfg, err := azure.ParseConfig(os.Getenv("RESTIC_TEST_AZURE_REPOSITORY"))
			if err != nil {
				return nil, err
			}

			cfg := azcfg.(azure.Config)
			cfg.AccountName = os.Getenv("RESTIC_TEST_AZURE_ACCOUNT_NAME")
			cfg.AccountKey = os.Getenv("RESTIC_TEST_AZURE_ACCOUNT_KEY")
			cfg.Prefix = fmt.Sprintf("test-%d", time.Now().UnixNano())
			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			cfg := config.(azure.Config)

			be, err := azure.Create(cfg, tr)
			if err != nil {
				return nil, err
			}

			exists, err := be.Test(context.TODO(), restic.Handle{Type: restic.ConfigFile})
			if err != nil {
				return nil, err
			}

			if exists {
				return nil, errors.New("config already exists")
			}

			return be, nil
		},

		// OpenFn is a function that opens a previously created temporary repository.
		Open: func(config interface{}) (restic.Backend, error) {
			cfg := config.(azure.Config)

			return azure.Open(cfg, tr)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			cfg := config.(azure.Config)

			be, err := azure.Open(cfg, tr)
			if err != nil {
				return err
			}

			return be.Delete(context.TODO())
		},
	}
}

func TestBackendAzure(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/azure.TestBackendAzure")
		}
	}()

	vars := []string{
		"RESTIC_TEST_AZURE_ACCOUNT_NAME",
		"RESTIC_TEST_AZURE_ACCOUNT_KEY",
		"RESTIC_TEST_AZURE_REPOSITORY",
	}

	for _, v := range vars {
		if os.Getenv(v) == "" {
			t.Skipf("environment variable %v not set", v)
			return
		}
	}

	t.Logf("run tests")
	newAzureTestSuite(t).RunTests(t)
}

func BenchmarkBackendAzure(t *testing.B) {
	vars := []string{
		"RESTIC_TEST_AZURE_ACCOUNT_NAME",
		"RESTIC_TEST_AZURE_ACCOUNT_KEY",
		"RESTIC_TEST_AZURE_REPOSITORY",
	}

	for _, v := range vars {
		if os.Getenv(v) == "" {
			t.Skipf("environment variable %v not set", v)
			return
		}
	}

	t.Logf("run tests")
	newAzureTestSuite(t).RunBenchmarks(t)
}
