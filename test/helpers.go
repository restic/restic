package test_helper

import (
	"compress/bzip2"
	"compress/gzip"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	mrand "math/rand"

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

// ParseID parses s as a backend.ID and panics if that fails.
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

	rnd := mrand.New(mrand.NewSource(int64(seed)))
	for i := 0; i < count; i++ {
		buf[i] = byte(rnd.Uint32())
	}

	return buf
}

type rndReader struct {
	src *mrand.Rand
}

func (r *rndReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(r.src.Uint32())
	}

	return len(p), nil
}

// RandomReader returns a reader that returns size bytes of pseudo-random data
// derived from the seed.
func RandomReader(seed, size int) io.Reader {
	r := &rndReader{src: mrand.New(mrand.NewSource(int64(seed)))}
	return io.LimitReader(r, int64(size))
}

// GenRandom returns a []byte filled with up to 1000 random bytes.
func GenRandom(t testing.TB) []byte {
	buf := make([]byte, mrand.Intn(1000))
	_, err := io.ReadFull(rand.Reader, buf)
	OK(t, err)
	return buf
}

// SetupTarTestFixture extracts the tarFile to outputDir.
func SetupTarTestFixture(t testing.TB, outputDir, tarFile string) {
	input, err := os.Open(tarFile)
	defer input.Close()
	OK(t, err)

	var rd io.Reader
	switch filepath.Ext(tarFile) {
	case ".gz":
		r, err := gzip.NewReader(input)
		OK(t, err)

		defer r.Close()
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

	OK(t, cmd.Run())
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

	if !TestCleanupTempDirs {
		t.Logf("leaving temporary directory %v used for test", tempdir)
		return
	}

	RemoveAll(t, tempdir)
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

func isFile(fi os.FileInfo) bool {
	return fi.Mode()&(os.ModeType|os.ModeCharDevice) == 0
}

// ResetReadOnly recursively resets the read-only flag recursively for dir.
// This is mainly used for tests on Windows, which is unable to delete a file
// set read-only.
func ResetReadOnly(t testing.TB, dir string) {
	OK(t, filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if fi.IsDir() {
			return os.Chmod(path, 0777)
		}

		if isFile(fi) {
			return os.Chmod(path, 0666)
		}

		return nil
	}))
}

// RemoveAll recursively resets the read-only flag of all files and dirs and
// afterwards uses os.RemoveAll() to remove the path.
func RemoveAll(t testing.TB, path string) {
	ResetReadOnly(t, path)
	OK(t, os.RemoveAll(path))
}
