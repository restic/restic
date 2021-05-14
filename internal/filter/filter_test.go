package filter_test

import (
	"bufio"
	"compress/bzip2"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restic/restic/internal/filter"
)

var matchTests = []struct {
	pattern string
	path    string
	match   bool
}{
	{"", "", true},
	{"", "foo", true},
	{"", "/x/y/z/foo", true},
	{"*.go", "/foo/bar/test.go", true},
	{"*.c", "/foo/bar/test.go", false},
	{"*", "/foo/bar/test.go", true},
	{"**", "/foo/bar/test.go", true},
	{"foo*", "/foo/bar/test.go", true},
	{"bar*", "/foo/bar/test.go", true},
	{"/bar*", "/foo/bar/test.go", false},
	{"bar/*", "/foo/bar/test.go", true},
	{"baz/*", "/foo/bar/test.go", false},
	{"bar/test.go", "/foo/bar/test.go", true},
	{"bar/*.go", "/foo/bar/test.go", true},
	{"ba*/*.go", "/foo/bar/test.go", true},
	{"bb*/*.go", "/foo/bar/test.go", false},
	{"test.*", "/foo/bar/test.go", true},
	{"tesT.*", "/foo/bar/test.go", false},
	{"bar/*", "/foo/bar/baz", true},
	{"bar", "/foo/bar", true},
	{"/foo/bar", "/foo/bar", true},
	{"/foo/bar/", "/foo/bar", true},
	{"/foo/bar", "/foo/baz", false},
	{"/foo/bar", "/foo/baz/", false},
	{"/foo///bar", "/foo/bar", true},
	{"/foo/../bar", "/foo/bar", false},
	{"/foo/../bar", "/bar", true},
	{"/foo", "/foo/baz", true},
	{"/foo/", "/foo/baz", true},
	{"/foo/*", "/foo", false},
	{"/foo/*", "/foo/baz", true},
	{"bar", "/foo/bar/baz", true},
	{"bar", "/foo/bar/test.go", true},
	{"/foo/*test.*", "/foo/bar/test.go", false},
	{"/foo/*/test.*", "/foo/bar/test.go", true},
	{"/foo/*/bar/test.*", "/foo/bar/test.go", false},
	{"/*/*/bar/test.*", "/foo/bar/test.go", false},
	{"/*/*/bar/test.*", "/foo/bar/baz/test.go", false},
	{"/*/*/baz/test.*", "/foo/bar/baz/test.go", true},
	{"/*/foo/bar/test.*", "/foo/bar/baz/test.go", false},
	{"/*/foo/bar/test.*", "/foo/bar/baz/test.go", false},
	{"/foo/bar/test.*", "bar/baz/test.go", false},
	{"/x/y/bar/baz/test.*", "bar/baz/test.go", false},
	{"/x/y/bar/baz/test.c", "bar/baz/test.go", false},
	{"baz/test.*", "bar/baz/test.go", true},
	{"baz/tesT.*", "bar/baz/test.go", false},
	{"test.go", "bar/baz/test.go", true},
	{"*.go", "bar/baz/test.go", true},
	{"*.c", "bar/baz/test.go", false},
	{"sdk", "/foo/bar/sdk", true},
	{"sdk", "/foo/bar/sdk/test/sdk_foo.go", true},
	{
		"sdk/*/cpp/*/*vars*.html",
		"/usr/share/doc/libreoffice/sdk/docs/cpp/ref/a00517.html",
		false,
	},
	{"foo/**/bar/*.go", "/home/user/foo/work/special/project/bar/test.go", true},
	{"foo/**/bar/*.go", "/home/user/foo/bar/test.go", true},
	{"foo/**/bar/*.go", "x/foo/bar/test.go", true},
	{"foo/**/bar/*.go", "foo/bar/test.go", true},
	{"foo/**/bar/*.go", "foo/bar/baz/bar/test.go", true},
	{"foo/**/bar/*.go", "/home/user/foo/test.c", false},
	{"foo/**/bar/*.go", "bar/foo/main.go", false},
	{"foo/**/bar/*.go", "/foo/bar/main.go", true},
	{"foo/**/bar/*.go", "bar/main.go", false},
	{"foo/**/bar", "/home/user/foo/x/y/bar", true},
	{"foo/**/bar", "/home/user/foo/x/y/bar/main.go", true},
	{"foo/**/bar/**/x", "/home/user/foo/bar/x", true},
	{"foo/**/bar/**/x", "/home/user/foo/blaaa/blaz/bar/shared/work/x", true},
	{"user/**/important*", "/home/user/work/x/y/hidden/x", false},
	{"user/**/hidden*/**/c", "/home/user/work/x/y/hidden/z/a/b/c", true},
	{"c:/foo/*test.*", "c:/foo/bar/test.go", false},
	{"c:/foo", "c:/foo/bar", true},
	{"c:/foo/", "c:/foo/bar", true},
	{"c:/foo/*/test.*", "c:/foo/bar/test.go", true},
	{"c:/foo/*/bar/test.*", "c:/foo/bar/test.go", false},
}

