package main

import (
	"context"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func testRunForgetMayFail(t testing.TB, gopts global.Options, opts ForgetOptions, args ...string) error {
	pruneOpts := PruneOptions{
		MaxUnused: "5%",
	}
	return withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runForget(context.TODO(), opts, pruneOpts, gopts, gopts.Term, args)
	})
}

func testRunForget(t testing.TB, gopts global.Options, opts ForgetOptions, args ...string) {
	rtest.OK(t, testRunForgetMayFail(t, gopts, opts, args...))
}

func testRunForgetWithOutput(t testing.TB, wantJSON bool, opts ForgetOptions,
	pruneOpts PruneOptions, gopts global.Options, args []string) []byte {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.JSON = wantJSON

		return runForget(context.TODO(), opts, pruneOpts, gopts, gopts.Term, args)
	})
	rtest.OK(t, err)
	return buf.Bytes()
}

const charset = "abcdefghijklmnopqrstuvwxyz ABCDEFGHIJKLMNOPQRSTUVWXYZ 0123456789 /=-+*{}[]<>()\n"

// GenerateRandomText returns a random string of length n
func testGenerateRandomText(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return b
}

func testCreateRandomTextFile(t *testing.T, filename string, sizeBytes int) {
	f, err := os.Create(filename)
	rtest.OK(t, err)

	defer func() {
		err := f.Close()
		rtest.OK(t, err)
	}()

	data := testGenerateRandomText(sizeBytes)
	_, err = f.Write(data)
	rtest.OK(t, err)
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
	err := testRunForgetMayFail(t, env.gopts, ForgetOptions{
		KeepTags: data.TagLists{data.TagList{"invalid"}},
		GroupBy:  data.SnapshotGroupByOptions{Host: true, Path: true},
	})
	rtest.Assert(t, strings.Contains(err.Error(), `refusing to delete last snapshot of snapshot group "host example, path`), "wrong error message got %v", err)

	// disallow `forget --unsafe-allow-remove-all`
	err = testRunForgetMayFail(t, env.gopts, ForgetOptions{
		UnsafeAllowRemoveAll: true,
	})
	rtest.Assert(t, strings.Contains(err.Error(), `--unsafe-allow-remove-all is not allowed unless a snapshot filter option is specified`), "wrong error message got %v", err)

	// disallow `forget` without options
	err = testRunForgetMayFail(t, env.gopts, ForgetOptions{})
	rtest.Assert(t, strings.Contains(err.Error(), `no policy was specified, no snapshots will be removed`), "wrong error message got %v", err)

	// `forget --host example --unsafe-allow-remove-all` should work
	testRunForget(t, env.gopts, ForgetOptions{
		UnsafeAllowRemoveAll: true,
		GroupBy:              data.SnapshotGroupByOptions{Host: true, Path: true},
		SnapshotFilter: data.SnapshotFilter{
			Hosts: []string{opts.Host},
		},
	})
	testListSnapshots(t, env.gopts, 0)
}

