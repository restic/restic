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
	stdout, stderr, err := testRunCheckOutput(t, gopts, true)
	if err != nil {
		t.Error(stdout)
		t.Error(stderr)
		t.Fatalf("unexpected error: %+v", err)
	}
}

func testRunCheckMustFail(t testing.TB, gopts global.Options) {
	t.Helper()
	_, _, err := testRunCheckOutput(t, gopts, false)
	rtest.Assert(t, err != nil, "expected non nil error after check of damaged repository")
}

func testRunCheckOutput(t testing.TB, gopts global.Options, checkUnused bool) (string, string, error) {
	stdout, stderr, err := withCaptureStdoutStderr(t, gopts, func(ctx context.Context, gopts global.Options) error {
		opts := CheckOptions{
			ReadData:    true,
			CheckUnused: checkUnused,
		}
		_, err := runCheck(ctx, opts, gopts, nil, gopts.Term)
		return err
	})
	return stdout.String(), stderr.String(), err
}

func testRunCheckOutputWithOpts(t testing.TB, gopts global.Options, opts CheckOptions, args []string) (string, error) {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.Verbosity = 2
		_, err := runCheck(ctx, opts, gopts, args, gopts.Term)
		return err
	})
	return buf.String(), err
}

func TestCheckWithSnaphotFilter(t *testing.T) {
	testCases := []struct {
		opts           CheckOptions
		args           []string
		expectedOutput string
	}{
		{ // full --read-data, all snapshots
			CheckOptions{ReadData: true},
			nil,
			"4 / 4 packs",
		},
		{ // full --read-data, all snapshots
			CheckOptions{ReadData: true},
			nil,
			"2 / 2 snapshots",
		},
		{ // full --read-data, latest snapshot
			CheckOptions{ReadData: true},
			[]string{"latest"},
			"2 / 2 packs",
		},
		{ // full --read-data, latest snapshot
			CheckOptions{ReadData: true},
			[]string{"latest"},
			"1 / 1 snapshots",
		},
		{ // --read-data-subset, latest snapshot
			CheckOptions{ReadDataSubset: "1%"},
			[]string{"latest"},
			"1 / 1 packs",
		},
		{ // --read-data-subset, latest snapshot
			CheckOptions{ReadDataSubset: "1%"},
			[]string{"latest"},
			"filtered",
		},
	}

	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, env.testdata+"/0", []string{"for_cmd_ls"}, opts, env.gopts)
	testRunBackup(t, env.testdata+"/0", []string{"0/9"}, opts, env.gopts)

	for _, testCase := range testCases {
		output, err := testRunCheckOutputWithOpts(t, env.gopts, testCase.opts, testCase.args)
		rtest.OK(t, err)

		hasOutput := strings.Contains(output, testCase.expectedOutput)
		rtest.Assert(t, hasOutput, `expected to find substring %q, but did not find it`, testCase.expectedOutput)
	}
}

func TestCheckFlags(t *testing.T) {
	tests := []struct {
		name    string
		opts    CheckOptions
		wantErr string // substring of expected error, "" for no error
	}{
		{
			name: "empty opts valid",
			opts: CheckOptions{},
		},
		{
			name:    "read-data-subset with read-data",
			opts:    CheckOptions{ReadData: true, ReadDataSubset: "20%"},
			wantErr: "--read-data and --read-data-subset cannot be used together",
		},
		{
			name:    "bad subset format 1",
			opts:    CheckOptions{ReadDataSubset: "abc"},
			wantErr: "--read-data-subset has invalid value",
		},
		{
			name:    "bad subset format 2",
			opts:    CheckOptions{ReadDataSubset: "1/512"},
			wantErr: "--read-data-subset=n/t t must be at most 256",
		},
		{
			name:    "bad subset format 3",
			opts:    CheckOptions{ReadDataSubset: "17/12"},
			wantErr: "--read-data-subset=n/t values must be positive integers, and n <= t",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkFlags(tc.opts)
			if tc.wantErr == "" {
				rtest.OK(t, err)
				return
			}
			rtest.Assert(t, err != nil, "expected an error, got nil")
			rtest.Assert(t, strings.Contains(err.Error(), tc.wantErr),
				"unexpected error: %v", err)
		})
	}
}
