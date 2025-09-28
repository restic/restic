package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func testRunLsWithOpts(t testing.TB, gopts global.Options, opts LsOptions, args []string) []byte {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.Quiet = true
		return runLs(context.TODO(), opts, gopts, args, gopts.Term)
	})
	rtest.OK(t, err)
	return buf.Bytes()
}

func testRunLs(t testing.TB, gopts global.Options, snapshotID string) []string {
	out := testRunLsWithOpts(t, gopts, LsOptions{}, []string{snapshotID})
	return strings.Split(string(out), "\n")
}

func assertIsValidJSON(t *testing.T, data []byte) {
	// Sanity check: output must be valid JSON.
	var v []any
	err := json.Unmarshal(data, &v)
	rtest.OK(t, err)
	rtest.Assert(t, len(v) == 4, "invalid ncdu output, expected 4 array elements, got %v", len(v))
}

func TestRunLsNcdu(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	// backup such that there are multiple toplevel elements
	testRunBackup(t, env.testdata+"/0", []string{"."}, opts, env.gopts)

	for _, paths := range [][]string{
		{"latest"},
		{"latest", "/0"},
		{"latest", "/0", "/0/9"},
	} {
		ncdu := testRunLsWithOpts(t, env.gopts, LsOptions{Ncdu: true}, paths)
		assertIsValidJSON(t, ncdu)
	}
}

func TestRunLsSort(t *testing.T) {
	rtest.Equals(t, SortMode(0), SortModeName, "unexpected default sort mode")

	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, env.testdata+"/0", []string{"for_cmd_ls"}, opts, env.gopts)

	for _, test := range []struct {
		mode     SortMode
		expected []string
	}{
		{
			SortModeSize,
			[]string{
				"/for_cmd_ls",
				"/for_cmd_ls/file2.txt",
				"/for_cmd_ls/file1.txt",
				"/for_cmd_ls/python.py",
				"",
			},
		},
		{
			SortModeExt,
			[]string{
				"/for_cmd_ls",
				"/for_cmd_ls/python.py",
				"/for_cmd_ls/file1.txt",
				"/for_cmd_ls/file2.txt",
				"",
			},
		},
		{
			SortModeName,
			[]string{
				"/for_cmd_ls",
				"/for_cmd_ls/file1.txt",
				"/for_cmd_ls/file2.txt",
				"/for_cmd_ls/python.py",
				"", // last empty line
			},
		},
	} {
		out := testRunLsWithOpts(t, env.gopts, LsOptions{Sort: test.mode}, []string{"latest"})
		fileList := strings.Split(string(out), "\n")
		rtest.Equals(t, test.expected, fileList, fmt.Sprintf("mismatch for mode %v", test.mode))
	}
}

// JSON lines test
func TestRunLsJson(t *testing.T) {
	pathList := []string{
		"/0",
		"/0/for_cmd_ls",
		"/0/for_cmd_ls/file1.txt",
		"/0/for_cmd_ls/file2.txt",
		"/0/for_cmd_ls/python.py",
	}

	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, env.testdata, []string{"0/for_cmd_ls"}, opts, env.gopts)
	snapshotIDs := testListSnapshots(t, env.gopts, 1)

	env.gopts.Quiet = true
	env.gopts.JSON = true
	buf := testRunLsWithOpts(t, env.gopts, LsOptions{}, []string{"latest"})
	byteLines := bytes.Split(buf, []byte{'\n'})

	// partial copy of snapshot structure from cmd_ls
	type lsSnapshot struct {
		*data.Snapshot
		ID          *restic.ID `json:"id"`
		ShortID     string     `json:"short_id"`     // deprecated
		MessageType string     `json:"message_type"` // "snapshot"
		StructType  string     `json:"struct_type"`  // "snapshot", deprecated
	}

	var snappy lsSnapshot
	rtest.OK(t, json.Unmarshal(byteLines[0], &snappy))
	rtest.Equals(t, snappy.ShortID, snapshotIDs[0].Str(), "expected snap IDs to be identical")

	// partial copy of node structure from cmd_ls
	type lsNode struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Path        string `json:"path"`
		Permissions string `json:"permissions,omitempty"`
		Inode       uint64 `json:"inode,omitempty"`
		MessageType string `json:"message_type"` // "node"
		StructType  string `json:"struct_type"`  // "node", deprecated
	}

	var testNode lsNode
	for i, nodeLine := range byteLines[1:] {
		if len(nodeLine) == 0 {
			break
		}

		rtest.OK(t, json.Unmarshal(nodeLine, &testNode))
		rtest.Equals(t, pathList[i], testNode.Path)
	}
}
