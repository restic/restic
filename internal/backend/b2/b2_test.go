package b2_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend/b2"
	"github.com/restic/restic/internal/backend/test"

	rtest "github.com/restic/restic/internal/test"
)

func newB2TestSuite() *test.Suite[b2.Config] {
	return &test.Suite[b2.Config]{
		// do not use excessive data
		MinimalData: true,

		// wait for at most 10 seconds for removed files to disappear
		WaitForDelayedRemoval: 10 * time.Second,

		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (*b2.Config, error) {
			cfg, err := b2.ParseConfig(os.Getenv("RESTIC_TEST_B2_REPOSITORY"))
			if err != nil {
				return nil, err
			}

			cfg.ApplyEnvironment("RESTIC_TEST_")
			cfg.Prefix = fmt.Sprintf("test-%d", time.Now().UnixNano())
			return cfg, nil
		},

		Factory: b2.NewFactory(),
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
	newB2TestSuite().RunTests(t)
}

func BenchmarkBackendb2(t *testing.B) {
	testVars(t)
	newB2TestSuite().RunBenchmarks(t)
}
