package rest_test

import (
	"bufio"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"restic"
	"strings"
	"testing"

	"restic/backend/rest"
	"restic/backend/test"
	. "restic/test"
)

func runRESTServer(t testing.TB, dir string) func() {
	srv, err := exec.LookPath("rest-server")
	if err != nil {
		t.Skip(err)
	}

	cmd := exec.Command(srv, "--path", dir)
	cmd.Stdout = os.Stdout
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	sc := bufio.NewScanner(stderr)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "Starting server") {
			break
		}
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
	dir, cleanup := TempDir(t)
	defer cleanup()

	cleanup = runRESTServer(t, dir)
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
