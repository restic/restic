package rest_test

import (
	"context"
	"net"
	"net/url"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/rest"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/restic"
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

func newTestSuite(ctx context.Context, t testing.TB, url *url.URL, minimalData bool) *test.Suite {
	tr, err := backend.Transport(backend.TransportOptions{})
	if err != nil {
		t.Fatalf("cannot create transport for tests: %v", err)
	}

	return &test.Suite{
		MinimalData: minimalData,

		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			cfg := rest.NewConfig()
			cfg.URL = url
			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			cfg := config.(rest.Config)
			return rest.Create(context.TODO(), cfg, tr)
		},

		// OpenFn is a function that opens a previously created temporary repository.
		Open: func(config interface{}) (restic.Backend, error) {
			cfg := config.(rest.Config)
			return rest.Open(cfg, tr)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			return nil
		},
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

	newTestSuite(ctx, t, serverURL, false).RunTests(t)
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

	c := cfg.(rest.Config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	newTestSuite(ctx, t, c.URL, true).RunTests(t)
}

func BenchmarkBackendREST(t *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir := rtest.TempDir(t)
	serverURL, cleanup := runRESTServer(ctx, t, dir)
	defer cleanup()

	newTestSuite(ctx, t, serverURL, false).RunBenchmarks(t)
}
