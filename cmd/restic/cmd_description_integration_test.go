package main

import (
	"context"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func testRunDescription(t testing.TB, description string, gopts GlobalOptions) {
	rtest.OK(t, runDescription(context.TODO(), DescriptionOptions{Description: description}, gopts, nil, []string{}))
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

	// Test removing description
	previousId = *newest.ID
	testRunDescription(t, "", env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}
	rtest.Assert(t, newest.Description == "",
		"removing description failed, still got '%v'", newest.Description)
	rtest.Assert(t, *newest.Original == previousId,
		"expected original ID to be set to the previous snapshot id")

}
