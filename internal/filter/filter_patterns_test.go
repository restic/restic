//go:build go1.16
// +build go1.16

// Before Go 1.16 filepath.Match returned early on a failed match,
// and thus did not report any later syntax error in the pattern.
// https://go.dev/doc/go1.16#path/filepath

package filter_test

import (
	"strings"
	"testing"

	"github.com/restic/restic/internal/filter"
	rtest "github.com/restic/restic/internal/test"
)

func TestValidPatterns(t *testing.T) {
	// Test invalid patterns are detected and returned
	t.Run("detect-invalid-patterns", func(t *testing.T) {
		allValid, invalidPatterns := filter.ValidatePatterns([]string{"*.foo", "*[._]log[.-][0-9]", "!*[._]log[.-][0-9]"})

		rtest.Assert(t, allValid == false, "Expected invalid patterns to be detected")

		rtest.Equals(t, invalidPatterns, []string{"*[._]log[.-][0-9]", "!*[._]log[.-][0-9]"})
	})

	// Test all patterns defined in matchTests are valid
	patterns := make([]string, 0)

	for _, data := range matchTests {
		patterns = append(patterns, data.pattern)
	}

	t.Run("validate-patterns", func(t *testing.T) {
		allValid, invalidPatterns := filter.ValidatePatterns(patterns)

		if !allValid {
			t.Errorf("Found invalid pattern(s):\n%s", strings.Join(invalidPatterns, "\n"))
		}
	})

	// Test all patterns defined in childMatchTests are valid
	childPatterns := make([]string, 0)

	for _, data := range childMatchTests {
		childPatterns = append(childPatterns, data.pattern)
	}

	t.Run("validate-child-patterns", func(t *testing.T) {
		allValid, invalidPatterns := filter.ValidatePatterns(childPatterns)

		if !allValid {
			t.Errorf("Found invalid child pattern(s):\n%s", strings.Join(invalidPatterns, "\n"))
		}
	})
}
