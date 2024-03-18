package smb_test

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/restic/restic/internal/backend/smb"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/options"
	rtest "github.com/restic/restic/internal/test"
)

func newTestSuite(t testing.TB) *test.Suite[smb.Config] {
	return &test.Suite[smb.Config]{
		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (*smb.Config, error) {

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

			return &cfg, nil
		},

		Factory: smb.NewFactory(),
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
