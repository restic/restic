package main

import (
	"slices"
	"testing"

	"github.com/restic/restic/internal/backend/all"
	rtest "github.com/restic/restic/internal/test"
)

func TestRedactLoggedArgs(t *testing.T) {
	registry := all.Backends()
	args := []string{
		"restic",
		"-r",
		"rest:https://user:secret@example.com/repo",
		"--repo=rest:https://user:secret@example.com/repo",
		"backup",
		"/srv/data",
	}
	copy := slices.Clone(args)
	got := redactLoggedArgs(args, registry)
	rtest.Equals(t, []string{
		"restic",
		"-r",
		"rest:https://user:***@example.com/repo/",
		"--repo=rest:https://user:***@example.com/repo/",
		"backup",
		"/srv/data",
	}, got)
	// ensure that the original arguments are not modified
	rtest.Equals(t, copy, args)
}
