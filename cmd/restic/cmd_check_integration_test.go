package main

import (
	"context"
	"strings"
	"testing"

	"github.com/restic/restic/internal/global"
	rtest "github.com/restic/restic/internal/test"
)

func testRunCheck(t testing.TB, gopts global.Options) {
	t.Helper()
	output, err := testRunCheckOutput(t, gopts, true)
	if err != nil {
		t.Error(output)
		t.Fatalf("unexpected error: %+v", err)
	}
}

func testRunCheckMustFail(t testing.TB, gopts global.Options) {
	t.Helper()
	_, err := testRunCheckOutput(t, gopts, false)
	rtest.Assert(t, err != nil, "expected non nil error after check of damaged repository")
}

func testRunCheckOutput(t testing.TB, gopts global.Options, checkUnused bool) (string, error) {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		opts := CheckOptions{
			ReadData:    true,
			CheckUnused: checkUnused,
		}
		_, err := runCheck(context.TODO(), opts, gopts, nil, gopts.Term)
		return err
	})
	return buf.String(), err
}

func testRunCheckOutputWithOpts(t testing.TB, gopts global.Options, opts CheckOptions, args []string) (string, error) {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.Verbosity = 2
		gopts.Verbose = 2
		_, err := runCheck(context.TODO(), opts, gopts, args, gopts.Term)
		return err
	})
	return buf.String(), err
}

func TestCheckFullOutput(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, env.testdata+"/0", []string{"for_cmd_ls"}, opts, env.gopts)
	testRunBackup(t, env.testdata+"/0", []string{"0/9"}, opts, env.gopts)

	snapshotIDs := testRunList(t, env.gopts, "snapshots")
	rtest.Assert(t, len(snapshotIDs) == 2, "expected two snapshots, got %v", snapshotIDs)

	output, err := testRunCheckOutputWithOpts(t, env.gopts, CheckOptions{ReadData: true}, nil)
	rtest.OK(t, err)

	// walk through 'output' and find
	// 'read data'
	// '4 / 4 packs'
	index := strings.Index(output, "read data")
	rtest.Assert(t, index >= 0, `expected to find substring "read data", but did not find it`)

	index = strings.Index(output, "4 / 4 packs")
	rtest.Assert(t, index >= 0, `expected to find substring "4 / 4 packs", but did not find it`)
}

func TestCheckSimpleFilter1(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, env.testdata+"/0", []string{"for_cmd_ls"}, opts, env.gopts)
	testRunBackup(t, env.testdata+"/0", []string{"0/9"}, opts, env.gopts)

	snapshotIDs := testRunList(t, env.gopts, "snapshots")
	rtest.Assert(t, len(snapshotIDs) == 2, "expected two snapshots, got %v", snapshotIDs)

	output, err := testRunCheckOutputWithOpts(t, env.gopts, CheckOptions{
		ReadData: true}, []string{"latest"})
	rtest.OK(t, err)

	//  find
	// 'read selected data'
	// '2 / 2 packs'
	index := strings.Index(output, "read selected data")
	rtest.Assert(t, index >= 0, `expected to find substring "read selected data", but did not find it`)

	index = strings.Index(output, "2 / 2 packs")
	rtest.Assert(t, index >= 0, `expected to find substring "2 / 2 packs", but did not find it`)

	// proof that exactly one snapshot is used in Structure() - Windows chokes on it.
	//index = strings.Index(output, "1 / 1 snapshots")
	//rtest.Assert(t, index >= 0, `expected to find substring "1 / 1 snapshots", but did not find it`)
}

func TestCheckWithMoreFilter(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, env.testdata+"/0", []string{"for_cmd_ls"}, opts, env.gopts)
	testRunBackup(t, env.testdata+"/0", []string{"0/9"}, opts, env.gopts)

	snapshotIDs := testRunList(t, env.gopts, "snapshots")
	rtest.Assert(t, len(snapshotIDs) == 2, "expected two snapshots, got %v", snapshotIDs)

	output, err := testRunCheckOutputWithOpts(t, env.gopts, CheckOptions{
		ReadDataSubset: "1%"}, []string{"latest"})
	rtest.OK(t, err)

	// find
	// 'selected data'
	// '1 / 1 packs'
	index := strings.Index(output, "selected data")
	rtest.Assert(t, index >= 0, `expected to find substring "selected data", but did not find it`)

	index = strings.Index(output, "1 / 1 packs")
	rtest.Assert(t, index >= 0, `expected to find substring "1 / 1 packs", but did not find it`)
}
