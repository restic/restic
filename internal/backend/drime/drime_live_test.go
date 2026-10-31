package drime_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend/drime"
	"github.com/restic/restic/internal/backend/test"

	rtest "github.com/restic/restic/internal/test"
)

func newDrimeTestSuite() *test.Suite[drime.Config] {
	return &test.Suite[drime.Config]{
		// do not use excessive data
		MinimalData: true,

		// wait for at most 10 seconds for removed files to disappear
		WaitForDelayedRemoval: 10 * time.Second,

		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (*drime.Config, error) {
			cfg, err := drime.ParseConfig(os.Getenv("RESTIC_TEST_DRIME_REPOSITORY"))
			if err != nil {
				return nil, err
			}

			cfg.ApplyEnvironment("RESTIC_TEST_")
			cfg.Path = fmt.Sprintf("%s/test-%d", cfg.Path, time.Now().UnixNano())
			return cfg, nil
		},

		Factory: drime.NewFactory(),
	}
}

func testVars(t testing.TB) {
	vars := []string{
		"RESTIC_TEST_DRIME_ACCESS_TOKEN",
		"RESTIC_TEST_DRIME_REPOSITORY",
	}

	for _, v := range vars {
		if os.Getenv(v) == "" {
			t.Skipf("environment variable %v not set", v)
			return
		}
	}
}

func TestBackendDrime(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/drime.TestBackendDrime")
		}
	}()

	testVars(t)
	newDrimeTestSuite().RunTests(t)
}

func BenchmarkBackendDrime(t *testing.B) {
	testVars(t)
	newDrimeTestSuite().RunBenchmarks(t)
}
