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
