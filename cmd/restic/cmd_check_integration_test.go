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

func testRunCheckOutputStderrWithOpts(t testing.TB, gopts global.Options, opts CheckOptions, args []string) (string, string, error) {
	bufStdout, bufStderr, err := withCaptureStdoutStderr(t, gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.Verbosity = 2
		_, err := runCheck(ctx, opts, gopts, args, gopts.Term)
		return err
	})
	return bufStdout.String(), bufStderr.String(), err
}

func TestCheckWithSnaphotFilter(t *testing.T) {
	testCases := []struct {
		opts           CheckOptions
		args           []string
		expectedOutput string
		expectedError  string
		absentMessage  string
	}{
		{ // full --read-data, all snapshots
			CheckOptions{ReadData: true},
			nil,
			"4 / 4 packs",
			"",
			"",
		},
		{ // full --read-data, all snapshots
			CheckOptions{ReadData: true},
			nil,
			"2 / 2 snapshots",
			"",
			"",
		},
		{ // full --read-data, latest snapshot
			CheckOptions{ReadData: true},
			[]string{"latest"},
			"2 / 2 packs",
			"",
			"",
		},
		{ // full --read-data, latest snapshot
			CheckOptions{ReadData: true},
			[]string{"latest"},
			"1 / 1 snapshots",
			"",
			"",
		},
		{ // --read-data-subset, latest snapshot
			CheckOptions{ReadDataSubset: "1%"},
			[]string{"latest"},
			"1 / 1 packs",
			"",
			"",
		},
		{ // --read-data-subset, latest snapshot
			CheckOptions{ReadDataSubset: "1%"},
			[]string{"latest"},
			"filtered",
			"",
			"",
		},
		{ // full --read-data, wrong short ID 1234567890
			CheckOptions{ReadData: true},
			[]string{"1234567890"},
			"no errors were found",
			"no matching ID found for prefix",
			"restic repair snapshots",
		},
		{ // full --read-data, wrong snapshot ID 1d20477115fb872069a28a80ffb95a82cb8b1b1920de046a68c0195da63f30ca
			CheckOptions{ReadData: true},
			[]string{"1d20477115fb872069a28a80ffb95a82cb8b1b1920de046a68c0195da63f30ca"},
			"no errors were found",
			"no such file or directory, ignored",
			"restic repair snapshots",
		},
	}

	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, env.testdata+"/0", []string{"for_cmd_ls"}, opts, env.gopts)
	testRunBackup(t, env.testdata+"/0", []string{"0/9"}, opts, env.gopts)

	for _, testCase := range testCases {
		stdout, stderr, err := testRunCheckOutputStderrWithOpts(t, env.gopts, testCase.opts, testCase.args)
		rtest.OK(t, err)
		rtest.Assert(t, strings.Contains(stdout, testCase.expectedOutput),
			`expected to find substring %q, but found %q`,
			testCase.expectedOutput, stdout)
		if testCase.expectedError != "" {
			rtest.Assert(t, strings.Contains(stderr, testCase.expectedError),
				`expected to find substring %q, but found %q`,
				testCase.expectedError, stderr)
		}
		if testCase.absentMessage != "" {
			rtest.Assert(t, !strings.Contains(stderr, testCase.absentMessage),
				`NOT expected to find substring %q, but found %q`,
				testCase.absentMessage, stderr)
		}
	}
}
