package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestCollectTargets(t *testing.T) {
	dir := rtest.TempDir(t)

	fooSpace := "foo "
	barStar := "bar*"              // Must sort before the others, below.
	if runtime.GOOS == "windows" { // Doesn't allow "*" or trailing space.
		fooSpace = "foo"
		barStar = "bar"
	}

	var expect []string
	for _, filename := range []string{
		barStar, "baz", "cmdline arg", fooSpace,
		"fromfile", "fromfile-raw", "fromfile-verbatim", "quux",
	} {
		// All mentioned files must exist for collectTargets.
		f, err := os.Create(filepath.Join(dir, filename))
		rtest.OK(t, err)
		rtest.OK(t, f.Close())

		expect = append(expect, f.Name())
	}

	f1, err := os.Create(filepath.Join(dir, "fromfile"))
	rtest.OK(t, err)
	// Empty lines should be ignored. A line starting with '#' is a comment.
	fmt.Fprintf(f1, "\n%s*\n # here's a comment\n", f1.Name())
	rtest.OK(t, f1.Close())

	f2, err := os.Create(filepath.Join(dir, "fromfile-verbatim"))
	rtest.OK(t, err)
	for _, filename := range []string{fooSpace, barStar} {
		// Empty lines should be ignored. CR+LF is allowed.
		fmt.Fprintf(f2, "%s\r\n\n", filepath.Join(dir, filename))
	}
	rtest.OK(t, f2.Close())

	f3, err := os.Create(filepath.Join(dir, "fromfile-raw"))
	rtest.OK(t, err)
	for _, filename := range []string{"baz", "quux"} {
		fmt.Fprintf(f3, "%s\x00", filepath.Join(dir, filename))
	}
	rtest.OK(t, err)
	rtest.OK(t, f3.Close())

	opts := BackupOptions{
		FilesFrom:         []string{f1.Name()},
		FilesFromVerbatim: []string{f2.Name()},
		FilesFromRaw:      []string{f3.Name()},
	}

	targets, err := collectTargets(opts, []string{filepath.Join(dir, "cmdline arg")})
	rtest.OK(t, err)
	sort.Strings(targets)
	rtest.Equals(t, expect, targets)
}

func TestReadFilenamesRaw(t *testing.T) {
	// These should all be returned exactly as-is.
	expected := []string{
		"\xef\xbb\xbf/utf-8-bom",
		"/absolute",
		"../.././relative",
		"\t\t leading and trailing space   \t\t",
		"newline\nin filename",
		"not UTF-8: \x80\xff/simple",
		` / *[]* \ `,
	}

	var buf bytes.Buffer
	for _, name := range expected {
		buf.WriteString(name)
		buf.WriteByte(0)
	}

	got, err := readFilenamesRaw(&buf)
	rtest.OK(t, err)
	rtest.Equals(t, expected, got)

	// Empty input is ok.
	got, err = readFilenamesRaw(strings.NewReader(""))
	rtest.OK(t, err)
	rtest.Equals(t, 0, len(got))

	// An empty filename is an error.
	_, err = readFilenamesRaw(strings.NewReader("foo\x00\x00"))
	rtest.Assert(t, err != nil, "no error for zero byte")
	rtest.Assert(t, strings.Contains(err.Error(), "empty filename"),
		"wrong error message: %v", err.Error())

	// No trailing NUL byte is an error, because it likely means we're
	// reading a line-oriented text file (someone forgot -print0).
	_, err = readFilenamesRaw(strings.NewReader("simple.txt"))
	rtest.Assert(t, err != nil, "no error for zero byte")
	rtest.Assert(t, strings.Contains(err.Error(), "zero byte"),
		"wrong error message: %v", err.Error())
}
