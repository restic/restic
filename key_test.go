package khepri_test

import (
	"flag"
	"io/ioutil"
	"os"
	"testing"

	"github.com/fd0/khepri"
	"github.com/fd0/khepri/backend"
)

var test_password = "foobar"
var testCleanup = flag.Bool("test.cleanup", true, "clean up after running tests (remove local backend directory with all content)")

func setupBackend(t *testing.T) *backend.Local {
	tempdir, err := ioutil.TempDir("", "khepri-test-")
	ok(t, err)

	b, err := backend.CreateLocal(tempdir)
	ok(t, err)

	t.Logf("created local backend at %s", tempdir)

	return b
}

func teardownBackend(t *testing.T, b *backend.Local) {
	if !*testCleanup {
		t.Logf("leaving local backend at %s\n", b.Location())
		return
	}

	ok(t, os.RemoveAll(b.Location()))
}

func setupKey(t *testing.T, be backend.Server, password string) *khepri.Key {
	c, err := khepri.CreateKey(be, password)
	ok(t, err)

	t.Logf("created Safe at %s", be.Location())

	return c
}

func TestSafe(t *testing.T) {
	be := setupBackend(t)
	defer teardownBackend(t, be)
	_ = setupKey(t, be, test_password)
}
