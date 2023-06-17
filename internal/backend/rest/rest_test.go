package rest_test

import (
	"context"
	"net"
	"net/url"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend/rest"
	"github.com/restic/restic/internal/backend/test"
	rtest "github.com/restic/restic/internal/test"
)

func runRESTServer(ctx context.Context, t testing.TB, dir string) (*url.URL, func()) {
	srv, err := exec.LookPath("rest-server")
	if err != nil {
		t.Skip(err)
	}

	cmd := exec.CommandContext(ctx, srv, "--no-auth", "--path", dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// wait until the TCP port is reachable
	var success bool
	for i := 0; i < 10; i++ {
		time.Sleep(200 * time.Millisecond)

		c, err := net.Dial("tcp", "localhost:8000")
		if err != nil {
			continue
		}

		success = true
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	}

	if !success {
		t.Fatal("unable to connect to rest server")
		return nil, nil
	}

	url, err := url.Parse("http://localhost:8000/restic-test/")
	if err != nil {
		t.Fatal(err)
	}

	cleanup := func() {
		if err := cmd.Process.Kill(); err != nil {
			t.Fatal(err)
		}

		// ignore errors, we've killed the process
		_ = cmd.Wait()
	}

	return url, cleanup
}

func newTestSuite(url *url.URL, minimalData bool) *test.Suite[rest.Config] {
	return &test.Suite[rest.Config]{
		MinimalData: minimalData,

		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (*rest.Config, error) {
			cfg := rest.NewConfig()
			cfg.URL = url
			return &cfg, nil
		},

		Factory: rest.NewFactory(),
	}
}

func TestBackendREST(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/rest.TestBackendREST")
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir := rtest.TempDir(t)
	serverURL, cleanup := runRESTServer(ctx, t, dir)
	defer cleanup()

	newTestSuite(serverURL, false).RunTests(t)
}

func TestBackendRESTExternalServer(t *testing.T) {
	repostr := os.Getenv("RESTIC_TEST_REST_REPOSITORY")
	if repostr == "" {
		t.Skipf("environment variable %v not set", "RESTIC_TEST_REST_REPOSITORY")
	}

	cfg, err := rest.ParseConfig(repostr)
	if err != nil {
		t.Fatal(err)
	}

	newTestSuite(cfg.URL, true).RunTests(t)
}

func BenchmarkBackendREST(t *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir := rtest.TempDir(t)
	serverURL, cleanup := runRESTServer(ctx, t, dir)
	defer cleanup()

	newTestSuite(serverURL, false).RunBenchmarks(t)
}
