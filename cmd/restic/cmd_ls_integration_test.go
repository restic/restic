package main

import (
	"context"
	"encoding/json"
	"path/filepath"
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
	var v interface{}
	err := json.Unmarshal(data, &v)
	rtest.OK(t, err)
}

func TestRunLsNcdu(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)

	for _, paths := range [][]string{
		{"latest"},
		{"latest", "/testdata"},
		{"latest", "/testdata/0", "/testdata/0/tests"},
	} {
		ncdu := testRunLsWithOpts(t, env.gopts, LsOptions{Ncdu: true}, paths)
		assertIsValidJSON(t, ncdu)
	}
}
