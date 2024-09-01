package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/termstatus"
)

func testRunForgetMayFail(gopts GlobalOptions, opts ForgetOptions, args ...string) error {
	pruneOpts := PruneOptions{
		MaxUnused: "5%",
	}
	return withTermStatus(gopts, func(ctx context.Context, term *termstatus.Terminal) error {
		return runForget(context.TODO(), opts, pruneOpts, gopts, term, args)
	})
}

func testRunForget(t testing.TB, gopts GlobalOptions, opts ForgetOptions, args ...string) {
	rtest.OK(t, testRunForgetMayFail(gopts, opts, args...))
}

func TestRunForgetSafetyNet(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)

	opts := BackupOptions{
		Host: "example",
	}
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, opts, env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, opts, env.gopts)
	testListSnapshots(t, env.gopts, 2)

	// --keep-tags invalid
	err := testRunForgetMayFail(env.gopts, ForgetOptions{
		KeepTags: restic.TagLists{restic.TagList{"invalid"}},
		GroupBy:  restic.SnapshotGroupByOptions{Host: true, Path: true},
	})
	rtest.Assert(t, strings.Contains(err.Error(), `refusing to delete last snapshot of snapshot group "host example, path`), "wrong error message got %v", err)

	// disallow `forget --unsafe-allow-remove-all`
	err = testRunForgetMayFail(env.gopts, ForgetOptions{
		UnsafeAllowRemoveAll: true,
	})
	rtest.Assert(t, strings.Contains(err.Error(), `--unsafe-allow-remove-all is not allowed unless a snapshot filter option is specified`), "wrong error message got %v", err)

	// disallow `forget` without options
	err = testRunForgetMayFail(env.gopts, ForgetOptions{})
	rtest.Assert(t, strings.Contains(err.Error(), `no policy was specified, no snapshots will be removed`), "wrong error message got %v", err)

	// `forget --host example --unsafe-allow-remove-all` should work
	testRunForget(t, env.gopts, ForgetOptions{
		UnsafeAllowRemoveAll: true,
		GroupBy:              restic.SnapshotGroupByOptions{Host: true, Path: true},
		SnapshotFilter: restic.SnapshotFilter{
			Hosts: []string{opts.Host},
		},
	})
	testListSnapshots(t, env.gopts, 0)
}
