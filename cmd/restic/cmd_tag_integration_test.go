package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	rtest "github.com/restic/restic/internal/test"
)

func testRunTag(t testing.TB, opts TagOptions, gopts global.Options) {
	rtest.OK(t, withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runTag(context.TODO(), opts, gopts, gopts.Term, []string{})
	}))
}

func TestTag(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ := testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a new backup, got nil")
	}

	rtest.Assert(t, len(newest.Tags) == 0,
		"expected no tags, got %v", newest.Tags)
	rtest.Assert(t, newest.Original == nil,
		"expected original ID to be nil, got %v", newest.Original)
	originalID := *newest.ID

	testRunTag(t, TagOptions{SetTags: data.TagLists{[]string{"NL"}}}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}
	rtest.Assert(t, len(newest.Tags) == 1 && newest.Tags[0] == "NL",
		"set failed, expected one NL tag, got %v", newest.Tags)
	rtest.Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
	rtest.Assert(t, *newest.Original == originalID,
		"expected original ID to be set to the first snapshot id")

	testRunTag(t, TagOptions{AddTags: data.TagLists{[]string{"CH"}}}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}
	rtest.Assert(t, len(newest.Tags) == 2 && newest.Tags[0] == "NL" && newest.Tags[1] == "CH",
		"add failed, expected CH,NL tags, got %v", newest.Tags)
	rtest.Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
	rtest.Assert(t, *newest.Original == originalID,
		"expected original ID to be set to the first snapshot id")

	testRunTag(t, TagOptions{RemoveTags: data.TagLists{[]string{"NL"}}}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}
	rtest.Assert(t, len(newest.Tags) == 1 && newest.Tags[0] == "CH",
		"remove failed, expected one CH tag, got %v", newest.Tags)
	rtest.Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
	rtest.Assert(t, *newest.Original == originalID,
		"expected original ID to be set to the first snapshot id")

	testRunTag(t, TagOptions{AddTags: data.TagLists{[]string{"US", "RU"}}}, env.gopts)
	testRunTag(t, TagOptions{RemoveTags: data.TagLists{[]string{"CH", "US", "RU"}}}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}
	rtest.Assert(t, len(newest.Tags) == 0,
		"expected no tags, got %v", newest.Tags)
	rtest.Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
	rtest.Assert(t, *newest.Original == originalID,
		"expected original ID to be set to the first snapshot id")

	// Check special case of removing all tags.
	testRunTag(t, TagOptions{SetTags: data.TagLists{[]string{""}}}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}
	rtest.Assert(t, len(newest.Tags) == 0,
		"expected no tags, got %v", newest.Tags)
	rtest.Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
	rtest.Assert(t, *newest.Original == originalID,
		"expected original ID to be set to the first snapshot id")
}

func TestTagPartialSnapshotSelection(t *testing.T) {
	for _, pos := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("missing-at-%d", pos), func(t *testing.T) {
			env, cleanup := withTestEnvironment(t)
			defer cleanup()
			testSetupBackupData(t, env)
			testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "2")}, BackupOptions{}, env.gopts)
			testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "3")}, BackupOptions{}, env.gopts)
			ids := testListSnapshots(t, env.gopts, 2)
			args := []string{ids[0].String(), ids[1].String()}
			args = append(args[:pos], append([]string{"missing-snapshot"}, args[pos:]...)...)
			opts := TagOptions{AddTags: data.TagLists{[]string{"selected"}}}
			err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
				return runTag(ctx, opts, gopts, gopts.Term, args)
			})
			rtest.Assert(t, err != nil, "tag must report the missing snapshot")
			rtest.Assert(t, strings.Contains(err.Error(), "missing-snapshot"), "error must identify the missing snapshot: %v", err)
			_, snapshots := testRunSnapshots(t, env.gopts)
			rtest.Equals(t, 2, len(snapshots))
			for _, snapshot := range snapshots {
				rtest.Equals(t, []string{"selected"}, snapshot.Tags)
			}
			testRunCheck(t, env.gopts)
		})
	}
}

type tagSaveFailureBackend struct {
	backend.Backend
	err    error
	failed bool
}

func (b *tagSaveFailureBackend) Save(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
	if h.Type == backend.SnapshotFile && !b.failed {
		b.failed = true
		return b.err
	}
	return b.Backend.Save(ctx, h, rd)
}

func TestTagPartialSaveFailure(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "2")}, BackupOptions{}, env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "3")}, BackupOptions{}, env.gopts)
	ids := testListSnapshots(t, env.gopts, 2)
	failure := errors.New("snapshot write failed")
	gopts := env.gopts
	gopts.BackendTestHook = func(be backend.Backend) (backend.Backend, error) {
		return &tagSaveFailureBackend{Backend: be, err: failure}, nil
	}
	err := withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runTag(ctx, TagOptions{AddTags: data.TagLists{[]string{"selected"}}}, gopts, gopts.Term,
			[]string{ids[0].String(), "missing-snapshot", ids[1].String()})
	})
	rtest.Assert(t, errors.Is(err, failure), "must preserve the save failure: %v", err)
	rtest.Assert(t, strings.Contains(err.Error(), ids[0].String()), "error must identify the failed snapshot: %v", err)
	rtest.Assert(t, strings.Contains(err.Error(), "missing-snapshot"), "must retain selection failures too: %v", err)
	_, snapshots := testRunSnapshots(t, env.gopts)
	rtest.Equals(t, 2, len(snapshots))
	rtest.Equals(t, []string(nil), snapshots[ids[0]].Tags)
	for id, sn := range snapshots {
		if id != ids[0] {
			rtest.Equals(t, []string{"selected"}, sn.Tags)
		}
	}
	testRunCheck(t, env.gopts)
}
