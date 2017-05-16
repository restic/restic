package main

import (
	"path/filepath"
	. "restic/test"
	"testing"
)

func TestRestoreLocalLayout(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
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
			datafile := filepath.Join("..", "..", "restic", "backend", "testdata", test.filename)

			SetupTarTestFixture(t, env.base, datafile)

			gopts.extended["local.layout"] = test.layout

			// check the repo
			testRunCheck(t, gopts)

			// restore latest snapshot
			target := filepath.Join(env.base, "restore")
			testRunRestoreLatest(t, gopts, target, nil, "")

			RemoveAll(t, filepath.Join(env.base, "repo"))
			RemoveAll(t, target)
		}
	})
}
