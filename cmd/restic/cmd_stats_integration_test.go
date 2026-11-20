package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/global"
	rtest "github.com/restic/restic/internal/test"
)

func TestStatsBlobsPerFile(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	testRunInit(t, env.gopts)

	contents := []byte("shared contents\n")
	for _, name := range []string{"first.txt", "second.txt"} {
		rtest.OK(t, os.WriteFile(filepath.Join(env.testdata, name), contents, 0600))
	}
	opts := BackupOptions{Host: "test-host"}
	testRunBackup(t, env.testdata, []string{"."}, opts, env.gopts)
	// A second snapshot reuses the same paths and blobs.
	testRunBackup(t, env.testdata, []string{"."}, opts, env.gopts)

	env.gopts.JSON = true
	output, err := withCaptureStdout(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runStats(ctx, StatsOptions{countMode: countModeBlobsPerFile}, gopts, nil, gopts.Term)
	})
	rtest.OK(t, err)

	var stats statsContainer
	rtest.OK(t, json.Unmarshal(output.Bytes(), &stats))
	rtest.Equals(t, 2, stats.SnapshotsCount)
	rtest.Equals(t, uint64(2), stats.TotalFileCount)
	rtest.Equals(t, uint64(2), stats.TotalBlobCount)
	rtest.Equals(t, uint64(2*len(contents)), stats.TotalSize)
}
