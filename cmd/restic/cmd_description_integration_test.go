package main

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/global"
	rtest "github.com/restic/restic/internal/test"
)

// TODO add tests

func testRunDescription(t testing.TB, description string, gopts global.Options) {
	withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runDescription(context.TODO(), DescriptionOptions{Description: description}, gopts, []string{})
	})
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
	testRunDescription(t, "new description", env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}
	rtest.Assert(t, newest.Description == "new description",
		"changing description failed, expected '%v', got '%v'", "new description", newest.Description)
	rtest.Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
	rtest.Assert(t, *newest.Original == originalId,
		"expected original ID to be set to the first snapshot id")

	// Test editing description
	previousId := *newest.ID
	testRunDescription(t, "edited description", env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}
	rtest.Assert(t, newest.Description == "edited description",
		"changing description failed, expected '%v', got '%v'", "edited description", newest.Description)
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
