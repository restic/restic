package backend_test

import (
	"flag"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/restic/restic/backend/local"
	. "github.com/restic/restic/test"
)

var testCleanup = flag.Bool("test.cleanup", true, "clean up after running tests (remove local backend directory with all content)")

func setupLocalBackend(t *testing.T) *local.Local {
	tempdir, err := ioutil.TempDir("", "restic-test-")
	OK(t, err)

	b, err := local.Create(tempdir)
	OK(t, err)

	t.Logf("created local backend at %s", tempdir)

	return b
}

func teardownLocalBackend(t *testing.T, b *local.Local) {
	if !*testCleanup {
		t.Logf("leaving local backend at %s\n", b.Location())
		return
	}

	OK(t, b.Delete())
}

func TestLocalBackend(t *testing.T) {
	// test for non-existing backend
	b, err := local.Open("/invalid-restic-test")
	Assert(t, err != nil, "opening invalid repository at /invalid-restic-test should have failed, but err is nil")
	Assert(t, b == nil, fmt.Sprintf("opening invalid repository at /invalid-restic-test should have failed, but b is not nil: %v", b))

	s := setupLocalBackend(t)
	defer teardownLocalBackend(t, s)

	testBackend(s, t)
}

func TestLocalBackendCreationFailures(t *testing.T) {
	b := setupLocalBackend(t)
	defer teardownLocalBackend(t, b)

	// test failure to create a new repository at the same location
	b2, err := local.Create(b.Location())
	Assert(t, err != nil && b2 == nil, fmt.Sprintf("creating a repository at %s for the second time should have failed", b.Location()))

	// test failure to create a new repository at the same location without a config file
	b2, err = local.Create(b.Location())
	Assert(t, err != nil && b2 == nil, fmt.Sprintf("creating a repository at %s for the second time should have failed", b.Location()))
}
