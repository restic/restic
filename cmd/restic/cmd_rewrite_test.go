package main

import (
	"strings"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestReadDescription(t *testing.T) {
	wantDescription := "This is a short test description."
	opts := descriptionOptions{
		Description: wantDescription,
	}
	gotDescription, err := readDescription(opts)

	rtest.OK(t, err)
	rtest.Assert(t, wantDescription == gotDescription, "Wanted '%s' description, got '%s'", wantDescription, gotDescription)
}

func TestReadTooLargeDescription(t *testing.T) {

	createDescription := func(t *testing.T, length int) string {
		t.Helper()

		builder := strings.Builder{}
		for range length {
			builder.WriteString("a")
		}
		description := builder.String()
		if len(description) != maxDescriptionLength+1 {
			t.Errorf("createDescription test function failed: expected len %d, got len %d", maxDescriptionLength+1, len(description))
		}
		return description
	}

	description := createDescription(t, maxDescriptionLength+1)
	_, err := readDescription(descriptionOptions{
		Description: description,
	})
	rtest.Assert(t, err == descriptionTooLargeErr, "Expected readDescription to return descriptionTooLargeError, got %v", err)
}
