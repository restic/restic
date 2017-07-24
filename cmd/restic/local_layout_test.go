package main

import (
	"path/filepath"
	"testing"

	. "github.com/restic/restic/internal/test"
)

func TestRestoreLocalLayout(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	var tests = []struct {
		filename string
		layout   string
	}{
		{"repo-layout-default.tar.gz", ""},
		{"repo-layout-s3legacy.tar.gz", ""},
		{"repo-layout-default.tar.gz", "default"},
		{"repo-layout-s3legacy.tar.gz", "s3legacy"},
	}

	for _, test := range tests {
		datafile := filepath.Join("..", "..", "internal", "backend", "testdata", test.filename)

		SetupTarTestFixture(t, env.base, datafile)

		env.gopts.extended["local.layout"] = test.layout

		// check the repo
		testRunCheck(t, env.gopts)

		// restore latest snapshot
		target := filepath.Join(env.base, "restore")
		testRunRestoreLatest(t, env.gopts, target, nil, "")

		RemoveAll(t, filepath.Join(env.base, "repo"))
		RemoveAll(t, target)
	}
}
