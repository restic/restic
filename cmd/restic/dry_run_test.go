package main

import (
	"os"
	"testing"
)

func Test_IsDryRun(t *testing.T) {

	defer os.Unsetenv("RESTIC_DRY_RUN")

	os.Unsetenv("RESTIC_DRY_RUN")
	if isDryRunByEnv(true) != true {
		t.Fatal("expected 'true', got 'false'")
	}
	if isDryRunByEnv(false) != false {
		t.Fatal("expected 'false', got 'true'")
	}

	os.Setenv("RESTIC_DRY_RUN", "")
	if isDryRunByEnv(true) != true {
		t.Fatal("expected 'true', got 'false'")
	}
	if isDryRunByEnv(false) != true {
		t.Fatal("expected 'true', got 'false'")
	}

	os.Setenv("RESTIC_DRY_RUN", "true")
	if isDryRunByEnv(true) != true {
		t.Fatal("expected 'true', got 'false'")
	}
	if isDryRunByEnv(false) != true {
		t.Fatal("expected 'true', got 'false'")
	}
}
