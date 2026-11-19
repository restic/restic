package main

import (
	"testing"

	"github.com/restic/restic/internal/filter"
	rtest "github.com/restic/restic/internal/test"
)

func TestRestoreSelectFilter(t *testing.T) {
	type result struct {
		selected, childMayBeSelected bool
	}

	for _, test := range []struct {
		name     string
		includes []string
		excludes []string
		item     string
		isDir    bool
		want     result
	}{
		{"none/file", nil, nil, "/a/file", false, result{true, false}},
		{"none/dir", nil, nil, "/a", true, result{true, true}},

		{"exclude/matched", nil, []string{"/a/b"}, "/a/b", true, result{false, false}},
		{"exclude/child", nil, []string{"/a/b"}, "/a/b/c", false, result{false, false}},
		{"exclude/sibling", nil, []string{"/a/b"}, "/a/c", true, result{true, true}},

		{"include/parent", []string{"/a/b"}, nil, "/a", true, result{false, true}},
		{"include/matched", []string{"/a/b"}, nil, "/a/b", true, result{true, true}},
		{"include/child", []string{"/a/b"}, nil, "/a/b/c", false, result{true, false}},
		{"include/sibling", []string{"/a/b"}, nil, "/a/c", true, result{false, false}},

		{"both/parent", []string{"/a/b"}, []string{"/a/b/c"}, "/a", true, result{false, true}},
		{"both/included", []string{"/a/b"}, []string{"/a/b/c"}, "/a/b/d", false, result{true, false}},
		{"both/excluded", []string{"/a/b"}, []string{"/a/b/c"}, "/a/b/c", true, result{false, false}},
		{"both/excluded-child", []string{"/a/b"}, []string{"/a/b/c"}, "/a/b/c/d", false, result{false, false}},
		{"both/sibling", []string{"/a/b"}, []string{"/a/b/c"}, "/a/c", true, result{false, false}},
		{"both/exclude-outside-include", []string{"/a/b"}, []string{"/a/c"}, "/a/c", true, result{false, false}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var includeFns []filter.IncludeByNameFunc
			if test.includes != nil {
				includeFns = append(includeFns, filter.IncludeByPattern(test.includes, t.Logf))
			}
			var excludeFns []filter.RejectByNameFunc
			if test.excludes != nil {
				excludeFns = append(excludeFns, filter.RejectByPattern(test.excludes, t.Logf))
			}

			selected, childMayBeSelected := newRestoreSelectFilter(includeFns, excludeFns)(test.item, test.isDir)
			rtest.Equals(t, test.want, result{selected, childMayBeSelected})
		})
	}
}
