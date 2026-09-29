package fileio

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// probeHelperEnv tells TestProbeHelper how to behave when it stands in for a probe
// process. Exercising the timeout needs a process that does not come back, which is
// easier to arrange than a filesystem that does not answer.
const probeHelperEnv = "RESTIC_TEST_PROBE_HELPER"

// TestMain makes this test binary answer probes, the way cmd/restic does. Without it
// the package's own preallocation tests would find preallocation switched off.
func TestMain(m *testing.M) {
	if RunProbe() {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestProbeHelper is not a test. It is the process the three tests below start.
func TestProbeHelper(t *testing.T) {
	switch os.Getenv(probeHelperEnv) {
	case "answer":
		fmt.Println(probeToken)
		os.Exit(0)
	case "silent": // a binary that does not call RunProbe
		os.Exit(0)
	case "hang":
		time.Sleep(time.Minute)
		os.Exit(0)
	}
}

func probeHelper(behaviour string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestProbeHelper$")
	cmd.Env = append(os.Environ(), probeHelperEnv+"="+behaviour)
	return cmd
}

// A probe that comes back settles the question, whatever it found.
func TestWaitForProbeAcceptsAnAnswer(t *testing.T) {
	if !waitForProbe(probeHelper("answer"), time.Minute) {
		t.Fatal("a probe that exited was not counted as an answer")
	}
}

// A binary that does not handle the probe argument answers immediately and cheaply:
// it must not be waited on, and must not be taken for a filesystem that answers.
func TestAskProbeProcessDeclinesANonCooperatingBinary(t *testing.T) {
	start := time.Now()
	// Stand-in for a binary that does not call RunProbe: given an argument it does
	// not know, it fails immediately instead of setting about its own work.
	if waitForProbe(exec.Command(os.Args[0], "--not-a-probe-argument", t.TempDir()), time.Minute) {
		t.Fatal("a binary that rejected the argument was counted as an answer")
	}
	if elapsed := time.Since(start); elapsed > 30*time.Second {
		t.Fatalf("took %v: it ran the binary's own work instead of being turned down", elapsed)
	}
}

// A binary that does not answer probes must not be taken for a filesystem that does.
func TestWaitForProbeNeedsTheToken(t *testing.T) {
	if waitForProbe(probeHelper("silent"), time.Minute) {
		t.Fatal("a process that printed nothing was counted as an answer")
	}
}

// A probe that does not come back must not hold up the caller: on a filesystem that
// never answers, the probe process is the one that gets stranded.
func TestWaitForProbeGivesUp(t *testing.T) {
	start := time.Now()
	if waitForProbe(probeHelper("hang"), 100*time.Millisecond) {
		t.Fatal("a probe that never exited was counted as an answer")
	}
	if elapsed := time.Since(start); elapsed > 30*time.Second {
		t.Fatalf("took %v: it waited for the probe instead of giving up", elapsed)
	}
}

// The probe must leave nothing in the directory it tested, whether or not it got as
// far as making the call, because a file whose preallocation is outstanding can no
// longer be unlinked.
func TestProbeDirectoryLeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	err := ProbeDirectory(dir)
	// Windows cannot unlink an open file, so there the probe declines to make the call
	// and reports why. Everywhere else it goes through on a working filesystem.
	if err != nil && runtime.GOOS != "windows" {
		t.Fatalf("probing a working filesystem failed: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("the probe left %d entries behind: %v", len(entries), entries)
	}
}

// A working filesystem answers, and the answer is remembered per filesystem rather
// than asked again for every directory on it.
func TestAnswersPreallocationOnAWorkingFilesystem(t *testing.T) {
	resetProbes(t)

	first, second := t.TempDir(), t.TempDir() // two directories, one filesystem
	if !answersPreallocation(first) {
		t.Fatal("a working filesystem was reported not to answer preallocation")
	}
	if !answersPreallocation(second) {
		t.Fatal("the second directory disagreed with the first")
	}
	probeMu.Lock()
	answers := len(probed)
	probeMu.Unlock()
	if answers != 1 {
		t.Fatalf("%d filesystems recorded for two directories on one, want 1", answers)
	}
}

// A filesystem that did not answer gets no further files preallocated on it.
func TestPreallocateFileSkipsAFilesystemThatDidNotAnswer(t *testing.T) {
	resetProbes(t)

	dir := t.TempDir()
	probeMu.Lock()
	probed[filesystemKey(dir)] = false
	probeMu.Unlock()

	f, err := os.Create(filepath.Join(dir, "file"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	if err := PreallocateFile(f, 4096); err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != 0 {
		t.Fatalf("size is %d, want 0: the file was preallocated anyway", fi.Size())
	}
}

// RunProbe must keep out of the way of a normal run. The test binary's own arguments
// stand in for them: they are not a probe request.
func TestRunProbeIgnoresANormalProcess(t *testing.T) {
	if RunProbe() {
		t.Fatalf("a normal process was taken for a probe, args: %v", os.Args)
	}
}

func resetProbes(t *testing.T) {
	t.Helper()
	set := func() {
		probeMu.Lock()
		defer probeMu.Unlock()
		probed = map[string]bool{}
	}
	set()
	t.Cleanup(set)
}
