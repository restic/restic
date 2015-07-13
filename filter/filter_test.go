package filter_test

import (
	"bufio"
	"compress/bzip2"
	"fmt"
	"os"
	"testing"

	"github.com/restic/restic/filter"
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
	{"sdk/*/cpp/*/*vars*.html", "/usr/share/doc/libreoffice/sdk/docs/cpp/ref/a00517.html", false},
}

func TestMatch(t *testing.T) {
	for i, test := range matchTests {
		match, err := filter.Match(test.pattern, test.path)
		if err != nil {
			t.Errorf("test %d failed: expected no error for pattern %q, but error returned: %v",
				i, test.pattern, err)
			continue
		}

		if match != test.match {
			t.Errorf("test %d: filter.Match(%q, %q): expected %v, got %v",
				i, test.pattern, test.path, test.match, match)
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
	patterns []string
	path     string
	match    bool
}{
	{[]string{"*.go"}, "/foo/bar/test.go", true},
	{[]string{"*.c"}, "/foo/bar/test.go", false},
	{[]string{"*.go", "*.c"}, "/foo/bar/test.go", true},
	{[]string{"*"}, "/foo/bar/test.go", true},
	{[]string{"x"}, "/foo/bar/test.go", false},
	{[]string{"?"}, "/foo/bar/test.go", false},
	{[]string{"?", "x"}, "/foo/bar/x", true},
	{[]string{"/*/*/bar/test.*"}, "/foo/bar/test.go", false},
	{[]string{"/*/*/bar/test.*", "*.go"}, "/foo/bar/test.go", true},
}

func TestMatchList(t *testing.T) {
	for i, test := range filterListTests {
		match, err := filter.MatchList(test.patterns, test.path)
		if err != nil {
			t.Errorf("test %d failed: expected no error for patterns %q, but error returned: %v",
				i, test.patterns, err)
			continue
		}

		if match != test.match {
			t.Errorf("test %d: filter.MatchList(%q, %q): expected %v, got %v",
				i, test.patterns, test.path, test.match, match)
		}
	}
}

func ExampleMatchList() {
	match, _ := filter.MatchList([]string{"*.c", "*.go"}, "/home/user/file.go")
	fmt.Printf("match: %v\n", match)
	// Output:
	// match: true
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

func BenchmarkFilterSingle(b *testing.B) {
	pattern := "sdk/*/cpp/*/*vars.html"
	line := "/usr/share/doc/libreoffice/sdk/docs/cpp/ref/a00517.html"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		filter.Match(pattern, line)
	}
}

type test struct {
	path  string
	match bool
}

var filterTests = []struct {
	include, exclude []string
	tests            []test
}{
	{
		[]string{"*.go", "/home/user"},
		[]string{},
		[]test{
			{"/home/user/foo/test.c", true},
			{"/home/user/foo/test.go", true},
			{"/home/foo/test.go", true},
			{"/home/foo/test.doc", false},
			{"/x", false},
			{"main.go", true},
		},
	},
	{
		nil,
		[]string{"*.docx", "*.xlsx"},
		[]test{
			{"/home/user/foo/test.c", true},
			{"/home/user/foo/test.docx", false},
			{"/home/foo/test.xlsx", false},
			{"/home/foo/test.doc", true},
			{"/x", true},
			{"main.go", true},
		},
	},
	{
		[]string{"accounting.*", "*Partner*"},
		[]string{"*.docx", "*.xlsx"},
		[]test{
			// {"/home/user/foo/test.c", true},
			{"/home/user/Partner/test.docx", true},
			{"/home/user/bar/test.docx", false},
			{"/home/user/test.xlsx", false},
			{"/home/foo/test.doc", true},
			{"/x", true},
			{"main.go", true},
			{"/users/A/accounting.xlsx", true},
			{"/users/A/Calculation Partner.xlsx", true},
		},
	},
}

func TestFilter(t *testing.T) {
	for i, test := range filterTests {
		f := filter.New(test.include, test.exclude)

		for _, testfile := range test.tests {
			matched, err := f.Match(testfile.path)
			if err != nil {
				t.Error(err)
			}

			if matched != testfile.match {
				t.Errorf("test %d: filter.Match(%q): expected %v, got %v",
					i, testfile.path, testfile.match, matched)
			}
		}
	}
}

func BenchmarkFilter(b *testing.B) {
	lines := extractTestLines(b)
	f := filter.New([]string{"sdk", "*.html"}, []string{"*.png"})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, line := range lines {
			f.Match(line)
		}
	}
}

func BenchmarkFilterInclude(b *testing.B) {
	lines := extractTestLines(b)
	f := filter.New([]string{"sdk", "*.html"}, nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, line := range lines {
			f.Match(line)
		}
	}
}

func BenchmarkFilterExclude(b *testing.B) {
	lines := extractTestLines(b)
	f := filter.New(nil, []string{"*.png"})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, line := range lines {
			f.Match(line)
		}
	}
}
