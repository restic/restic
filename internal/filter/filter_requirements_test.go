package filter_test

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/restic/restic/internal/filter"
)

// referenceList evaluates patterns one at a time with Match (and ChildMatch
// when withChild is set, as ListWithChild does), which do not use the
// requirements List checks first.
func referenceList(patterns []string, str string, withChild bool) (matched bool, childMayMatch bool, err error) {
	hasNegated := false
	for _, p := range patterns {
		hasNegated = hasNegated || strings.HasPrefix(p, "!")
	}
	for _, p := range patterns {
		negated := strings.HasPrefix(p, "!")
		p = strings.TrimPrefix(p, "!")
		m, err := filter.Match(p, str)
		if err != nil {
			return false, false, err
		}
		// list computes the child match of negated patterns too.
		c := true
		if withChild {
			c, err = filter.ChildMatch(p, str)
			if err != nil {
				return false, false, err
			}
		}
		if negated {
			matched = matched && !m
			childMayMatch = childMayMatch && !m
			continue
		}
		matched = matched || m
		childMayMatch = childMayMatch || c
		if matched && childMayMatch && !hasNegated {
			break
		}
	}
	return matched, childMayMatch, nil
}

// List skips a pattern when the path lacks text every match needs. The
// result must be the same as matching every pattern, for any patterns.
func TestListRequirementsKeepResults(t *testing.T) {
	names := []string{"a", "b", "foo", "foo.txt", "bar.tmp", "node_modules", "Cache", "x.cache", "$RECYCLE.BIN", "sdk", "cpp"}
	globs := []string{"*", "**", "*.tmp", "*cache*", "*Cache", "f?o*", "[ab]", "?", "*.txt", "*$RECYCLE.BIN", "["}
	r := rand.New(rand.NewSource(1))
	part := func() string {
		if r.Intn(3) == 0 {
			return globs[r.Intn(len(globs))]
		}
		return names[r.Intn(len(names))]
	}
	pattern := func() string {
		parts := make([]string, 1+r.Intn(4))
		for i := range parts {
			parts[i] = part()
		}
		p := strings.Join(parts, "/")
		if r.Intn(3) == 0 {
			p = "/" + p
		}
		if r.Intn(6) == 0 {
			p = "!" + p
		}
		return p
	}
	path := func() string {
		parts := make([]string, 1+r.Intn(8))
		for i := range parts {
			parts[i] = names[r.Intn(len(names))]
		}
		return "/" + strings.Join(parts, "/")
	}

	for round := 0; round < 2000; round++ {
		patterns := make([]string, 1+r.Intn(8))
		for i := range patterns {
			patterns[i] = pattern()
		}
		parsed := filter.ParsePatterns(patterns)
		for i := 0; i < 50; i++ {
			str := path()
			wantMatch, wantChild, wantErr := referenceList(patterns, str, true)
			gotMatch, gotChild, gotErr := filter.ListWithChild(parsed, str)
			if (gotErr != nil) != (wantErr != nil) || gotMatch != wantMatch || gotChild != wantChild {
				t.Fatalf("patterns %q, path %q: got (%v, %v, %v), want (%v, %v, %v)",
					patterns, str, gotMatch, gotChild, gotErr, wantMatch, wantChild, wantErr)
			}
			wantMatch, _, wantErr = referenceList(patterns, str, false)
			listMatch, listErr := filter.List(parsed, str)
			if (listErr != nil) != (wantErr != nil) || listMatch != wantMatch {
				t.Fatalf("List: patterns %q, path %q: got (%v, %v), want (%v, %v)", patterns, str, listMatch, listErr, wantMatch, wantErr)
			}
		}
	}
}

// Rules as found in real Windows exclude lists: most name a folder, many
// use "**", and nearly every path matches none of them.
func BenchmarkFilterManyRules(b *testing.B) {
	lines := extractTestLines(b)
	var patterns []string
	for _, dir := range []string{"node_modules", "Cache", "GPUCache", "ShaderCache", "npm-cache", "__pycache__", "Temp", "Logs", "CrashDumps", "INetCache"} {
		patterns = append(patterns, "**/"+dir+"/", "/home/**/"+dir+"/*", "/usr/*/**/"+dir+"/*")
	}
	for _, ext := range []string{"*.tmp", "*.temp", "*.dmp", "*.crdownload", "*~", "*.swp", "*.bak", "*.log"} {
		patterns = append(patterns, ext, "/usr/**/"+ext)
	}
	parsed := filter.ParsePatterns(patterns)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, line := range lines {
			if _, err := filter.List(parsed, line); err != nil {
				b.Fatal(err)
			}
		}
	}
}
