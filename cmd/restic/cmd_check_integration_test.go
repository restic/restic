package main

import (
	"bytes"
	"context"
	"testing"

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
		return runCheck(context.TODO(), opts, gopts, nil, term)
	})
	return buf.String(), err
}
