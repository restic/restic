package swift_test

import (
	"fmt"
	"os"
	"restic"
	"testing"
	"time"

	"restic/errors"
	. "restic/test"

	"restic/backend/swift"
	"restic/backend/test"
)

func newSwiftTestSuite(t testing.TB) *test.Suite {
	return &test.Suite{
		// do not use excessive data
		MinimalData: true,

		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			swiftcfg, err := swift.ParseConfig(os.Getenv("RESTIC_TEST_SWIFT"))
			if err != nil {
				return nil, err
			}

			cfg := swiftcfg.(swift.Config)
			if err = swift.ApplyEnvironment("RESTIC_TEST_", &cfg); err != nil {
				return nil, err
			}
			cfg.Prefix += fmt.Sprintf("/test-%d", time.Now().UnixNano())
			t.Logf("using prefix %v", cfg.Prefix)
			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			cfg := config.(swift.Config)

			be, err := swift.Open(cfg)
			if err != nil {
				return nil, err
			}

			exists, err := be.Test(restic.Handle{Type: restic.ConfigFile})
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
			cfg := config.(swift.Config)
			return swift.Open(cfg)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			cfg := config.(swift.Config)

			be, err := swift.Open(cfg)
			if err != nil {
				return err
			}

			if err := be.(restic.Deleter).Delete(); err != nil {
				return err
			}

			return nil
		},
	}
}

func TestBackendSwift(t *testing.T) {
	defer func() {
		if t.Skipped() {
			SkipDisallowed(t, "restic/backend/swift.TestBackendSwift")
		}
	}()

	if os.Getenv("RESTIC_TEST_SWIFT") == "" {
		t.Skip("RESTIC_TEST_SWIFT unset, skipping test")
		return
	}

	t.Logf("run tests")
	newSwiftTestSuite(t).RunTests(t)
}

func BenchmarkBackendSwift(t *testing.B) {
	if os.Getenv("RESTIC_TEST_SWIFT") == "" {
		t.Skip("RESTIC_TEST_SWIFT unset, skipping test")
		return
	}

	t.Logf("run tests")
	newSwiftTestSuite(t).RunBenchmarks(t)
}
