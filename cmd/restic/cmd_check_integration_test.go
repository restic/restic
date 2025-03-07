package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/termstatus"
)

func testRunCheck(t testing.TB, gopts GlobalOptions) {
	t.Helper()
	output, err := testRunCheckOutput(gopts, true)
	if err != nil {
		t.Error(output)
		t.Fatalf("unexpected error: %+v", err)
	}
}

func testRunCheckMustFail(t testing.TB, gopts GlobalOptions) {
	t.Helper()
	_, err := testRunCheckOutput(gopts, false)
	rtest.Assert(t, err != nil, "expected non nil error after check of damaged repository")
}

func testRunCheckOutput(gopts GlobalOptions, checkUnused bool) (string, error) {
	buf := bytes.NewBuffer(nil)
	gopts.stdout = buf
	err := withTermStatus(gopts, func(ctx context.Context, term *termstatus.Terminal) error {
		opts := CheckOptions{
			ReadData:    true,
			CheckUnused: checkUnused,
		}
		_, err := runCheck(context.TODO(), opts, gopts, nil, term)
		return err
	})
	return buf.String(), err
}

func testRunCheckOutputWithArgs(gopts GlobalOptions, opts CheckOptions, args []string) (string, error) {
	buf := bytes.NewBuffer(nil)
	gopts.stdout = buf
	err := withTermStatus(gopts, func(ctx context.Context, term *termstatus.Terminal) error {
		_, err := runCheck(context.TODO(), opts, gopts, args, term)
		return err
	})
	return buf.String(), err
}

func TestRunCheckWrongArgs1(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	testSetupBackupData(t, env)

	_, err := testRunCheckOutputWithArgs(env.gopts, CheckOptions{}, []string{"blubber"})
	rtest.Assert(t, err != nil && err.Error() != "",
		// blubber gets quoted - the error string looks messy
		"expected specific error message - got %q", err)
}

func TestRunCheckWrongArgs2(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	testSetupBackupData(t, env)

	opts := CheckOptions{
		SnapshotFilter: restic.SnapshotFilter{Hosts: []string{""}},
	}
	_, err := testRunCheckOutputWithArgs(env.gopts, opts, []string{})
	rtest.Assert(t, err != nil && err.Error() == "snapshotfilter active but no snapshot selected",
		"expected specific error message - got %q", err)
}
