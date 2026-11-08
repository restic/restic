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
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
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
	_, err = fmt.Fprintf(f1, "\n%s*\n # here's a comment\n", f1.Name())
	rtest.OK(t, err)
	rtest.OK(t, f1.Close())

	f2, err := os.Create(filepath.Join(dir, "fromfile-verbatim"))
	rtest.OK(t, err)
	for _, filename := range []string{fooSpace, barStar} {
		// Empty lines should be ignored. CR+LF is allowed.
		_, err = fmt.Fprintf(f2, "%s\r\n\n", filepath.Join(dir, filename))
		rtest.OK(t, err)
	}
	rtest.OK(t, f2.Close())

	f3, err := os.Create(filepath.Join(dir, "fromfile-raw"))
	rtest.OK(t, err)
	for _, filename := range []string{"baz", "quux"} {
		_, err = fmt.Fprintf(f3, "%s\x00", filepath.Join(dir, filename))
		rtest.OK(t, err)
	}
	rtest.OK(t, err)
	rtest.OK(t, f3.Close())

	opts := BackupOptions{
		FilesFrom:         []string{f1.Name()},
		FilesFromVerbatim: []string{f2.Name()},
		FilesFromRaw:      []string{f3.Name()},
	}

	targets, err := collectTargets(opts, []string{filepath.Join(dir, "cmdline arg")}, t.Logf, nil, fs.NewLocal(), "")
	rtest.OK(t, err)
	sort.Strings(targets)
	rtest.Equals(t, expect, targets)

	_, err = collectTargets(opts, []string{filepath.Join(dir, "cmdline arg"), filepath.Join(dir, "non-existing-file")}, t.Logf, nil, fs.NewLocal(), "")
	rtest.Assert(t, err == ErrInvalidSourceData, "expected error when not all targets exist")
}

func TestCollectTargetsS3(t *testing.T) {
	dir := rtest.TempDir(t)
	client := fs.NewS3MemClient(map[string][]byte{
		"prefix/dir/a.jpg": []byte("a"),
		"prefix/dir/b.txt": []byte("b"),
		"prefix/x/y":       []byte("y"),
		"other":            []byte("other"),
		"priv/dir/b.jpg":   []byte("private"),
	}, time.Now())
	filesys, err := fs.NewS3(t.Context(), client, "bucket", fs.S3Options{})
	rtest.OK(t, err)

	writeFile := func(name, content string) string {
		filename := filepath.Join(dir, name)
		rtest.OK(t, os.WriteFile(filename, []byte(content), 0644))
		return filename
	}
	empty := writeFile("empty", "")
	verbatim := writeFile("verbatim", "dir/a.jpg\n/prefix/x/y\n")
	patterns := writeFile("patterns", "# comment\n  dir/*.jpg \n\nx/*\nnope*\n")
	noMatch := writeFile("no-match", "nope*\n")

	for _, test := range []struct {
		opts BackupOptions
		args []string
		want []string
	}{
		{BackupOptions{}, nil, []string{"/bucket/prefix"}},
		{BackupOptions{}, []string{"dir", "/prefix/x"}, []string{"/bucket/prefix/dir", "/bucket/prefix/x"}},
		{BackupOptions{FilesFromVerbatim: []string{verbatim}}, nil, []string{"/bucket/prefix/dir/a.jpg", "/bucket/prefix/x/y"}},
		{BackupOptions{FilesFrom: []string{patterns}}, []string{"dir/b.txt"}, []string{"/bucket/prefix/dir/a.jpg", "/bucket/prefix/x/y", "/bucket/prefix/dir/b.txt"}},
	} {
		targets, err := collectTargets(test.opts, test.args, t.Logf, nil, filesys, "/bucket/prefix")
		rtest.OK(t, err)
		rtest.Equals(t, test.want, targets)
	}

	// an empty list of targets must not default to the whole bucket
	for _, opts := range []BackupOptions{
		{FilesFromVerbatim: []string{empty}},
		{FilesFrom: []string{noMatch}},
	} {
		_, err = collectTargets(opts, nil, t.Logf, nil, filesys, "/bucket/prefix")
		rtest.Assert(t, err != nil && strings.Contains(err.Error(), "nothing to backup"), "unexpected error %v", err)
	}

	// targets outside the prefix are skipped
	var warnings []string
	warnf := func(msg string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(msg, args...))
	}
	outside := writeFile("outside", "/foo/*.jpg\n/other*\n/p*/dir/*.jpg\n")
	targets, err := collectTargets(BackupOptions{FilesFrom: []string{outside}}, []string{"/other", "../other", "/"}, warnf, nil, filesys, "/bucket/prefix")
	rtest.OK(t, err)
	rtest.Equals(t, []string{"/bucket/prefix/dir/a.jpg"}, targets)
	rtest.Equals(t, []string{
		"pattern \"/foo/*.jpg\" is outside prefix, skipping\n",
		"pattern \"/other*\" is outside prefix, skipping\n",
		"\"/other\" is outside prefix, skipping\n",
		"\"../other\" is outside prefix, skipping\n",
		"\"/\" is outside prefix, skipping\n",
	}, warnings)

	// a pattern which cannot match inside the prefix is rejected without
	// listing the bucket
	client.Lists = 0
	_, err = collectTargets(BackupOptions{FilesFrom: []string{writeFile("foo", "/foo/*.jpg\n")}}, nil, warnf, nil, filesys, "/bucket/prefix")
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "nothing to backup"), "unexpected error %v", err)
	rtest.Equals(t, 0, client.Lists)
}

func TestFilterExistingUnreadable(t *testing.T) {
	dir := rtest.TempDir(t)

	existing := filepath.Join(dir, "existing")
	rtest.OK(t, os.Mkdir(existing, 0755))

	file := filepath.Join(dir, "file")
	rtest.OK(t, os.WriteFile(file, []byte("x"), 0600))

	// Regression test for #5667. A target whose Lstat fails with an error other
	// than ErrNotExist must be skipped (ENOTDIR on unix, NUL byte everywhere).
	for _, unreadable := range []string{filepath.Join(file, "child"), "invalid\x00path"} {
		result, err := filterExisting([]string{unreadable}, fs.NewLocal(), t.Logf)
		rtest.Assert(t, errors.Is(err, ErrNoSourceData), "input %q: expected ErrNoSourceData; got %v", unreadable, err)
		rtest.Assert(t, len(result) == 0, "input %q: expected no targets; got %v", unreadable, result)

		result, err = filterExisting([]string{existing, unreadable}, fs.NewLocal(), t.Logf)
		rtest.Assert(t, errors.Is(err, ErrInvalidSourceData), "input %q: expected ErrInvalidSourceData; got %v", unreadable, err)
		rtest.Equals(t, []string{existing}, result)
	}
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
