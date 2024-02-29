package main

import (
	"context"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func testRunForget(t testing.TB, gopts GlobalOptions, args ...string) {
	opts := ForgetOptions{}
	pruneOpts := PruneOptions{
		MaxUnused: "5%",
	}
	rtest.OK(t, runForget(context.TODO(), opts, pruneOpts, gopts, args))
}