func testpattern(t *testing.T, pattern, path string, shouldMatch bool) {
	match, err := filter.Match(pattern, path)
	if err != nil {
		t.Errorf("test pattern %q failed: expected no error for path %q, but error returned: %v",
			pattern, path, err)
	}

	if match != shouldMatch {
		t.Errorf("test: filter.Match(%q, %q): expected %v, got %v",
			pattern, path, shouldMatch, match)
	}
}

func TestMatch(t *testing.T) {
	for _, test := range matchTests {
		t.Run("", func(t *testing.T) {
			testpattern(t, test.pattern, test.path, test.match)
		})

		// Test with native path separator
		if filepath.Separator != '/' {
			pattern := strings.Replace(test.pattern, "/", string(filepath.Separator), -1)
			// Test with pattern as native
			t.Run("pattern-native", func(t *testing.T) {
				testpattern(t, pattern, test.path, test.match)
			})

			path := strings.Replace(test.path, "/", string(filepath.Separator), -1)
			t.Run("path-native", func(t *testing.T) {
				// Test with path as native
				testpattern(t, test.pattern, path, test.match)
			})

			t.Run("both-native", func(t *testing.T) {
				// Test with both pattern and path as native
				testpattern(t, pattern, path, test.match)
			})
		}
	}
}

var childMatchTests = []struct {
	pattern string
	path    string
	match   bool
}{
	{"", "", true},
	{"", "/foo", true},
	{"", "/x/y/z/foo", true},
	{"foo/bar", "/foo", true},
	{"baz/bar", "/foo", true},
	{"foo", "/foo/bar", true},
	{"bar", "/foo", true},
	{"baz", "/foo/bar", true},
	{"*", "/foo", true},
	{"*", "/foo/bar", true},
	{"/foo/bar", "/foo", true},
	{"/foo/bar/baz", "/foo", true},
	{"/foo/bar/baz", "/foo/bar", true},
	{"/foo/bar/baz", "/foo/baz", false},
	{"/foo/**/baz", "/foo/bar/baz", true},
	{"/foo/**/baz", "/foo/bar/baz/blah", true},
	{"/foo/**/qux", "/foo/bar/baz/qux", true},
	{"/foo/**/qux", "/foo/bar/baz", true},
	{"/foo/**/qux", "/foo/bar/baz/boo", true},
	{"/foo/**", "/foo/bar/baz", true},
	{"/foo/**", "/foo/bar", true},
	{"foo/**/bar/**/x", "/home/user/foo", true},
	{"foo/**/bar/**/x", "/home/user/foo/bar", true},
	{"foo/**/bar/**/x", "/home/user/foo/blaaa/blaz/bar/shared/work/x", true},
	{"/foo/*/qux", "/foo/bar", true},
	{"/foo/*/qux", "/foo/bar/boo", false},
	{"/foo/*/qux", "/foo/bar/boo/xx", false},
	{"/baz/bar", "/foo", false},
	{"/foo", "/foo/bar", true},
	{"/*", "/foo", true},
	{"/*", "/foo/bar", true},
	{"/foo", "/foo/bar", true},
	{"/**", "/foo", true},
	{"/*/**", "/foo", true},
	{"/*/**", "/foo/bar", true},
	{"/*/bar", "/foo", true},
	{"/bar/*", "/foo", false},
	{"/foo/*/baz", "/foo/bar", true},
	{"/foo/*/baz", "/foo/baz", true},
	{"/foo/*/baz", "/bar/baz", false},
	{"/**/*", "/foo", true},
	{"/**/bar", "/foo/bar", true},
}

