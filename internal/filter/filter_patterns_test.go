package filter_test

import (
	"testing"

	"github.com/restic/restic/internal/filter"
	rtest "github.com/restic/restic/internal/test"
)

func TestValidPatterns(t *testing.T) {
	// Test invalid patterns are detected and returned
	t.Run("detect-invalid-patterns", func(t *testing.T) {
		err := filter.ValidatePatterns([]string{"*.foo", "*[._]log[.-][0-9]", "!*[._]log[.-][0-9]"})

		rtest.Assert(t, err != nil, "Expected invalid patterns to be detected")

		if ip, ok := err.(*filter.InvalidPatternError); ok {
			rtest.Equals(t, ip.InvalidPatterns, []string{"*[._]log[.-][0-9]", "!*[._]log[.-][0-9]"})
		} else {
			t.Errorf("wrong error type %v", err)
		}
	})

	// Test all patterns defined in matchTests are valid
	patterns := make([]string, 0)

	for _, data := range matchTests {
		patterns = append(patterns, data.pattern)
	}

	t.Run("validate-patterns", func(t *testing.T) {
		err := filter.ValidatePatterns(patterns)

		if err != nil {
			t.Error(err)
		}
	})

	// Test all patterns defined in childMatchTests are valid
	childPatterns := make([]string, 0)

	for _, data := range childMatchTests {
		childPatterns = append(childPatterns, data.pattern)
	}

	t.Run("validate-child-patterns", func(t *testing.T) {
		err := filter.ValidatePatterns(childPatterns)

		if err != nil {
			t.Error(err)
		}
	})
}
