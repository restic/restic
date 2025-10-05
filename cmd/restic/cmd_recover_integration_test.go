package main

import (
	"context"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func testRunRecover(t testing.TB, gopts GlobalOptions) {
	rtest.OK(t, withTermStatus(t, gopts, func(ctx context.Context, gopts GlobalOptions) error {
		return runRecover(context.TODO(), gopts, gopts.term)
	}))
}

func TestRecover(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	// must list index more than once
	env.gopts.backendTestHook = nil
	defer cleanup()

	testSetupBackupData(t, env)

	// create backup and forget it afterwards
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	ids := testListSnapshots(t, env.gopts, 1)
	sn := testLoadSnapshot(t, env.gopts, ids[0])
	testRunForget(t, env.gopts, ForgetOptions{}, ids[0].String())
	testListSnapshots(t, env.gopts, 0)

	testRunRecover(t, env.gopts)
	ids = testListSnapshots(t, env.gopts, 1)
	testRunCheck(t, env.gopts)
	// check that the root tree is included in the snapshot
	rtest.OK(t, withTermStatus(t, env.gopts, func(ctx context.Context, gopts GlobalOptions) error {
		return runCat(context.TODO(), gopts, []string{"tree", ids[0].String() + ":" + sn.Tree.Str()}, gopts.term)
	}))
}
