package s3_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/s3"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/options"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func mkdir(t testing.TB, dir string) {
	err := os.MkdirAll(dir, 0700)
	if err != nil {
		t.Fatal(err)
	}
}

func runMinio(ctx context.Context, t testing.TB, dir, key, secret string) func() {
	mkdir(t, filepath.Join(dir, "config"))
	mkdir(t, filepath.Join(dir, "root"))

	cmd := exec.CommandContext(ctx, "minio",
		"server",
		"--address", "127.0.0.1:9000",
		"--config-dir", filepath.Join(dir, "config"),
		filepath.Join(dir, "root"))
	cmd.Env = append(os.Environ(),
		"MINIO_ACCESS_KEY="+key,
		"MINIO_SECRET_KEY="+secret,
	)
	cmd.Stderr = os.Stderr

	err := cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	// wait until the TCP port is reachable
	var success bool
	for i := 0; i < 100; i++ {
		time.Sleep(200 * time.Millisecond)

		c, err := net.Dial("tcp", "localhost:9000")
		if err == nil {
			success = true
			if err := c.Close(); err != nil {
				t.Fatal(err)
			}
			break
		}
	}

	if !success {
		t.Fatal("unable to connect to minio server")
		return nil
	}

	return func() {
		err = cmd.Process.Kill()
		if err != nil {
			t.Fatal(err)
		}

		// ignore errors, we've killed the process
		_ = cmd.Wait()
	}
}

func newRandomCredentials(t testing.TB) (key, secret string) {
	buf := make([]byte, 10)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		t.Fatal(err)
	}
	key = hex.EncodeToString(buf)

	_, err = io.ReadFull(rand.Reader, buf)
	if err != nil {
		t.Fatal(err)
	}
	secret = hex.EncodeToString(buf)

	return key, secret
}

type MinioTestConfig struct {
	s3.Config

	tempdir    string
	stopServer func()
}

func createS3(t testing.TB, cfg MinioTestConfig, tr http.RoundTripper) (be restic.Backend, err error) {
	for i := 0; i < 10; i++ {
		be, err = s3.Create(context.TODO(), cfg.Config, tr)
		if err != nil {
			t.Logf("s3 open: try %d: error %v", i, err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		break
	}

	return be, err
}

func newMinioTestSuite(ctx context.Context, t testing.TB) *test.Suite {
	tr, err := backend.Transport(backend.TransportOptions{})
	if err != nil {
		t.Fatalf("cannot create transport for tests: %v", err)
	}

	return &test.Suite{
		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			cfg := MinioTestConfig{}

			cfg.tempdir = rtest.TempDir(t)
			key, secret := newRandomCredentials(t)
			cfg.stopServer = runMinio(ctx, t, cfg.tempdir, key, secret)

			cfg.Config = s3.NewConfig()
			cfg.Config.Endpoint = "localhost:9000"
			cfg.Config.Bucket = "restictestbucket"
			cfg.Config.Prefix = fmt.Sprintf("test-%d", time.Now().UnixNano())
			cfg.Config.UseHTTP = true
			cfg.Config.KeyID = key
			cfg.Config.Secret = options.NewSecretString(secret)
			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			cfg := config.(MinioTestConfig)

			be, err := createS3(t, cfg, tr)
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
			cfg := config.(MinioTestConfig)
			return s3.Open(ctx, cfg.Config, tr)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			cfg := config.(MinioTestConfig)
			if cfg.stopServer != nil {
				cfg.stopServer()
			}
			return nil
		},
	}
}

func TestBackendMinio(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/s3.TestBackendMinio")
		}
	}()

	// try to find a minio binary
	_, err := exec.LookPath("minio")
	if err != nil {
		t.Skip(err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	newMinioTestSuite(ctx, t).RunTests(t)
}

func BenchmarkBackendMinio(t *testing.B) {
	// try to find a minio binary
	_, err := exec.LookPath("minio")
	if err != nil {
		t.Skip(err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	newMinioTestSuite(ctx, t).RunBenchmarks(t)
}

func newS3TestSuite(t testing.TB) *test.Suite {
	tr, err := backend.Transport(backend.TransportOptions{})
	if err != nil {
		t.Fatalf("cannot create transport for tests: %v", err)
	}

	return &test.Suite{
		// do not use excessive data
		MinimalData: true,

		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			s3cfg, err := s3.ParseConfig(os.Getenv("RESTIC_TEST_S3_REPOSITORY"))
			if err != nil {
				return nil, err
			}

			cfg := s3cfg.(s3.Config)
			cfg.KeyID = os.Getenv("RESTIC_TEST_S3_KEY")
			cfg.Secret = options.NewSecretString(os.Getenv("RESTIC_TEST_S3_SECRET"))
			cfg.Prefix = fmt.Sprintf("test-%d", time.Now().UnixNano())
			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			cfg := config.(s3.Config)

			be, err := s3.Create(context.TODO(), cfg, tr)
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
			cfg := config.(s3.Config)
			return s3.Open(context.TODO(), cfg, tr)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			cfg := config.(s3.Config)

			be, err := s3.Open(context.TODO(), cfg, tr)
			if err != nil {
				return err
			}

			return be.Delete(context.TODO())
		},
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
	newS3TestSuite(t).RunTests(t)
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
	newS3TestSuite(t).RunBenchmarks(t)
}
