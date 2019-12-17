package oss_test

import (
	"context"
	"fmt"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/oss"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"os"
	"testing"
	"time"
)

func newOSSTestSuite(t testing.TB) *test.Suite {
	tr, err := backend.Transport(backend.TransportOptions{})
	if err != nil {
		t.Fatalf("cannot create transport for tests: %v", err)
	}

	return &test.Suite{
		// do not use excessive data
		MinimalData: true,

		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			osscfg, err := oss.ParseConfig(os.Getenv("RESTIC_TEST_OSS_REPOSITORY"))
			if err != nil {
				return nil, err
			}

			cfg := osscfg.(oss.Config)
			cfg.AccessKeyID = os.Getenv("RESTIC_TEST_OSS_ACCESS_KEY_ID")
			cfg.AccessKeySecret = os.Getenv("RESTIC_TEST_OSS_ACCESS_KEY_SECRET")
			cfg.Prefix = fmt.Sprintf("test-%d", time.Now().UnixNano())
			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			cfg := config.(oss.Config)

			be, err := oss.Create(cfg, tr)
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
			cfg := config.(oss.Config)
			return oss.Open(cfg, tr)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			cfg := config.(oss.Config)

			be, err := oss.Open(cfg, tr)
			if err != nil {
				return err
			}

			return be.Delete(context.TODO())
		},
	}
}

func TestBackendOSS(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/oss.TestBackendOSS")
		}
	}()

	vars := []string{
		"RESTIC_TEST_OSS_ACCESS_KEY_ID",
		"RESTIC_TEST_OSS_ACCESS_KEY_SECRET",
		"RESTIC_TEST_OSS_REPOSITORY",
	}

	for _, v := range vars {
		if os.Getenv(v) == "" {
			t.Skipf("environment variable %v not set", v)
			return
		}
	}

	t.Logf("run tests")
	newOSSTestSuite(t).RunTests(t)
}

func BenchmarkBackendOSS(t *testing.B) {
	vars := []string{
		"RESTIC_TEST_OSS_ACCESS_KEY_ID",
		"RESTIC_TEST_OSS_ACCESS_KEY_SECRET",
		"RESTIC_TEST_OSS_REPOSITORY",
	}

	for _, v := range vars {
		if os.Getenv(v) == "" {
			t.Skipf("environment variable %v not set", v)
			return
		}
	}

	t.Logf("run tests")
	newOSSTestSuite(t).RunBenchmarks(t)
}
