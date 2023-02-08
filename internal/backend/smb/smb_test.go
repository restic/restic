package smb_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/restic/restic/internal/backend/smb"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/options"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func newTestSuite(t testing.TB) *test.Suite {
	return &test.Suite{
		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {

			cfg := smb.NewConfig()
			cfg.Host = "127.0.0.1"
			cfg.User = "smbuser"
			cfg.ShareName = cfg.User
			cfg.Path = "Repo-" + uuid.New().String()
			cfg.Password = options.NewSecretString("mGoWwqvgdnwtmh07")
			cfg.Connections = smb.DefaultConnections
			timeout := smb.DefaultIdleTimeout
			cfg.IdleTimeout = timeout
			domain := os.Getenv("RESTIC_SMB_DOMAIN")
			if domain == "" {
				cfg.Domain = smb.DefaultDomain
			}

			t.Logf("create new backend at %v", cfg.Host+"/"+cfg.ShareName)

			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			cfg := config.(smb.Config)
			return smb.Create(context.TODO(), cfg)
		},

		// OpenFn is a function that opens a previously created temporary repository.
		Open: func(config interface{}) (restic.Backend, error) {
			cfg := config.(smb.Config)
			return smb.Open(context.TODO(), cfg)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			cfg := config.(smb.Config)
			if !rtest.TestCleanupTempDirs {
				t.Logf("leaving test backend dir at %v", cfg.Path)
			}

			rtest.RemoveAll(t, cfg.Path)
			return nil
		},
	}
}

func TestBackendSMB(t *testing.T) {
	if !rtest.RunSMBTest {
		t.Skip("Skipping smb tests")
	}
	t.Logf("run tests")

	newTestSuite(t).RunTests(t)
}

func BenchmarkBackendSMB(t *testing.B) {
	if !rtest.RunSMBTest {
		t.Skip("Skipping smb tests")
	}
	t.Logf("run benchmarks")

	newTestSuite(t).RunBenchmarks(t)
}