func TestRunForgetShowRemovedFiles(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)

	optsBackup := BackupOptions{}
	backupPath := filepath.Join(env.testdata, "0", "0", "9")
	rtest.OK(t, os.Remove(filepath.Join(backupPath, "0")))
	for i := 4; i < 68; i++ {
		rtest.OK(t, os.Remove(filepath.Join(backupPath, strconv.Itoa(i))))
	}

	// files f1, f2, f3
	testRunBackup(t, "", []string{backupPath}, optsBackup, env.gopts)
	snapshotIDs := testListSnapshots(t, env.gopts, 1)
	sn1 := snapshotIDs[0]
	sn1Str := sn1.Str()

	f1 := filepath.Join(backupPath, "1")
	f2 := filepath.Join(backupPath, "2")
	f3 := filepath.Join(backupPath, "3")
	f4 := filepath.Join(backupPath, "4")
	f5 := filepath.Join(backupPath, "5")
	rtest.OK(t, os.Remove(f1))
	testCreateRandomTextFile(t, f4, 10)

	// file f2, f3, new f4
	testRunBackup(t, "", []string{backupPath}, optsBackup, env.gopts)
	snapshotIDs = testListSnapshots(t, env.gopts, 2)
	snapSet := restic.NewIDSet(snapshotIDs...)
	sn2 := snapSet.Sub(restic.NewIDSet(sn1)).List()[0]
	sn2Str := sn2.Str()

	rtest.OK(t, os.Remove(f2))
	testCreateRandomTextFile(t, f1, 10)
	testCreateRandomTextFile(t, f5, 10)

	// file new f1, f3, f4, new f5
	testRunBackup(t, "", []string{backupPath}, optsBackup, env.gopts)
	snapshotIDs = testListSnapshots(t, env.gopts, 3)
	snapSet = restic.NewIDSet(snapshotIDs...)
	sn3 := snapSet.Sub(restic.NewIDSet(sn1, sn2)).List()[0]
	sn3Str := sn3.Str()

	rtest.OK(t, os.Remove(f3))
	testCreateRandomTextFile(t, f2, 10)

	// file new f2, f4, f5
	testRunBackup(t, "", []string{backupPath}, optsBackup, env.gopts)
	snapshotIDs = testListSnapshots(t, env.gopts, 4)
	snapSet = restic.NewIDSet(snapshotIDs...)
	sn4 := snapSet.Sub(restic.NewIDSet(sn1, sn2, sn3)).List()[0]
	sn4Str := sn4.Str()

	optsForget := ForgetOptions{
		DryRun:           true,
		ShowRemovedFiles: true,
	}
	optsForgetS := ForgetOptions{
		DryRun:           true,
		ShowRemovedFiles: true,
		SearchFiles:      true,
	}
	pruneOpts := PruneOptions{
		MaxUnused: "unlimited",
	}

	// the xxx[2:] is to get rid of the difference of windows paths in and out
	// "C:/Users/RUNNER~1/AppData/Local/Temp/restic-test-2058676641/testdata/0/0/9/1" versus
	// "/C/Users/RUNNER~1/AppData/Local/Temp/restic-test-2058676641/testdata/0/0/9/1"

	output := testRunForgetWithOutput(t, true, optsForget, pruneOpts, env.gopts, []string{sn1Str})
	deletedFilenames := DeletedFilenamesJSON{}
	rtest.OK(t, json.Unmarshal(output, &deletedFilenames))
	rtest.Equals(t, 1, len(deletedFilenames.DeletedFiles))
	rtest.Equals(t, sn1Str, deletedFilenames.DeletedFiles[0].SnapshotID.Str())
	rtest.Equals(t, filepath.ToSlash(f1)[2:], filepath.ToSlash(deletedFilenames.DeletedFiles[0].Path)[2:])

	output = testRunForgetWithOutput(t, true, optsForget, pruneOpts, env.gopts, []string{sn2Str})
	rtest.OK(t, json.Unmarshal(output, &deletedFilenames))
	rtest.Equals(t, 0, len(deletedFilenames.DeletedFiles))

	output = testRunForgetWithOutput(t, true, optsForget, pruneOpts, env.gopts, []string{sn1Str, sn2Str})
	rtest.OK(t, json.Unmarshal(output, &deletedFilenames))
	rtest.Equals(t, 2, len(deletedFilenames.DeletedFiles))
	rtest.Equals(t, sn1Str, deletedFilenames.DeletedFiles[0].SnapshotID.Str())
	rtest.Equals(t, filepath.ToSlash(f1)[2:], filepath.ToSlash(deletedFilenames.DeletedFiles[0].Path)[2:])

	rtest.Equals(t, sn1Str, deletedFilenames.DeletedFiles[1].SnapshotID.Str())
	rtest.Equals(t, filepath.ToSlash(f2)[2:], filepath.ToSlash(deletedFilenames.DeletedFiles[1].Path)[2:])

	output = testRunForgetWithOutput(t, true, optsForget, pruneOpts, env.gopts, []string{sn2Str, sn3Str, sn4Str})
	rtest.OK(t, json.Unmarshal(output, &deletedFilenames))

	rtest.Equals(t, 4, len(deletedFilenames.DeletedFiles))
	rtest.Equals(t, sn3Str, deletedFilenames.DeletedFiles[0].SnapshotID.Str())
	rtest.Equals(t, filepath.ToSlash(f1)[2:], filepath.ToSlash(deletedFilenames.DeletedFiles[0].Path)[2:])
	rtest.Equals(t, sn4Str, deletedFilenames.DeletedFiles[1].SnapshotID.Str())
	rtest.Equals(t, filepath.ToSlash(f2)[2:], filepath.ToSlash(deletedFilenames.DeletedFiles[1].Path)[2:])

	output = testRunForgetWithOutput(t, true, optsForgetS, pruneOpts, env.gopts, []string{sn2Str, sn3Str, sn4Str})
	rtest.OK(t, json.Unmarshal(output, &deletedFilenames))
	// can't investigate the difference since I have restic windows development environment
	// have to exclude this test from windows
	if runtime.GOOS != "windows" {
		rtest.Equals(t, sn2Str, deletedFilenames.DeletedFiles[0].SnapshotID.Str())
		rtest.Equals(t, filepath.ToSlash(f4)[2:], filepath.ToSlash(deletedFilenames.DeletedFiles[0].Path)[2:])
		rtest.Equals(t, sn3Str, deletedFilenames.DeletedFiles[1].SnapshotID.Str())
		rtest.Equals(t, filepath.ToSlash(f5)[2:], filepath.ToSlash(deletedFilenames.DeletedFiles[1].Path)[2:])
	}
}
