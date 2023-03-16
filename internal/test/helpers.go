package test

import (
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/restic/restic/internal/errors"

	mrand "math/rand"
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
		fmt.Printf("\033[31m%s:%d: unexpected error: %+v\033[39m\n\n", filepath.Base(file), line, err)
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
			fmt.Printf("\033[31m%s:%d: unexpected error: %+v\033[39m\n\n", filepath.Base(file), line, err.Error())
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

// Random returns size bytes of pseudo-random data derived from the seed.
func Random(seed, count int) []byte {
	p := make([]byte, count)

	rnd := mrand.New(mrand.NewSource(int64(seed)))

	for i := 0; i < len(p); i += 8 {
		val := rnd.Int63()
		var data = []byte{
			byte((val >> 0) & 0xff),
			byte((val >> 8) & 0xff),
			byte((val >> 16) & 0xff),
			byte((val >> 24) & 0xff),
			byte((val >> 32) & 0xff),
			byte((val >> 40) & 0xff),
			byte((val >> 48) & 0xff),
			byte((val >> 56) & 0xff),
		}

		for j := range data {
			cur := i + j
			if cur >= len(p) {
				break
			}
			p[cur] = data[j]
		}
	}

	return p
}

// SetupTarTestFixture extracts the tarFile to outputDir.
func SetupTarTestFixture(t testing.TB, outputDir, tarFile string) {
	input, err := os.Open(tarFile)
	OK(t, err)
	defer func() {
		OK(t, input.Close())
	}()

	var rd io.Reader
	switch filepath.Ext(tarFile) {
	case ".gz":
		r, err := gzip.NewReader(input)
		OK(t, err)

		defer func() {
			OK(t, r.Close())
		}()
		rd = r
	case ".bzip2":
		rd = bzip2.NewReader(input)
	default:
		rd = input
	}

	cmd := exec.Command("tar", "xf", "-")
	cmd.Dir = outputDir

	cmd.Stdin = rd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		t.Fatalf("running command %v %v failed: %v", cmd.Path, cmd.Args, err)
	}
}

// Env creates a test environment and extracts the repository fixture.
// Returned is the repo path and a cleanup function.
func Env(t testing.TB, repoFixture string) (repodir string, cleanup func()) {
	tempdir, err := os.MkdirTemp(TestTempDir, "restic-test-env-")
	OK(t, err)

	fd, err := os.Open(repoFixture)
	if err != nil {
		t.Fatal(err)
	}
	OK(t, fd.Close())

	SetupTarTestFixture(t, tempdir, repoFixture)

	return filepath.Join(tempdir, "repo"), func() {
		if !TestCleanupTempDirs {
			t.Logf("leaving temporary directory %v used for test", tempdir)
			return
		}

		RemoveAll(t, tempdir)
	}
}

func isFile(fi os.FileInfo) bool {
	return fi.Mode()&(os.ModeType|os.ModeCharDevice) == 0
}

// ResetReadOnly recursively resets the read-only flag recursively for dir.
// This is mainly used for tests on Windows, which is unable to delete a file
// set read-only.
func ResetReadOnly(t testing.TB, dir string) {
	err := filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if fi == nil {
			return err
		}

		if fi.IsDir() {
			return os.Chmod(path, 0777)
		}

		if isFile(fi) {
			return os.Chmod(path, 0666)
		}

		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	OK(t, err)
}

// RemoveAll recursively resets the read-only flag of all files and dirs and
// afterwards uses os.RemoveAll() to remove the path.
func RemoveAll(t testing.TB, path string) {
	ResetReadOnly(t, path)
	err := os.RemoveAll(path)
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	OK(t, err)
}

// TempDir returns a temporary directory that is removed by t.Cleanup,
// except if TestCleanupTempDirs is set to false.
func TempDir(t testing.TB) string {
	tempdir, err := os.MkdirTemp(TestTempDir, "restic-test-")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if !TestCleanupTempDirs {
			t.Logf("leaving temporary directory %v used for test", tempdir)
			return
		}

		RemoveAll(t, tempdir)
	})
	return tempdir
}

// Chdir changes the current directory to dest.
// The function back returns to the previous directory.
func Chdir(t testing.TB, dest string) (back func()) {
	t.Helper()

	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("chdir to %v", dest)
	err = os.Chdir(dest)
	if err != nil {
		t.Fatal(err)
	}

	return func() {
		t.Helper()
		t.Logf("chdir back to %v", prev)
		err = os.Chdir(prev)
		if err != nil {
			t.Fatal(err)
		}
	}
}
