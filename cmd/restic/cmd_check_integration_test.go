package main

import (
	"context"
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
