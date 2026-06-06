package s3_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/backend/s3"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/options"
	rtest "github.com/restic/restic/internal/test"
	"github.com/testcontainers/testcontainers-go"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
)

func runMinio(ctx context.Context, t testing.TB) (*s3.Config, func()) {
	container, err := tcminio.Run(ctx, "minio/minio:RELEASE.2025-09-07T16-13-09Z")
	if err != nil || container == nil {
		t.Fatal(err)
	}
	endpoint, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}

	cfg := s3.NewConfig()
	cfg.Endpoint = endpoint
	cfg.Bucket = "restictestbucket"
	cfg.Prefix = fmt.Sprintf("test-%d", time.Now().UnixNano())
	cfg.UseHTTP = true
	cfg.KeyID = container.Username
	cfg.Secret = options.NewSecretString(container.Password)

	return &cfg, func() {
		if err := container.Terminate(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func newMinioTestSuite(t testing.TB) (*test.Suite[s3.Config], func()) {
	ctx, cancel := context.WithCancel(context.Background())

	cfg, cleanupContainer := runMinio(ctx, t)

	return &test.Suite[s3.Config]{
			NewConfig: func() (*s3.Config, error) {
				return cfg, nil
			},

			Factory: location.NewHTTPBackendFactory("s3", s3.ParseConfig, location.NoPassword, func(ctx context.Context, cfg s3.Config, rt http.RoundTripper, errorLog func(string, ...interface{})) (be backend.Backend, err error) {
				for i := 0; i < 50; i++ {
					be, err = s3.Create(ctx, cfg, rt, errorLog)
					if err != nil {
						t.Logf("s3 open: try %d: error %v", i, err)
						time.Sleep(500 * time.Millisecond)
						continue
					}
					break
				}
				return be, err
			}, s3.Open),
		}, func() {
			defer cancel()
			defer cleanupContainer()
		}
}

// copy from testcontainer.SkipIfProviderIsNotHealthy
// just replace type for more unify for test and benchmark
func SkipIfProviderIsNotHealthy(t testing.TB) (skip bool) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("Recovered from panic: %v. Docker is not running. Testcontainers can't perform is work without it", r)
			skip = true
		}
	}()

	ctx := context.Background()
	provider, err := testcontainers.ProviderDocker.GetProvider()
	if err != nil {
		t.Skipf("Docker is not running. Testcontainers can't perform is work without it: %s", err)
		return true
	}
	err = provider.Health(ctx)
	if err != nil {
		t.Skipf("Docker is not running. Testcontainers can't perform is work without it: %s", err)
		return true
	}
	return false
}

func TestBackendMinio(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/s3.TestBackendMinio")
		}
	}()

	if SkipIfProviderIsNotHealthy(t) {
		return
	}

	suite, cleanup := newMinioTestSuite(t)
	defer cleanup()

	suite.RunTests(t)
}

func BenchmarkBackendMinio(t *testing.B) {
	if SkipIfProviderIsNotHealthy(t) {
		return
	}

	suite, cleanup := newMinioTestSuite(t)
	defer cleanup()

	suite.RunBenchmarks(t)
}

func newS3TestSuite() *test.Suite[s3.Config] {
	return &test.Suite[s3.Config]{
		// do not use excessive data
		MinimalData: true,

		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (*s3.Config, error) {
			cfg, err := s3.ParseConfig(os.Getenv("RESTIC_TEST_S3_REPOSITORY"))
			if err != nil {
				return nil, err
			}

			cfg.KeyID = os.Getenv("RESTIC_TEST_S3_KEY")
			cfg.Secret = options.NewSecretString(os.Getenv("RESTIC_TEST_S3_SECRET"))
			cfg.Prefix = fmt.Sprintf("test-%d", time.Now().UnixNano())
			return cfg, nil
		},

		Factory: s3.NewFactory(),
	}
}

func TestBackendS3(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/s3.TestBackendS3")
		}
	}()

	vars := []string{
		"RESTIC_TEST_S3_KEY",
		"RESTIC_TEST_S3_SECRET",
		"RESTIC_TEST_S3_REPOSITORY",
	}

	for _, v := range vars {
		if os.Getenv(v) == "" {
			t.Skipf("environment variable %v not set", v)
			return
		}
	}

	t.Logf("run tests")
	newS3TestSuite().RunTests(t)
}

func BenchmarkBackendS3(t *testing.B) {
	vars := []string{
		"RESTIC_TEST_S3_KEY",
		"RESTIC_TEST_S3_SECRET",
		"RESTIC_TEST_S3_REPOSITORY",
	}

	for _, v := range vars {
		if os.Getenv(v) == "" {
			t.Skipf("environment variable %v not set", v)
			return
		}
	}

	t.Logf("run tests")
	newS3TestSuite().RunBenchmarks(t)
}
