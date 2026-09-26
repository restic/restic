package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/global"
	rtest "github.com/restic/restic/internal/test"
)

func testRunRecover(t testing.TB, gopts global.Options) {
	rtest.OK(t, withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runRecover(context.TODO(), gopts, gopts.Term)
	}))
}

func TestRecover(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	// must list index more than once
	env.gopts.BackendTestHook = nil
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
	rtest.OK(t, withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runCat(context.TODO(), gopts, []string{"tree", ids[0].String() + ":" + sn.Tree.Str()}, gopts.Term)
	}))
}

// TestRecoverMultipleRoots covers https://github.com/restic/restic/issues/22018:
// recover must process unreferenced roots in a stable order, since the node
// names it derives from them must be added in strictly ascending order. A
// single root is trivially ordered, so distinct content is backed up here to
// produce several roots instead of reusing testSetupBackupData's one fixture.
func TestRecoverMultipleRoots(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	// must list index more than once
	env.gopts.BackendTestHook = nil
	defer cleanup()

	testRunInit(t, env.gopts)

	for i := 0; i < 5; i++ {
		dir := filepath.Join(env.testdata, "data"+string(rune('a'+i)))
		rtest.OK(t, os.MkdirAll(dir, 0755))
		rtest.OK(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte{byte(i), byte(i + 1)}, 0644))
		testRunBackup(t, "", []string{dir}, BackupOptions{}, env.gopts)
	}

	ids := testListSnapshots(t, env.gopts, 5)
	for _, id := range ids {
		testRunForget(t, env.gopts, ForgetOptions{}, id.String())
	}
	testListSnapshots(t, env.gopts, 0)

	testRunRecover(t, env.gopts)
	testListSnapshots(t, env.gopts, 1)
	testRunCheck(t, env.gopts)
}
