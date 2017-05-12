package rest_test

import (
	"context"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"os/exec"
	"restic"
	"testing"
	"time"

	"restic/backend/rest"
	"restic/backend/test"
	. "restic/test"
)

func runRESTServer(ctx context.Context, t testing.TB, dir string) func() {
	srv, err := exec.LookPath("rest-server")
	if err != nil {
		t.Skip(err)
	}

	cmd := exec.CommandContext(ctx, srv, "--path", dir)
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
		return nil
	}

	return func() {
		if err := cmd.Process.Kill(); err != nil {
			t.Fatal(err)
		}

		// ignore errors, we've killed the process
		_ = cmd.Wait()
	}
}

func TestBackend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir, cleanup := TempDir(t)
	defer cleanup()

	cleanup = runRESTServer(ctx, t, dir)
	defer cleanup()

	suite := test.Suite{
		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			dir, err := ioutil.TempDir(TestTempDir, "restic-test-rest-")
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("create new backend at %v", dir)

			url, err := url.Parse("http://localhost:8000/restic-test")
			if err != nil {
				t.Fatal(err)
			}

			cfg := rest.Config{
				URL: url,
			}
			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			cfg := config.(rest.Config)
			return rest.Create(cfg)
		},

		// OpenFn is a function that opens a previously created temporary repository.
		Open: func(config interface{}) (restic.Backend, error) {
			cfg := config.(rest.Config)
			return rest.Open(cfg)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			return nil
		},
	}

	suite.RunTests(t)
}
