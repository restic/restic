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

	"github.com/restic/restic/internal/errors"
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

	targets, err := collectTargets(opts, []string{filepath.Join(dir, "cmdline arg")}, t.Logf, nil)
	rtest.OK(t, err)
	sort.Strings(targets)
	rtest.Equals(t, expect, targets)

	_, err = collectTargets(opts, []string{filepath.Join(dir, "cmdline arg"), filepath.Join(dir, "non-existing-file")}, t.Logf, nil)
	rtest.Assert(t, err == ErrInvalidSourceData, "expected error when not all targets exist")
}

// TestCollectTargetsGlobDoesNotFollowWildcardDirSymlink is a regression
// test for #21790. filepath.Glob follows directory symlinks, so a
// Wine-style dosdevices/z: link is expanded into the target tree.
// Include Glob must not return matches whose path goes through that
// symlink (wildcard-matched dirs are not followed).
func TestCollectTargetsGlobDoesNotFollowWildcardDirSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Wine-style z: symlink is a Unix issue")
	}

	dir := rtest.TempDir(t)

	// Tiny decoy (NOT /proc) that the z: symlink points at.
	decoyEtc := filepath.Join(dir, "decoy", "etc")
	rtest.OK(t, os.MkdirAll(decoyEtc, 0o755))
	rtest.OK(t, os.WriteFile(filepath.Join(decoyEtc, "rc_maps.cfg"), []byte("x"), 0o644))

	// Wine-shaped prefix/dosdevices/z: -> decoy
	dosdevices := filepath.Join(dir, "prefix", "dosdevices")
	rtest.OK(t, os.MkdirAll(dosdevices, 0o755))
	rtest.OK(t, os.Symlink(filepath.Join(dir, "decoy"), filepath.Join(dosdevices, "z:")))

	// A real directory next to z: so normal include patterns still match.
	realEtc := filepath.Join(dosdevices, "real", "etc")
	rtest.OK(t, os.MkdirAll(realEtc, 0o755))
	rtest.OK(t, os.WriteFile(filepath.Join(realEtc, "local.cfg"), []byte("y"), 0o644))

	// Pattern: prefix/dosdevices/*/*/*.cfg*  — * matches z: then etc then rc_maps.cfg
	// if Glob follows the symlink.
	pattern := filepath.Join(dir, "prefix", "dosdevices", "*", "*", "*.cfg*")
	include := filepath.Join(dir, "include.txt")
	rtest.OK(t, os.WriteFile(include, []byte(pattern+"\n"), 0o644))

	targets, err := collectTargets(BackupOptions{FilesFrom: []string{include}}, nil, t.Logf, nil)
	rtest.OK(t, err)

	symlinkMarker := string(os.PathSeparator) + "z:" + string(os.PathSeparator)
	var through []string
	for _, tgt := range targets {
		if strings.Contains(tgt, symlinkMarker) {
			through = append(through, tgt)
		}
	}
	t.Logf("collectTargets returned %d target(s), %d through z: symlink: %v", len(targets), len(through), through)
	rtest.Assert(t, len(through) == 0, "include Glob followed z: symlink: %d match(es) through z:: %v", len(through), through)

	// The same pattern must still match through a real (non-symlink) directory.
	var sawReal bool
	for _, tgt := range targets {
		if strings.Contains(tgt, string(os.PathSeparator)+"real"+string(os.PathSeparator)) && strings.HasSuffix(tgt, "local.cfg") {
			sawReal = true
			break
		}
	}
	rtest.Assert(t, sawReal, "normal include through a real directory disappeared: %v", targets)

	// Literal symlink prefixes are still followed (pattern names the link).
	linkdir := filepath.Join(dir, "linkdir")
	rtest.OK(t, os.Symlink(filepath.Join(dir, "decoy"), linkdir))
	literal := filepath.Join(linkdir, "etc", "*.cfg*")
	include2 := filepath.Join(dir, "include-literal.txt")
	rtest.OK(t, os.WriteFile(include2, []byte(literal+"\n"), 0o644))
	targets, err = collectTargets(BackupOptions{FilesFrom: []string{include2}}, nil, t.Logf, nil)
	rtest.OK(t, err)
	rtest.Assert(t, len(targets) == 1, "literal symlink prefix should still expand: %v", targets)
}

// TestCollectTargetsGlobFollowsLiteralDirSymlinkAfterWildcard is the
// counterpart to the Wine case above. The Wine bug is wildcard-expanded
// directories that are dosdevices-style symlinks (matched by "*"). A
// *literal* path component after a wildcard must still be followed if it
// is a directory symlink, so a pattern like tmp/*/foo/bar.cfg still
// matches when foo is a symlink to a directory.
func TestCollectTargetsGlobFollowsLiteralDirSymlinkAfterWildcard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory symlink layout is a Unix issue")
	}

	dir := rtest.TempDir(t)

	// Real tree that the literal "foo" symlink will point at.
	realFoo := filepath.Join(dir, "realfoo")
	rtest.OK(t, os.MkdirAll(realFoo, 0o755))
	rtest.OK(t, os.WriteFile(filepath.Join(realFoo, "bar.cfg"), []byte("z"), 0o644))

	// Wildcard-matched directory containing a literal symlink named foo.
	wild := filepath.Join(dir, "tmp", "matched")
	rtest.OK(t, os.MkdirAll(wild, 0o755))
	rtest.OK(t, os.Symlink(realFoo, filepath.Join(wild, "foo")))

	// Pattern: tmp/*/foo/*.cfg — "*" matches "matched", "foo" is literal.
	pattern := filepath.Join(dir, "tmp", "*", "foo", "*.cfg")
	include := filepath.Join(dir, "include-literal-after-wild.txt")
	rtest.OK(t, os.WriteFile(include, []byte(pattern+"\n"), 0o644))

	targets, err := collectTargets(BackupOptions{FilesFrom: []string{include}}, nil, t.Logf, nil)
	rtest.OK(t, err)
	rtest.Assert(t, len(targets) == 1, "literal symlink dir after wildcard should still match: %v", targets)
	wantSuffix := filepath.Join("foo", "bar.cfg")
	rtest.Assert(t, strings.HasSuffix(targets[0], wantSuffix), "expected match through literal foo symlink ending in %q, got %v", wantSuffix, targets)
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
		result, err := filterExisting([]string{unreadable}, t.Logf)
		rtest.Assert(t, errors.Is(err, ErrNoSourceData), "input %q: expected ErrNoSourceData; got %v", unreadable, err)
		rtest.Assert(t, len(result) == 0, "input %q: expected no targets; got %v", unreadable, result)

		result, err = filterExisting([]string{existing, unreadable}, t.Logf)
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
