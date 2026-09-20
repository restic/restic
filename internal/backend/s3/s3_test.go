package s3_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/backend/s3"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/options"
	rtest "github.com/restic/restic/internal/test"
)

const seaweedFSS3Addr = "127.0.0.1:8333"

func mkdir(t testing.TB, dir string) {
	err := os.MkdirAll(dir, 0700)
	if err != nil {
		t.Fatal(err)
	}
}

func runSeaweedFS(ctx context.Context, t testing.TB, dir, key, secret string) func() {
	mkdir(t, dir)

	cmd := exec.CommandContext(ctx, "weed",
		"mini",
		"-dir", dir,
		"-ip", "127.0.0.1",
		"-ip.bind", "127.0.0.1",
		"-s3.port", "8333",
		"-s3.port.iceberg", "0",
		"-s3.port.lance", "0",
		"-webdav=false",
		"-admin.ui=false",
		"-master.telemetry=false",
	)
	cmd.Env = append(os.Environ(),
		"AWS_ACCESS_KEY_ID="+key,
		"AWS_SECRET_ACCESS_KEY="+secret,
	)
	cmd.Stderr = os.Stderr

	err := cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	// wait until the S3 TCP port is reachable
	var success bool
	for range 100 {
		time.Sleep(200 * time.Millisecond)

		c, err := net.Dial("tcp", seaweedFSS3Addr)
		if err == nil {
			success = true
			if err := c.Close(); err != nil {
				t.Fatal(err)
			}
			break
		}
	}

	if !success {
		t.Fatal("unable to connect to seaweedfs s3 endpoint")
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

func newSeaweedFSTestSuite(t testing.TB) (*test.Suite[s3.Config], func()) {
	ctx, cancel := context.WithCancel(context.Background())

	tempdir := rtest.TempDir(t)
	key, secret := newRandomCredentials(t)
	cleanup := runSeaweedFS(ctx, t, tempdir, key, secret)

	return &test.Suite[s3.Config]{
			// NewConfig returns a config for a new temporary backend that will be used in tests.
			NewConfig: func() (*s3.Config, error) {
				cfg := s3.NewConfig()
				cfg.Endpoint = seaweedFSS3Addr
				cfg.Bucket = "restictestbucket"
				cfg.Prefix = fmt.Sprintf("test-%d", time.Now().UnixNano())
				cfg.UseHTTP = true
				cfg.Region = "us-east-1"
				cfg.BucketLookup = "path"
				cfg.KeyID = key
				cfg.Secret = options.NewSecretString(secret)
				return &cfg, nil
			},

			Factory: location.NewHTTPBackendFactory("s3", s3.ParseConfig, location.NoPassword, func(ctx context.Context, cfg s3.Config, rt http.RoundTripper, errorLog func(string, ...any)) (be backend.Backend, err error) {
				for i := range 50 {
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
			defer cleanup()
		}
}

func TestBackendSeaweedFS(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/s3.TestBackendSeaweedFS")
		}
	}()

	_, err := exec.LookPath("weed")
	if err != nil {
		t.Skip(err)
		return
	}

	suite, cleanup := newSeaweedFSTestSuite(t)
	defer cleanup()

	suite.RunTests(t)
}

func BenchmarkBackendSeaweedFS(t *testing.B) {
	_, err := exec.LookPath("weed")
	if err != nil {
		t.Skip(err)
		return
	}

	suite, cleanup := newSeaweedFSTestSuite(t)
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