func testchildpattern(t *testing.T, pattern, path string, shouldMatch bool) {
	match, err := filter.ChildMatch(pattern, path)
	if err != nil {
		t.Errorf("test child pattern %q failed: expected no error for path %q, but error returned: %v",
			pattern, path, err)
	}

	if match != shouldMatch {
		t.Errorf("test: filter.ChildMatch(%q, %q): expected %v, got %v",
			pattern, path, shouldMatch, match)
	}
}

func TestChildMatch(t *testing.T) {
	for _, test := range childMatchTests {
		t.Run("", func(t *testing.T) {
			testchildpattern(t, test.pattern, test.path, test.match)
		})

		// Test with native path separator
		if filepath.Separator != '/' {
			pattern := strings.Replace(test.pattern, "/", string(filepath.Separator), -1)
			// Test with pattern as native
			t.Run("pattern-native", func(t *testing.T) {
				testchildpattern(t, pattern, test.path, test.match)
			})

			path := strings.Replace(test.path, "/", string(filepath.Separator), -1)
			t.Run("path-native", func(t *testing.T) {
				// Test with path as native
				testchildpattern(t, test.pattern, path, test.match)
			})

			t.Run("both-native", func(t *testing.T) {
				// Test with both pattern and path as native
				testchildpattern(t, pattern, path, test.match)
			})
		}
	}
}

func ExampleMatch() {
	match, _ := filter.Match("*.go", "/home/user/file.go")
	fmt.Printf("match: %v\n", match)
	// Output:
	// match: true
}

func ExampleMatch_wildcards() {
	match, _ := filter.Match("/home/[uU]ser/?.go", "/home/user/F.go")
	fmt.Printf("match: %v\n", match)
	// Output:
	// match: true
}

var filterListTests = []struct {
	patterns   []string
	path       string
	match      bool
	childMatch bool
}{
	{[]string{}, "/foo/bar/test.go", false, false},
	{[]string{"*.go"}, "/foo/bar/test.go", true, true},
	{[]string{"*.c"}, "/foo/bar/test.go", false, true},
	{[]string{"*.go", "*.c"}, "/foo/bar/test.go", true, true},
	{[]string{"*"}, "/foo/bar/test.go", true, true},
	{[]string{"x"}, "/foo/bar/test.go", false, true},
	{[]string{"?"}, "/foo/bar/test.go", false, true},
	{[]string{"?", "x"}, "/foo/bar/x", true, true},
	{[]string{"/*/*/bar/test.*"}, "/foo/bar/test.go", false, false},
	{[]string{"/*/*/bar/test.*", "*.go"}, "/foo/bar/test.go", true, true},
	{[]string{"", "*.c"}, "/foo/bar/test.go", false, true},
}

func TestList(t *testing.T) {
	for i, test := range filterListTests {
		patterns := filter.ParsePatterns(test.patterns)
		match, err := filter.List(patterns, test.path)
		if err != nil {
			t.Errorf("test %d failed: expected no error for patterns %q, but error returned: %v",
				i, test.patterns, err)
			continue
		}

		if match != test.match {
			t.Errorf("test %d: filter.List(%q, %q): expected %v, got %v",
				i, test.patterns, test.path, test.match, match)
		}

		match, childMatch, err := filter.ListWithChild(patterns, test.path)
		if err != nil {
			t.Errorf("test %d failed: expected no error for patterns %q, but error returned: %v",
				i, test.patterns, err)
			continue
		}

		if match != test.match || childMatch != test.childMatch {
			t.Errorf("test %d: filter.ListWithChild(%q, %q): expected %v, %v, got %v, %v",
				i, test.patterns, test.path, test.match, test.childMatch, match, childMatch)
		}
	}
}

