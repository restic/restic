package main

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func testRunTag(t testing.TB, opts TagOptions, gopts GlobalOptions) {
	rtest.OK(t, runTag(context.TODO(), opts, gopts, []string{}))
}

// nolint: staticcheck // false positive nil pointer dereference check
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

	testRunTag(t, TagOptions{SetTags: restic.TagLists{[]string{"NL"}}}, env.gopts)
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

	testRunTag(t, TagOptions{AddTags: restic.TagLists{[]string{"CH"}}}, env.gopts)
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

	testRunTag(t, TagOptions{RemoveTags: restic.TagLists{[]string{"NL"}}}, env.gopts)
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

	testRunTag(t, TagOptions{AddTags: restic.TagLists{[]string{"US", "RU"}}}, env.gopts)
	testRunTag(t, TagOptions{RemoveTags: restic.TagLists{[]string{"CH", "US", "RU"}}}, env.gopts)
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
	testRunTag(t, TagOptions{SetTags: restic.TagLists{[]string{""}}}, env.gopts)
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
