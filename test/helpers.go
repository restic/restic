package test_helper

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/backend/local"
	"github.com/restic/restic/repository"
)

// Assert fails the test if the condition is false.
func Assert(tb testing.TB, condition bool, msg string, v ...interface{}) {
	if !condition {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: "+msg+"\033[39m\n\n", append([]interface{}{filepath.Base(file), line}, v...)...)
		tb.FailNow()
	}
}

// OK fails the test if an err is not nil.
func OK(tb testing.TB, err error) {
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: unexpected error: %s\033[39m\n\n", filepath.Base(file), line, err.Error())
		tb.FailNow()
	}
}

// OKs fails the test if any error from errs is not nil.
func OKs(tb testing.TB, errs []error) {
	errFound := false
	for _, err := range errs {
		if err != nil {
			errFound = true
			_, file, line, _ := runtime.Caller(1)
			fmt.Printf("\033[31m%s:%d: unexpected error: %s\033[39m\n\n", filepath.Base(file), line, err.Error())
		}
	}
	if errFound {
		tb.FailNow()
	}
}

// Equals fails the test if exp is not equal to act.
func Equals(tb testing.TB, exp, act interface{}) {
	if !reflect.DeepEqual(exp, act) {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d:\n\n\texp: %#v\n\n\tgot: %#v\033[39m\n\n", filepath.Base(file), line, exp, act)
		tb.FailNow()
	}
}

func ParseID(s string) backend.ID {
	id, err := backend.ParseID(s)
	if err != nil {
		panic(err)
	}

	return id
}

// Random returns size bytes of pseudo-random data derived from the seed.
func Random(seed, count int) []byte {
	buf := make([]byte, count)

	rnd := rand.New(rand.NewSource(int64(seed)))
	for i := 0; i < count; i++ {
		buf[i] = byte(rnd.Uint32())
	}

	return buf
}

// RandomReader returns a reader that returns size bytes of pseudo-random data
// derived from the seed.
func RandomReader(seed, size int) *bytes.Reader {
	return bytes.NewReader(Random(seed, size))
}

// SetupTarTestFixture extracts the tarFile to outputDir.
func SetupTarTestFixture(t testing.TB, outputDir, tarFile string) {
	err := System("sh", "-c", `(cd "$1" && tar xzf - ) < "$2"`,
		"sh", outputDir, tarFile)
	OK(t, err)
}

// System runs the command and returns the exit code. Output is passed on to
// stdout/stderr.
func System(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// WithTestEnvironment creates a test environment, extracts the repository
// fixture and and calls f with the repository dir.
func WithTestEnvironment(t testing.TB, repoFixture string, f func(repodir string)) {
	tempdir, err := ioutil.TempDir(TestTempDir, "restic-test-")
	OK(t, err)

	fd, err := os.Open(repoFixture)
	if err != nil {
		panic(err)
	}
	OK(t, fd.Close())

	SetupTarTestFixture(t, tempdir, repoFixture)

	f(filepath.Join(tempdir, "repo"))

	if !TestCleanup {
		t.Logf("leaving temporary directory %v used for test", tempdir)
		return
	}

	OK(t, os.RemoveAll(tempdir))
}

// OpenLocalRepo opens the local repository located at dir.
func OpenLocalRepo(t testing.TB, dir string) *repository.Repository {
	be, err := local.Open(dir)
	OK(t, err)

	repo := repository.New(be)
	err = repo.SearchKey(TestPassword)
	OK(t, err)

	return repo
}
