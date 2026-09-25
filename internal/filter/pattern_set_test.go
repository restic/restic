package filter_test

import (
	"math/rand"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/restic/restic/internal/filter"
)

// RejectByPattern and IncludeByPattern index wildcard-free patterns, they must
// return the same results as List and ListWithChild.
func TestPatternSetMatchesList(t *testing.T) {
	// every fourth line is enough and keeps the test fast
	var lines []string
	for i, line := range extractTestLines(t) {
		if i%4 == 0 {
			lines = append(lines, line)
		}
	}
	paths := append([]string{"/", `C:\`, `\\server\share\dir`}, lines...)
	for i := 0; i < len(lines); i += 25 {
		parts := strings.Split(lines[i][1:], "/")
		relative := strings.Join(parts[len(parts)/2:], "/")
		paths = append(paths, path.Dir(lines[i]), relative, "C:"+filepath.FromSlash(lines[i]))
	}

	// patterns which can contain an empty part, which means "**"
	edge := []string{"/", "//usr/share", `C:\`, `\\server\share`, "**", "/usr/**", "usr/**/doc"}

	r := rand.New(rand.NewSource(1))
	gen := func(kind int) string {
		line := lines[r.Intn(len(lines))]
		parts := strings.Split(line[1:], "/")
		// skip the first parts, most paths start with /usr/share/doc/libreoffice
		i := len(parts)/2 + r.Intn(len(parts)-len(parts)/2)
		switch kind {
		case 0:
			return "/" + strings.Join(parts[:i+1], "/")
		case 1:
			return parts[i]
		case 2:
			return strings.Join(parts[i/2:i+1], "/")
		case 3:
			return line + "-no-match"
		case 4:
			return "*" + path.Ext(line)
		case 5:
			return "/usr/**/" + parts[i]
		case 6:
			return "sdk/*/" + parts[i]
		default:
			return filepath.FromSlash(line)
		}
	}

	for round := 0; round < 60; round++ {
		// use only some kinds of patterns per round, relative patterns hide
		// differences in childMayMatch
		kinds := r.Perm(8)[:1+r.Intn(3)]
		if round < len(edge) {
			kinds = []int{0, 3}
		}
		var patterns []string
		for n := 8 + r.Intn(40); n > 0; n-- {
			patterns = append(patterns, gen(kinds[r.Intn(len(kinds))]))
		}
		if round < len(edge) {
			patterns = append(patterns, edge[round])
		}
		if round%10 == 9 {
			// with a negated pattern the order of the patterns matters
			line := lines[r.Intn(len(lines))]
			patterns = append(patterns, path.Dir(line), "!"+line)
		}

		parsed := filter.ParsePatterns(patterns)
		var warned bool
		warnf := func(string, ...any) { warned = true }
		reject := filter.RejectByPattern(patterns, warnf)
		include := filter.IncludeByPattern(patterns, warnf)

		for _, p := range paths {
			want, err := filter.List(parsed, p)
			warned = false
			if got := reject(p); got != want || warned != (err != nil) {
				t.Fatalf("patterns %q, path %q: RejectByPattern returned %v, List returned %v, %v", patterns, p, got, want, err)
			}

			want, wantChild, err := filter.ListWithChild(parsed, p)
			warned = false
			if got, gotChild := include(p); got != want || gotChild != wantChild || warned != (err != nil) {
				t.Fatalf("patterns %q, path %q: IncludeByPattern returned %v %v, ListWithChild returned %v %v, %v", patterns, p, got, gotChild, want, wantChild, err)
			}
		}
	}
}

func BenchmarkRejectByPattern(b *testing.B) {
	lines := extractTestLines(b)
	var files []string
	for _, line := range lines {
		if path.Ext(line) != "" {
			files = append(files, line)
		}
	}
	many := func(n int) []string {
		var patterns []string
		for i := 0; i < n; i++ {
			// half of the patterns exclude a file, the rest never match
			if i%2 == 0 {
				patterns = append(patterns, files[(i*7)%len(files)])
			} else {
				patterns = append(patterns, "/var/lib/pkg/file-"+strconv.Itoa(i))
			}
		}
		return patterns
	}
	literals := []string{"/etc", "/home/user/test", "/usr/share/doc/libreoffice/sdk/docs/java"}
	wildcards := []string{"*.tmp", "/home/*/.cache", "**/node_modules", "sdk/*/cpp/*/*vars.html"}

	tests := []struct {
		name     string
		patterns []string
	}{
		{"3Literals", literals},
		{"4Wildcards", wildcards},
		{"7Mixed", append(slices.Clone(literals), wildcards...)},
		{"27Mixed", append(append(many(20), literals...), wildcards...)},
		{"1kLiterals", many(1000)},
		{"100kLiterals", many(100000)},
	}

	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			reject := filter.RejectByPattern(test.patterns, func(string, ...any) {})
			b.ReportAllocs()
			for b.Loop() {
				for _, line := range lines {
					reject(line)
				}
			}
		})
	}
}
