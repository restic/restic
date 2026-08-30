package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restic/restic/internal/global"
	rtest "github.com/restic/restic/internal/test"
)

// testRunStats runs `restic stats` with capturing std and stderr
func testRunStats(t testing.TB, wantJSON bool, opts StatsOptions, gopts global.Options, args []string,
) ([]byte, []byte) {

	bufStdout, bufStderr, err := withCaptureStdoutStderr(t, gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.JSON = wantJSON

		return runStats(ctx, opts, gopts, args, gopts.Term)
	})
	rtest.OK(t, err)

	return bufStdout.Bytes(), bufStderr.Bytes()
}

func TestStatsDebug1(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	// backup of subtree 0/0/9
	testRunBackup(t, env.testdata, []string{filepath.Join(env.testdata, "0", "0", "9")}, BackupOptions{}, env.gopts)

	optsStats := StatsOptions{countMode: "debug"}
	// allow multiple reads of different restic file types
	env.gopts.BackendTestHook = nil
	stdout, stderr := testRunStats(t, false, optsStats, env.gopts, nil)

	rtest.Equals(t, "", string(stdout), "stdout should be empty")
	offset := bytes.Index(stderr, []byte("Distinct"))
	rtest.Assert(t, offset > 0, "did not find the word 'Distinct'")
	lines := strings.Split(string(stderr[offset:]), "\n")
	rtest.Assert(t, len(lines) > 11, "expected at least 11 lines of output starting from here")
	rtest.Equals(t, "Count: 69", lines[1])
	rtest.Equals(t, "10000 - 99999 Byte  69", lines[5])
	rtest.Equals(t, "file            69", lines[11])
}

func TestStatsDebug2(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	// backup of subtree 0/for_cmd_ls
	testRunBackup(t, env.testdata, []string{filepath.Join(env.testdata, "0", "for_cmd_ls")}, BackupOptions{}, env.gopts)

	env.gopts.BackendTestHook = nil
	optsStats := StatsOptions{countMode: "debug"}
	stdout, stderr := testRunStats(t, false, optsStats, env.gopts, nil)

	rtest.Equals(t, "", string(stdout), "stdout should be empty")
	offset := bytes.Index(stderr, []byte("extension"))
	rtest.Assert(t, offset > 0, "did not find the word 'extension'")
	lines := strings.Split(string(stderr[offset:]), "\n")
	rtest.Assert(t, len(lines) > 6, "expected at least 6 lines of output starting from here")
	rtest.Equals(t, "txt                        2       118 B", lines[2])
	rtest.Equals(t, "py                         1       113 B", lines[3])
}
