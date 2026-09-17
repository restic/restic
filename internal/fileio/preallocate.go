package fileio

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// probeFlag asks a copy of this executable to test one directory, and probeToken is
// what it prints once the call has come back. Both are internal to askProbeProcess.
const (
	probeFlag  = "--__preallocate-probe"
	probeToken = "restic-preallocate-probe-answered"
)

// probeTimeout bounds one probe. A filesystem that implements preallocation answers
// in microseconds, so this is generous.
const probeTimeout = 5 * time.Second

var (
	probeMu sync.Mutex
	// probed records, per filesystem, whether it answers preallocation. Asking is
	// expensive — it costs a process — so the answer must not be looked up per
	// directory: a restore walks thousands of them and they nearly all share a
	// filesystem.
	probed   = make(map[string]bool)
	warnOnce sync.Once
)

// PreallocateFile reserves space for a file that is about to be written. It is an
// optimization: every caller lets the writes extend the file when it fails.
//
// The syscall is issued only on a filesystem that has already answered it once,
// because issuing it is irreversible and, on a filesystem that does not answer,
// ruinous. The kernel holds the inode lock while it waits, so the file can never
// afterwards be written, truncated, or unlinked — unlinking it blocks as well, and
// takes its directory with it. Worse, the waiting thread cannot be interrupted, not
// even by SIGKILL, so the process it belongs to can no longer exit. A deadline around
// the call would therefore not only come too late for that file, it would doom the
// run: see ProbeDirectory for how the question is asked safely instead.
func PreallocateFile(wr *os.File, size int64) error {
	if size <= 0 || !answersPreallocation(filepath.Dir(wr.Name())) {
		return nil
	}
	return preallocateFile(wr, size)
}

// ProbeDirectory preallocates a throwaway file in dir and reports whether the call
// returned. It is meant for a process that exists only to ask that question: a
// filesystem that does not answer leaves a thread that can never be reaped, so the
// asking process may never be able to exit.
//
// The throwaway file is unlinked before the call, leaving nothing behind that anyone
// could trip over — and nothing to clean up afterwards, which matters because
// unlinking a file whose preallocation is outstanding blocks in the same way.
func ProbeDirectory(dir string) error {
	f, err := os.CreateTemp(dir, ".restic-preallocate-probe-")
	if err != nil {
		return err
	}
	if err := os.Remove(f.Name()); err != nil {
		// Windows cannot unlink an open file, so there is no way to make the call
		// without risking a file that can never be removed. Leave it untried: the
		// filesystems that do not answer are not the ones found there.
		_ = f.Close()
		_ = os.Remove(f.Name())
		return err
	}
	defer func() { _ = f.Close() }()

	return preallocateFile(f, 4096)
}

// RunProbe reports whether this process was started to probe a directory, and if so
// does it and says on stdout that the call came back. A process that answers here must
// exit immediately afterwards; it cannot be reused, as it may hold a syscall that will
// never return.
//
// A binary that links this package should call RunProbe before parsing its arguments.
// One that does not will reject the argument instead, and preallocation is then left
// alone rather than risked — see askProbeProcess.
func RunProbe() (ranProbe bool) {
	if len(os.Args) != 3 || os.Args[1] != probeFlag {
		return false
	}
	dir := os.Args[2]
	// Whether the call failed does not matter, only that it came back at all. Saying
	// so is the whole purpose of this process.
	_ = ProbeDirectory(dir)
	fmt.Println(probeToken)
	return true
}

// answersPreallocation reports whether preallocation returns for files in dir, asking
// once per filesystem and remembering the answer.
func answersPreallocation(dir string) bool {
	if dir == "" {
		return false
	}

	probeMu.Lock()
	defer probeMu.Unlock()

	key := filesystemKey(dir)
	if answer, ok := probed[key]; ok {
		return answer
	}

	answer := askProbeProcess(dir, probeTimeout)
	if !answer {
		warnOnce.Do(func() {
			fmt.Fprintf(os.Stderr, "preallocation did not return within %s on this filesystem; continuing without it\n", probeTimeout)
		})
	}
	probed[key] = answer
	return answer
}

// filesystemKey identifies the filesystem holding dir: its device where that can be
// had, and otherwise the volume it sits on, which is as close as Windows gets.
func filesystemKey(dir string) string {
	if dev, ok := deviceID(dir); ok {
		return strconv.FormatUint(dev, 10)
	}
	return filepath.VolumeName(dir)
}

// askProbeProcess runs a copy of this executable to preallocate a throwaway file in
// dir, and reports whether it got that far within timeout. Whether the call succeeded
// is not the question — only whether the filesystem answers at all, which a failing
// call also settles.
//
// The work happens in another process so that a filesystem which never answers
// strands that process instead of this one. The strand cannot be undone: the child is
// asked to go away, but a thread waiting on such a filesystem ignores even SIGKILL.
//
// The child says so by printing probeToken. A binary that does not handle probeFlag
// rejects it straight away and prints nothing, so preallocation is left alone rather
// than risked — and since such a child never gets as far as preallocating anything, it
// cannot start a probe of its own.
func askProbeProcess(dir string, timeout time.Duration) bool {
	self, err := os.Executable()
	if err != nil {
		return false
	}

	return waitForProbe(exec.Command(self, probeFlag, dir), timeout)
}

// waitForProbe runs cmd and reports whether it answered — printed probeToken and
// exited — within timeout.
func waitForProbe(cmd *exec.Cmd, timeout time.Duration) bool {
	out := &bytes.Buffer{}
	cmd.Stdout = out
	if err := cmd.Start(); err != nil {
		return false
	}

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	select {
	case <-done:
		return strings.Contains(out.String(), probeToken)
	case <-deadline.C:
		_ = cmd.Process.Kill()
		return false
	}
}
