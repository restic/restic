package gs_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/gs"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func newGSTestSuite(t testing.TB) *test.Suite {
	tr, err := backend.Transport(backend.TransportOptions{})
	if err != nil {
		t.Fatalf("cannot create transport for tests: %v", err)
	}

	return &test.Suite{
		// do not use excessive data
		MinimalData: true,

		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			gscfg, err := gs.ParseConfig(os.Getenv("RESTIC_TEST_GS_REPOSITORY"))
			if err != nil {
				return nil, err
			}

			cfg := gscfg.(gs.Config)
			cfg.ProjectID = os.Getenv("RESTIC_TEST_GS_PROJECT_ID")
			cfg.Prefix = fmt.Sprintf("test-%d", time.Now().UnixNano())
			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			cfg := config.(gs.Config)

			be, err := gs.Create(cfg, tr)
			if err != nil {
				return nil, err
			}

			_, err = be.Stat(context.TODO(), restic.Handle{Type: restic.ConfigFile})
			if err != nil && !be.IsNotExist(err) {
				return nil, err
			}

			if err == nil {
				return nil, errors.New("config already exists")
			}

			return be, nil
		},

		// OpenFn is a function that opens a previously created temporary repository.
		Open: func(config interface{}) (restic.Backend, error) {
			cfg := config.(gs.Config)
			return gs.Open(cfg, tr)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			cfg := config.(gs.Config)

			be, err := gs.Open(cfg, tr)
			if err != nil {
				return err
			}

			return be.Delete(context.TODO())
		},
	}
}

func TestBackendGS(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/gs.TestBackendGS")
		}
	}()

	vars := []string{
		"RESTIC_TEST_GS_PROJECT_ID",
		"RESTIC_TEST_GS_REPOSITORY",
	}

	for _, v := range vars {
		if os.Getenv(v) == "" {
			t.Skipf("environment variable %v not set", v)
			return
		}
	}
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")+os.Getenv("GOOGLE_ACCESS_TOKEN") == "" {
		t.Skipf("environment variable GOOGLE_APPLICATION_CREDENTIALS not set, nor GOOGLE_ACCESS_TOKEN")
		return
	}

	t.Logf("run tests")
	newGSTestSuite(t).RunTests(t)
}

func BenchmarkBackendGS(t *testing.B) {
	vars := []string{
		"RESTIC_TEST_GS_PROJECT_ID",
		"RESTIC_TEST_GS_REPOSITORY",
	}

	for _, v := range vars {
		if os.Getenv(v) == "" {
			t.Skipf("environment variable %v not set", v)
			return
		}
	}
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")+os.Getenv("GOOGLE_ACCESS_TOKEN") == "" {
		t.Skipf("environment variable GOOGLE_APPLICATION_CREDENTIALS not set, nor GOOGLE_ACCESS_TOKEN")
		return
	}

	t.Logf("run tests")
	newGSTestSuite(t).RunBenchmarks(t)
}
