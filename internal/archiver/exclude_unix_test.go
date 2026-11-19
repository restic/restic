//go:build !windows

package archiver

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/restic/restic/internal/fs"
	rtest "github.com/restic/restic/internal/test"
)

func TestIsExcludedByFileFIFO(t *testing.T) {
	const (
		tagFilename = "CACHEDIR.TAG"
		header      = "Signature: 8a477f597d28d172789f06886806bc55"
	)

	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, dir string)
	}{
		{"fifo", func(t *testing.T, dir string) {
			rtest.OK(t, syscall.Mkfifo(filepath.Join(dir, tagFilename), 0o600))
		}},
		{"symlink to fifo", func(t *testing.T, dir string) {
			fifo := filepath.Join(t.TempDir(), "fifo")
			rtest.OK(t, syscall.Mkfifo(fifo, 0o600))
			rtest.OK(t, os.Symlink(fifo, filepath.Join(dir, tagFilename)))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			foo := filepath.Join(dir, "foo")
			rtest.OK(t, os.WriteFile(foo, []byte("foo"), 0o644))
			tc.setup(t, dir)

			var warnings []string
			warnf := func(msg string, args ...any) {
				warnings = append(warnings, msg)
			}

			done := make(chan bool, 1)
			go func() {
				done <- isExcludedByFile(foo, tagFilename, header, newRejectionCache(), fs.NewLocal(), warnf)
			}()

			select {
			case excluded := <-done:
				rtest.Assert(t, !excluded, "directory with a non-regular tagfile was excluded")
				rtest.Assert(t, len(warnings) == 1 && strings.Contains(warnings[0], "not a regular file"),
					"expected a warning about the non-regular tagfile, got %q", warnings)
			case <-time.After(5 * time.Second):
				t.Fatal("isExcludedByFile blocked on a FIFO tagfile")
			}
		})
	}
}
