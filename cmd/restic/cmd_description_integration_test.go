package main

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/global"
	rtest "github.com/restic/restic/internal/test"
)

// TODO add tests

func testRunDescription(t testing.TB, description string, gopts global.Options) {
	testRunDescriptionWithArgs(t, description, gopts, []string{"latest"})
}

func testRunDescriptionWithArgs(t testing.TB, description string, gopts global.Options, args []string) {
	err := withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runDescription(context.TODO(), changeDescriptionOptions{descriptionOptions: descriptionOptions{Description: description}}, gopts, args)
	})
	rtest.OK(t, err)
}

func testRunDescriptionRemove(t testing.TB, gopts global.Options) {
	err := withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runDescription(context.TODO(), changeDescriptionOptions{removeDescription: true}, gopts, []string{"latest"})
	})
	rtest.OK(t, err)
}

func TestDescription(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ := testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a new backup, got nil")
	}

	rtest.Assert(t, newest.Description == "",
		"expected no description, got '%v'", newest.Description)
	rtest.Assert(t, newest.Original == nil,
		"expected original ID to be nil, got %v", newest.Original)
	originalId := *newest.ID

	// Test adding a description
	const newDescription = "new description"
	testRunDescription(t, newDescription, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}
	rtest.Assert(t, newest.Description == newDescription,
		"changing description failed, expected '%v', got '%v'", newDescription, newest.Description)
	rtest.Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
	rtest.Assert(t, *newest.Original == originalId,
		"expected original ID to be set to the first snapshot id")

	// Test editing description
	const editDescription = "edited description"
	previousId := *newest.ID
	testRunDescription(t, editDescription, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}
	rtest.Assert(t, newest.Description == editDescription,
		"changing description failed, expected '%v', got '%v'", editDescription, newest.Description)
	rtest.Assert(t, *newest.Original == previousId,
		"expected original ID to be set to the previous snapshot id")

	// Editing with same description should not create a new snapshot
	previousId = *newest.ID
	testRunDescription(t, editDescription, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}
	rtest.Assert(t, newest.Description == editDescription,
		"description changed, expected '%v', got '%v'", editDescription, newest.Description)
	rtest.Assert(t, *newest.ID == previousId,
		"snapshot id changed, expected %q, got %q", previousId, *newest.ID)

	// Test removing description
	previousId = *newest.ID
	testRunDescriptionRemove(t, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}
	rtest.Assert(t, newest.Description == "",
		"removing description failed, still got '%v'", newest.Description)
	rtest.Assert(t, *newest.Original == previousId,
		"expected original ID to be set to the previous snapshot id")

	// Test editing multiple descriptions at once
	// Create second snapshot
	first := newest
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	second, _ := testRunSnapshots(t, env.gopts)
	expectedThirdDescription := "third snapshot description which should not be changed"
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{DescriptionOptions: descriptionOptions{Description: expectedThirdDescription}}, env.gopts)
	third, _ := testRunSnapshots(t, env.gopts)
	rtest.Assert(t, *second.Parent == *first.ID, "expected second snapshot to have third snapshot as parent")
	rtest.Assert(t, *third.Parent == *second.ID, "expected third snapshot to have second snapshot as parent")
	// Assert unchanged descriptions
	rtest.Assert(t, first.Description == "", "expected empty description in first snapshot, got %s", first.Description)
	rtest.Assert(t, second.Description == "", "expected empty description in second snapshot, got %s", second.Description)
	rtest.Assert(t, third.Description == expectedThirdDescription,
		"expected description %s in third snapshot, got %s", expectedThirdDescription, third.Description)
	// Change description of first and second snapshot at once
	expectedChangedDescription := "This is a new description for the first and second snapshot."
	testRunDescriptionWithArgs(t, expectedChangedDescription, env.gopts, []string{first.ShortID, second.ShortID})
	// Assert changed description in first and second snapshot, unchanged description in third snapshot
	_, snapmap := testRunSnapshots(t, env.gopts)
	var changedFirst, changedSecond Snapshot
	for id, snapshot := range snapmap {
		if id == *third.ID {
			// Third snapshot should not change an therefor has no original ID
			continue
		}
		switch *snapshot.Original {
		case *first.ID:
			changedFirst = snapshot
		case *second.ID:
			changedSecond = snapshot
		default:
			t.Fatalf("Unexpected snapshot %v with original %v", snapshot, snapshot.Original)
		}
	}

	rtest.Assert(t, changedFirst.Description == expectedChangedDescription,
		"Expected description '%s' in first snapshot, got '%s'", expectedChangedDescription, changedFirst.Description)
	rtest.Assert(t, changedSecond.Description == expectedChangedDescription,
		"Expected description '%s' in second snapshot, got '%s'", expectedChangedDescription, changedSecond.Description)
	rtest.Assert(t, third.Description == expectedThirdDescription,
		"Expected description '%s' in third snapshot, got '%s'", expectedChangedDescription, third.Description)
}
