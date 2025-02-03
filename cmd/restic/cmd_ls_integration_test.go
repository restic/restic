package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func testRunLsWithOpts(t testing.TB, gopts GlobalOptions, opts LsOptions, args []string) []byte {
	buf, err := withCaptureStdout(func() error {
		gopts.Quiet = true
		return runLs(context.TODO(), opts, gopts, args)
	})
	rtest.OK(t, err)
	return buf.Bytes()
}

func testRunLs(t testing.TB, gopts GlobalOptions, snapshotID string) []string {
	out := testRunLsWithOpts(t, gopts, LsOptions{Sort: "name"}, []string{snapshotID})
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
		ncdu := testRunLsWithOpts(t, env.gopts, LsOptions{Ncdu: true, Sort: "name"}, paths)
		assertIsValidJSON(t, ncdu)
	}
}

func TestRunLsSort(t *testing.T) {
	compareName := []string{
		"/for_cmd_ls",
		"/for_cmd_ls/file1.txt",
		"/for_cmd_ls/file2.txt",
		"/for_cmd_ls/python.py",
		"", // last empty line
	}

	compareSize := []string{
		"/for_cmd_ls",
		"/for_cmd_ls/file2.txt",
		"/for_cmd_ls/file1.txt",
		"/for_cmd_ls/python.py",
		"",
	}

	compareExt := []string{
		"/for_cmd_ls",
		"/for_cmd_ls/python.py",
		"/for_cmd_ls/file1.txt",
		"/for_cmd_ls/file2.txt",
		"",
	}

	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, env.testdata+"/0", []string{"for_cmd_ls"}, opts, env.gopts)

	// sort by size
	out := testRunLsWithOpts(t, env.gopts, LsOptions{Sort: "size"}, []string{"latest"})
	fileList := strings.Split(string(out), "\n")
	rtest.Assert(t, len(fileList) == 5, "invalid ls --sort size, expected 5 array elements, got %v", len(fileList))
	for i, item := range compareSize {
		rtest.Assert(t, item == fileList[i], "invalid ls --sort size, expected element '%s', got '%s'", item, fileList[i])
	}

	// sort by file extension
	out = testRunLsWithOpts(t, env.gopts, LsOptions{Sort: "extension"}, []string{"latest"})
	fileList = strings.Split(string(out), "\n")
	rtest.Assert(t, len(fileList) == 5, "invalid ls --sort extension, expected 5 array elements, got %v", len(fileList))
	for i, item := range compareExt {
		rtest.Assert(t, item == fileList[i], "invalid ls --sort extension, expected element '%s', got '%s'", item, fileList[i])
	}

	// explicit name sort
	out = testRunLsWithOpts(t, env.gopts, LsOptions{Sort: "name"}, []string{"latest"})
	fileList = strings.Split(string(out), "\n")
	rtest.Assert(t, len(fileList) == 5, "invalid ls --sort name, expected 5 array elements, got %v", len(fileList))
	for i, item := range compareName {
		rtest.Assert(t, item == fileList[i], "invalid ls --sort name, expected element '%s', got '%s'", item, fileList[i])
	}
}
