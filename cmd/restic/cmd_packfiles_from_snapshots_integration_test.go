package main

import (
	"context"
	"encoding/json"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func testRunPackfileLIst(t testing.TB, gopts GlobalOptions, opts PackfileListOptions, args []string) []byte {
	buf, err := withCaptureStdout(func() error {
		gopts.Quiet = true
		gopts.JSON = true
		return runPackfileList(context.TODO(), opts, gopts, args)
	})
	rtest.OK(t, err)
	return buf.Bytes()
}

func TestPackfileList(t *testing.T) {
	// setup
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	testSetupBackupData(t, env)

	// backup
	opts := BackupOptions{}
	testRunBackup(t, env.testdata+"/0/0/9", []string{"."}, opts, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	// run packfilelist
	buf := testRunPackfileLIst(t, env.gopts, PackfileListOptions{detail: 2}, []string{"latest"})

	output := &outputStruct{}
	rtest.OK(t, json.Unmarshal(buf, output))
	rtest.Equals(t, output.SummaryInfo.CountTreeFiles, 1, "expected 1 tree packfile")
	rtest.Equals(t, output.SummaryInfo.CountDataFiles, 1, "expected 1 data packfile")
	rtest.Equals(t, output.SummaryInfo.CountPackfiles, 2, "expected 2 packfiles in total")

	for _, pfinfo := range output.PackfileList {
		if pfinfo.Type == "data" {
			rtest.Assert(t, pfinfo.CountUsedBlobs == 69, "expected 69 data blobs, but got %d blobs", pfinfo.CountUsedBlobs)
			rtest.Assert(t, pfinfo.CountAllBlobs == 69, "expected 69 data blobs, but got %d blobs", pfinfo.CountAllBlobs)
		}
	}
}
