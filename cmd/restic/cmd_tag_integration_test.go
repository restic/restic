package main

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/progress"
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

func TestTagKeepOriginal(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	originalID := testListSnapshots(t, env.gopts, 1)[0]

	// --keep-original only needs a non-exclusive lock, thus it must not conflict with other non-exclusive locks
	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, _, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		err = runTag(ctx, TagOptions{AddTags: data.TagLists{[]string{"NL"}}}, gopts, gopts.Term, []string{})
		rtest.Assert(t, repository.IsAlreadyLocked(err), "expected already locked error, got %v", err)

		return runTag(ctx, TagOptions{AddTags: data.TagLists{[]string{"NL"}}, KeepOriginal: true}, gopts, gopts.Term, []string{})
	})
	rtest.OK(t, err)
	testRunCheck(t, env.gopts)

	snapshotIDs := testListSnapshots(t, env.gopts, 2)
	var newID restic.ID
	for _, id := range snapshotIDs {
		if id != originalID {
			newID = id
		}
	}

	original := testLoadSnapshot(t, env.gopts, originalID)
	rtest.Assert(t, len(original.Tags) == 0,
		"expected no tags on original snapshot, got %v", original.Tags)

	newSn := testLoadSnapshot(t, env.gopts, newID)
	rtest.Assert(t, len(newSn.Tags) == 1 && newSn.Tags[0] == "NL",
		"add failed, expected one NL tag, got %v", newSn.Tags)
	rtest.Assert(t, newSn.Original != nil && *newSn.Original == originalID,
		"expected original ID to be set to the first snapshot id, got %v", newSn.Original)
}
