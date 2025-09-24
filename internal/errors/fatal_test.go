package errors_test

import (
	"testing"

	"github.com/restic/restic/internal/errors"
)

func TestFatal(t *testing.T) {
	for _, v := range []struct {
		err      error
		expected bool
	}{
		{errors.Fatal("broken"), true},
		{errors.Fatalf("broken %d", 42), true},
		{errors.New("error"), false},
	} {
		if errors.IsFatal(v.err) != v.expected {
			t.Fatalf("IsFatal for %q, expected: %v, got: %v", v.err, v.expected, errors.IsFatal(v.err))
		}
	}
}

func TestFatalErrorWrapping(t *testing.T) {
	underlying := errors.New("underlying error")
	fatal := errors.Fatalf("fatal error: %v", underlying)

	// Test that the fatal error message is preserved
	if fatal.Error() != "Fatal: fatal error: underlying error" {
		t.Errorf("unexpected error message: %v", fatal.Error())
	}

	// Test that we can unwrap to get the underlying error
	if !errors.Is(fatal, underlying) {
		t.Error("fatal error should wrap the underlying error")
	}

	// Test that the error is marked as fatal
	if !errors.IsFatal(fatal) {
		t.Error("error should be marked as fatal")
	}
}
