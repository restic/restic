package gdrive_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend/gdrive"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func newGDriveTestSuite(t testing.TB) *test.Suite {
	return &test.Suite{
		// do not use excessive data
		MinimalData: true,

		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			prefix := os.Getenv("RESTIC_TEST_GDRIVE_PREFIX")
			jsonKeyPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")

			cfg := gdrive.NewConfig()
			cfg.JSONKeyPath = jsonKeyPath
			cfg.Prefix = fmt.Sprintf("%s/test-%d", prefix, time.Now().UnixNano())
			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			cfg := config.(gdrive.Config)
			return gdrive.Create(context.TODO(), cfg, http.DefaultTransport)
		},

		// OpenFn is a function that opens a previously created temporary repository.
		Open: func(config interface{}) (restic.Backend, error) {
			cfg := config.(gdrive.Config)
			return gdrive.Open(context.TODO(), cfg, http.DefaultTransport)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			cfg := config.(gdrive.Config)

			be, err := gdrive.Open(context.TODO(), cfg, http.DefaultTransport)
			if err != nil {
				if be.IsNotExist(err) {
					return nil
				}
				return err
			}

			return be.Delete(context.TODO())
		},
	}
}

func assertEnvironment(t testing.TB) {
	vars := []string{
		"RESTIC_TEST_GDRIVE_PREFIX",
		"GOOGLE_APPLICATION_CREDENTIALS",
	}

	for _, v := range vars {
		if os.Getenv(v) == "" {
			t.Skipf("environment variable %v not set", v)
			return
		}
	}
}

func TestBackendGDrive(t *testing.T) {
	defer func() {
		if t.Skipped() {
			// see RESTIC_TEST_GDRIVE_PREFIX check in run_integration_tests.go
			rtest.SkipDisallowed(t, "restic/backend/gdrive.TestBackendGDrive")
		}
	}()

	assertEnvironment(t)

	t.Logf("run tests")
	newGDriveTestSuite(t).RunTests(t)
}

func BenchmarkGDrive(t *testing.B) {
	assertEnvironment(t)

	t.Logf("run tests")
	newGDriveTestSuite(t).RunBenchmarks(t)
}
