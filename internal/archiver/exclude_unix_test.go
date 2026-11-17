//go:build unix

package archiver

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/test"
)

// TestIsExcludedByFileNonRegularTagfile checks that a tagfile which is
// not a regular file does not block the backup: opening a FIFO for
// reading blocks until a writer shows up.
func TestIsExcludedByFileNonRegularTagfile(t *testing.T) {
	const (
		tagFilename = "CACHEDIR.TAG"
		header      = "Signature: 8a477f597d28d172789f06886806bc55"
	)

	setup := func(t *testing.T, makeTagfile func(dir string)) string {
		t.Helper()
		dir := test.TempDir(t)
		foo := filepath.Join(dir, "foo")
		test.OK(t, os.WriteFile(foo, []byte("foo"), 0o600))
		makeTagfile(dir)
		return foo
	}

	t.Run("FIFO", func(t *testing.T) {
		foo := setup(t, func(dir string) {
			test.OK(t, syscall.Mkfifo(filepath.Join(dir, tagFilename), 0o600))
		})
		if got := isExcludedByFile(foo, tagFilename, header, newRejectionCache(), fs.NewLocal(), func(string, ...any) {}); got {
			t.Fatalf("FIFO tagfile must not exclude the directory, got %v", got)
		}
	})

	t.Run("SymlinkToFIFO", func(t *testing.T) {
		foo := setup(t, func(dir string) {
			fifo := filepath.Join(dir, "realfifo")
			test.OK(t, syscall.Mkfifo(fifo, 0o600))
			test.OK(t, os.Symlink(fifo, filepath.Join(dir, tagFilename)))
		})
		if got := isExcludedByFile(foo, tagFilename, header, newRejectionCache(), fs.NewLocal(), func(string, ...any) {}); got {
			t.Fatalf("tagfile symlink to a FIFO must not exclude the directory, got %v", got)
		}
	})

	t.Run("SymlinkToRegularFile", func(t *testing.T) {
		foo := setup(t, func(dir string) {
			target := filepath.Join(dir, "target")
			test.OK(t, os.WriteFile(target, []byte(header), 0o600))
			test.OK(t, os.Symlink(target, filepath.Join(dir, tagFilename)))
		})
		if got := isExcludedByFile(foo, tagFilename, header, newRejectionCache(), fs.NewLocal(), func(string, ...any) {}); !got {
			t.Fatalf("tagfile symlink to a regular file with a valid signature must exclude the directory, got %v", got)
		}
	})

	t.Run("Device", func(t *testing.T) {
		foo := setup(t, func(dir string) {
			test.OK(t, os.Symlink("/dev/null", filepath.Join(dir, tagFilename)))
		})
		if got := isExcludedByFile(foo, tagFilename, header, newRejectionCache(), fs.NewLocal(), func(string, ...any) {}); got {
			t.Fatalf("tagfile pointing at a device must not exclude the directory, got %v", got)
		}
	})
}
