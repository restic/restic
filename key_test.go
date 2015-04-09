package restic_test

import (
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/backend/local"
	. "github.com/restic/restic/test"
)

var testPassword = "foobar"
var testCleanup = flag.Bool("test.cleanup", true, "clean up after running tests (remove local backend directory with all content)")
var testLargeCrypto = flag.Bool("test.largecrypto", false, "also test crypto functions with large payloads")
var testTempDir = flag.String("test.tempdir", "", "use this directory for temporary storage (default: system temp dir)")

func setupBackend(t testing.TB) restic.Server {
	tempdir, err := ioutil.TempDir(*testTempDir, "restic-test-")
	OK(t, err)

	// create repository below temp dir
	b, err := local.Create(filepath.Join(tempdir, "repo"))
	OK(t, err)

	// set cache dir
	err = os.Setenv("RESTIC_CACHE", filepath.Join(tempdir, "cache"))
	OK(t, err)

	return restic.NewServer(b)
}

func teardownBackend(t testing.TB, s restic.Server) {
	if !*testCleanup {
		l := s.Backend().(*local.Local)
		t.Logf("leaving local backend at %s\n", l.Location())
		return
	}

	OK(t, s.Delete())
}

func setupKey(t testing.TB, s restic.Server, password string) *restic.Key {
	k, err := restic.CreateKey(s, password)
	OK(t, err)

	return k
}

func TestRepo(t *testing.T) {
	s := setupBackend(t)
	defer teardownBackend(t, s)
	_ = setupKey(t, s, testPassword)
}