func ExampleList() {
	patterns := filter.ParsePatterns([]string{"*.c", "*.go"})
	match, _ := filter.List(patterns, "/home/user/file.go")
	fmt.Printf("match: %v\n", match)
	// Output:
	// match: true
}

func TestInvalidStrs(t *testing.T) {
	_, err := filter.Match("test", "")
	if err == nil {
		t.Error("Match accepted invalid path")
	}

	_, err = filter.ChildMatch("test", "")
	if err == nil {
		t.Error("ChildMatch accepted invalid path")
	}

	patterns := []string{"test"}
	_, err = filter.List(filter.ParsePatterns(patterns), "")
	if err == nil {
		t.Error("List accepted invalid path")
	}
}

func TestInvalidPattern(t *testing.T) {
	patterns := []string{"test/["}
	_, err := filter.List(filter.ParsePatterns(patterns), "test/example")
	if err == nil {
		t.Error("List accepted invalid pattern")
	}

	patterns = []string{"test/**/["}
	_, err = filter.List(filter.ParsePatterns(patterns), "test/example")
	if err == nil {
		t.Error("List accepted invalid pattern")
	}
}

func extractTestLines(t testing.TB) (lines []string) {
	f, err := os.Open("testdata/libreoffice.txt.bz2")
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	sc := bufio.NewScanner(bzip2.NewReader(f))
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}

	return lines
}

func TestFilterPatternsFile(t *testing.T) {
	lines := extractTestLines(t)

	var testPatterns = []struct {
		pattern string
		hits    uint
	}{
		{"*.html", 18249},
		{"sdk", 22186},
		{"sdk/*/cpp/*/*vars.html", 3},
	}

	for _, test := range testPatterns {
		var c uint
		for _, line := range lines {
			match, err := filter.Match(test.pattern, line)
			if err != nil {
				t.Error(err)
				continue
			}

			if match {
				c++
				// fmt.Printf("pattern %q, line %q\n", test.pattern, line)
			}
		}

		if c != test.hits {
			t.Errorf("wrong number of hits for pattern %q: want %d, got %d",
				test.pattern, test.hits, c)
		}
	}
}

func BenchmarkFilterLines(b *testing.B) {
	pattern := "sdk/*/cpp/*/*vars.html"
	lines := extractTestLines(b)
	var c uint

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		c = 0
		for _, line := range lines {
			match, err := filter.Match(pattern, line)
			if err != nil {
				b.Fatal(err)
			}

			if match {
				c++
			}
		}

		if c != 3 {
			b.Fatalf("wrong number of matches: expected 3, got %d", c)
		}
	}
}

func BenchmarkFilterPatterns(b *testing.B) {
	lines := extractTestLines(b)
	modlines := make([]string, 200)
	for i, line := range lines {
		if i >= len(modlines) {
			break
		}
		modlines[i] = line + "-does-not-match"
	}
	tests := []struct {
		name     string
		patterns []filter.Pattern
		matches  uint
	}{
		{"Relative", filter.ParsePatterns([]string{
			"does-not-match",
			"sdk/*",
			"*.html",
		}), 22185},
		{"Absolute", filter.ParsePatterns([]string{
			"/etc",
			"/home/*/test",
			"/usr/share/doc/libreoffice/sdk/docs/java",
		}), 150},
		{"Wildcard", filter.ParsePatterns([]string{
			"/etc/**/example",
			"/home/**/test",
			"/usr/**/java",
		}), 150},
		{"ManyNoMatch", filter.ParsePatterns(modlines), 0},
	}

	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			var c uint
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				c = 0
				for _, line := range lines {
					match, err := filter.List(test.patterns, line)
					if err != nil {
						b.Fatal(err)
					}

					if match {
						c++
					}
				}

				if c != test.matches {
					b.Fatalf("wrong number of matches: expected %d, got %d", test.matches, c)
				}
			}
		})
	}
}
