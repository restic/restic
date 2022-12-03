package swift_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/swift"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func newSwiftTestSuite(t testing.TB) *test.Suite {
	tr, err := backend.Transport(backend.TransportOptions{})
	if err != nil {
		t.Fatalf("cannot create transport for tests: %v", err)
	}

	return &test.Suite{
		// do not use excessive data
		MinimalData: true,

		// wait for removals for at least 5m
		WaitForDelayedRemoval: 5 * time.Minute,

		ErrorHandler: func(t testing.TB, be restic.Backend, err error) error {
			if err == nil {
				return nil
			}

			if be.IsNotExist(err) {
				t.Logf("swift: ignoring error %v", err)
				return nil
			}

			return err
		},

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

			be, err := swift.Open(context.TODO(), cfg, tr)
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
			cfg := config.(swift.Config)
			return swift.Open(context.TODO(), cfg, tr)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			cfg := config.(swift.Config)

			be, err := swift.Open(context.TODO(), cfg, tr)
			if err != nil {
				return err
			}

			return be.Delete(context.TODO())
		},
	}
}

func TestBackendSwift(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/swift.TestBackendSwift")
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
