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
