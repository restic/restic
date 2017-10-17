package oss_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend/oss"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/restic"

	rtest "github.com/restic/restic/internal/test"
)

func newOSSTestSuite(t testing.TB) *test.Suite {
	return &test.Suite{
		// do not use excessive data
		MinimalData: true,

		// wait for at most 10 seconds for removed files to disappear
		WaitForDelayedRemoval: 10 * time.Second,

		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			osscfg, err := oss.ParseConfig(os.Getenv("RESTIC_TEST_OSS_REPOSITORY"))
			if err != nil {
				return nil, err
			}

			cfg := osscfg.(oss.Config)
			cfg.AccessID = os.Getenv("RESTIC_TEST_OS_ACCESS_ID")
			cfg.AccessKey = os.Getenv("RESTIC_TEST_OSS_ACCESS_KEY")
			cfg.Prefix = fmt.Sprintf("test-%d", time.Now().UnixNano())
			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			cfg := config.(oss.Config)
			return oss.Create(cfg)
		},

		// OpenFn is a function that opens a previously created temporary repository.
		Open: func(config interface{}) (restic.Backend, error) {
			cfg := config.(oss.Config)
			return oss.Open(cfg)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			cfg := config.(oss.Config)
			be, err := oss.Open(cfg)
			if err != nil {
				return err
			}

			return be.Delete(context.TODO())
		},
	}
}

func testVars(t testing.TB) {
	vars := []string{
		"RESTIC_TEST_OSS_REPOSITORY",
		"RESTIC_TEST_OS_ACCESS_ID",
		"RESTIC_TEST_OSS_ACCESS_KEY",
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
	newOSSTestSuite(t).RunTests(t)
}

func BenchmarkBackendb2(t *testing.B) {
	testVars(t)
	newOSSTestSuite(t).RunBenchmarks(t)
}
