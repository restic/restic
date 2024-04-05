package main

import (
	"context"
	"testing"

	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/termstatus"
)

func testRunForget(t testing.TB, gopts GlobalOptions, args ...string) {
	opts := ForgetOptions{}
	pruneOpts := PruneOptions{
		MaxUnused: "5%",
	}
	rtest.OK(t, withTermStatus(gopts, func(ctx context.Context, term *termstatus.Terminal) error {
		return runForget(context.TODO(), opts, pruneOpts, gopts, term, args)
	}))
}
