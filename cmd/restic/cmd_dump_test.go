package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/dump"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestDumpSplitPath(t *testing.T) {
	testPaths := []struct {
		path   string
		result []string
	}{
		{"", []string{""}},
		{"test", []string{"test"}},
		{"test/dir", []string{"test", "dir"}},
		{"test/dir/sub", []string{"test", "dir", "sub"}},
		{"/", []string{""}},
		{"/test", []string{"test"}},
		{"/test/dir", []string{"test", "dir"}},
		{"/test/dir/sub", []string{"test", "dir", "sub"}},
	}
	for _, path := range testPaths {
		parts := splitPath(path.path)
		rtest.Equals(t, path.result, parts)
	}
}

func TestPrintFromTreeSkipsUnorderedNodes(t *testing.T) {
	ctx := t.Context()
	repo := repository.TestRepository(t)

	var rootID restic.ID
	rtest.OK(t, repo.WithBlobUploader(ctx, func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		rootID = data.TestSaveUnorderedSampleTree(t, ctx, uploader)
		return nil
	}))

	load := func() data.TreeNodeIterator {
		t.Helper()
		tree, err := data.LoadTree(ctx, repo, rootID)
		rtest.OK(t, err)
		return tree
	}
	dumpFile := func(tree data.TreeNodeIterator, parts []string) (reported []string, err error) {
		t.Helper()
		d := dump.New("tar", repo, &bytes.Buffer{})
		d.Error = func(nodePath string, err error) {
			rtest.Assert(t, errors.Is(err, data.ErrTreeNotOrdered), "unexpected error %v", err)
			reported = append(reported, nodePath)
		}
		err = printFromTree(ctx, tree, repo, "/", parts, d, func() error { return nil })
		return reported, err
	}

	reported, err := dumpFile(load(), []string{"a"})
	rtest.Assert(t, errors.Is(err, data.ErrTreeNotOrdered), "missing unordered file: %v", err)
	rtest.Assert(t, len(reported) == 0, "lookup failure was reported via Error: %v", reported)

	reported, err = dumpFile(load(), []string{"z"})
	rtest.OK(t, err)
	rtest.Assert(t, len(reported) == 0, "in-order match reported unordered siblings: %v", reported)

	reported, err = dumpFile(load(), []string{"missing"})
	rtest.Assert(t, err != nil && !errors.Is(err, data.ErrTreeNotOrdered), "missing path: %v", err)
	rtest.Assert(t, len(reported) == 0, "unrelated unordered nodes were reported: %v", reported)
}
