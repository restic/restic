package main

import (
	"context"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restic/restic/internal/global"
	rtest "github.com/restic/restic/internal/test"
)

func testRunCat(t testing.TB, gopts global.Options, tpe string, ID string) []byte {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.Quiet = true
		return runCat(ctx, gopts, []string{tpe, ID}, gopts.Term)
	})
	rtest.OK(t, err)
	return buf.Bytes()
}

func TestFullTree(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}

	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, opts, env.gopts)
	testRunCheck(t, env.gopts)
	sn := testListSnapshots(t, env.gopts, 1)[0]
	snapID := sn.String()

	// gather the tree blobs from the repository: run 'restic list blobs'
	treeIDs := testRunListTreeBlobs(t, env.gopts)

	outString := string(testRunCat(t, env.gopts, "full-tree", snapID))

	rtest.Assert(t, strings.Contains(outString, `"root"`), "expected to find string 'root', but did not see it")
	rtest.Assert(t, strings.Contains(outString, snapID), "expected to find %q, but did not see it", snapID)
	backupPath := path.Join(env.testdata, "0", "0", "9")
	rtest.Assert(t, strings.Contains(outString, backupPath), "expected to find %q, but did not see it", backupPath)

	for _, treeID := range treeIDs {
		rtest.Assert(t, strings.Contains(outString, treeID), "expected treeID %s in output string, got nothing", treeID)
	}
}
