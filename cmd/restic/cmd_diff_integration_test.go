package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/restic/restic/internal/global"
	rtest "github.com/restic/restic/internal/test"
)

const DouglasAdamsPar1 = `
Far out in the uncharted backwaters
of the unfashionable end of the western spiral arm of the Galaxy
lies a small unregarded yellow sun.
`
const DouglasAdamsPar2 = `
Orbiting this at a distance of roughly ninety-two million miles
is an utterly insignificant little blue-green planet
whose ape-descended life forms are so amazingly primitive
that they still think digital watches are a pretty neat idea.
`

func testRunDiffOutput(t testing.TB, gopts global.Options, firstSnapshotID string, secondSnapshotID string, content bool) (string, error) {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		opts := DiffOptions{
			ShowMetadata:    false,
			ShowContentDiff: content,
		}
		return runDiff(ctx, opts, gopts, []string{firstSnapshotID, secondSnapshotID}, gopts.Term)
	})
	return buf.String(), err
}

func copyFile(dst string, src string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		// ignore subsequent errors
		_ = srcFile.Close()
		return err
	}

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		// ignore subsequent errors
		_ = srcFile.Close()
		_ = dstFile.Close()
		return err
	}

	err = srcFile.Close()
	if err != nil {
		// ignore subsequent errors
		_ = dstFile.Close()
		return err
	}

	err = dstFile.Close()
	if err != nil {
		return err
	}

	return nil
}

var diffOutputRegexPatterns = []string{
	"M.+DouglasAdams",
	"-.+modfile",
	"M.+modfile1",
	"\\+.+modfile2",
	"\\+.+modfile3",
	"\\+.+modfile4",
	"-.+submoddir",
	"-.+submoddir.subsubmoddir",
	"\\+.+submoddir2",
	"\\+.+submoddir2.subsubmoddir",
	"Files: +2 new, +1 removed, +3 changed",
	"Dirs: +3 new, +2 removed",
	"Data Blobs: +4 new, +3 removed",
	"Added: +8[0-9]{2}\\.[0-9]{3} KiB",
	"Removed: +3[0-9]{2}\\.[0-9]{3} KiB",
}

func testModifyFile(filename string) error {
	// 1. Read the entire file into memory
	input, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	length := len(input)

	// 2. pick 5 places and change that byte to a newline or
	length7 := length / 7
	offset := length7
	for range 5 {
		input[offset] = '|'
		offset += length7
	}

	// 3. Write the modified data back to the same file
	err = os.WriteFile(filename, input, 0644)
	return err
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 \n"

// GenerateRandomText returns a random string of length n
func testGenerateRandomText(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return b
}

func testCreateRandomTextFile(filename string, sizeBytes int) error {
	// 1. Create the file
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	// 2. Generate and write the data
	// Note: For very large files, write in chunks to save RAM
	data := testGenerateRandomText(sizeBytes)
	_, err = f.Write(data)
	return err
}

func setupDiffRepo(t *testing.T) (*testEnvironment, func(), string, string) {
	env, cleanup := withTestEnvironment(t)
	testRunInit(t, env.gopts)

	datadir := filepath.Join(env.base, "testdata")
	testdir := filepath.Join(datadir, "testdir")
	rtest.OK(t, os.Mkdir(testdir, 0755))
	largeDir := filepath.Join(testdir, "largeFiles")
	rtest.OK(t, os.Mkdir(largeDir, 0755))
	subtestdir := filepath.Join(testdir, "subtestdir")
	testfile := filepath.Join(testdir, "testfile")
	douglasAdams := filepath.Join(largeDir, "DouglasAdams")
	largeTextFile := filepath.Join(largeDir, "random-text-file")
	dAdams, err := os.Create(douglasAdams)
	rtest.OK(t, err)

	// write the original text file
	writer := bufio.NewWriter(dAdams)
	_, err = writer.WriteString(DouglasAdamsPar1)
	rtest.OK(t, err)
	rtest.OK(t, writer.Flush())
	rtest.OK(t, dAdams.Close())

	// large random text file
	rtest.OK(t, testCreateRandomTextFile(largeTextFile, 50<<10))

	//rtest.OK(t, os.Mkdir(testdir, 0755))
	rtest.OK(t, os.Mkdir(subtestdir, 0755))
	rtest.OK(t, appendRandomData(testfile, 256*1024))

	moddir := filepath.Join(datadir, "moddir")
	submoddir := filepath.Join(moddir, "submoddir")
	subsubmoddir := filepath.Join(submoddir, "subsubmoddir")
	modfile := filepath.Join(moddir, "modfile")
	rtest.OK(t, os.Mkdir(moddir, 0755))
	rtest.OK(t, os.Mkdir(submoddir, 0755))
	rtest.OK(t, os.Mkdir(subsubmoddir, 0755))
	rtest.OK(t, copyFile(modfile, testfile))
	rtest.OK(t, appendRandomData(modfile+"1", 256*1024))

	snapshots := make(map[string]struct{})
	opts := BackupOptions{}
	testRunBackup(t, "", []string{datadir}, opts, env.gopts)
	snapshots, firstSnapshotID := lastSnapshot(snapshots, loadSnapshotMap(t, env.gopts))

	rtest.OK(t, os.Rename(modfile, modfile+"3"))
	rtest.OK(t, os.Rename(submoddir, submoddir+"2"))
	rtest.OK(t, appendRandomData(modfile+"1", 256*1024))
	rtest.OK(t, appendRandomData(modfile+"2", 256*1024))
	rtest.OK(t, os.Mkdir(modfile+"4", 0755))
	dAdams, err = os.OpenFile(douglasAdams, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	rtest.OK(t, err)
	// append to text file
	_, err = dAdams.WriteString(DouglasAdamsPar2)
	rtest.OK(t, err)
	rtest.OK(t, dAdams.Close())

	// rewrite largeTextFile
	rtest.OK(t, testModifyFile(largeTextFile))

	testRunBackup(t, "", []string{datadir}, opts, env.gopts)
	_, secondSnapshotID := lastSnapshot(snapshots, loadSnapshotMap(t, env.gopts))

	return env, cleanup, firstSnapshotID, secondSnapshotID
}

func setupDiffRepoContent(t *testing.T) (*testEnvironment, func(), string, string) {
	env, cleanup := withTestEnvironment(t)
	testRunInit(t, env.gopts)

	datadir := filepath.Join(env.base, "testdata")
	largeDir := filepath.Join(datadir, "largeFiles")
	rtest.OK(t, os.Mkdir(largeDir, 0755))
	douglasAdams := filepath.Join(largeDir, "DouglasAdams")
	largeTextFile := filepath.Join(largeDir, "random-text-file")
	dAdams, err := os.Create(douglasAdams)
	rtest.OK(t, err)

	// write the original text file
	writer := bufio.NewWriter(dAdams)
	_, err = writer.WriteString(DouglasAdamsPar1)
	rtest.OK(t, err)
	rtest.OK(t, writer.Flush())
	rtest.OK(t, dAdams.Close())

	// large random text file
	rtest.OK(t, testCreateRandomTextFile(largeTextFile, 50<<10))

	snapshots := make(map[string]struct{})
	opts := BackupOptions{}
	testRunBackup(t, "", []string{largeDir}, opts, env.gopts)
	snapshots, firstSnapshotID := lastSnapshot(snapshots, loadSnapshotMap(t, env.gopts))

	dAdams, err = os.OpenFile(douglasAdams, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	rtest.OK(t, err)
	// append to text file
	_, err = dAdams.WriteString(DouglasAdamsPar2)
	rtest.OK(t, err)
	rtest.OK(t, dAdams.Close())

	// rewrite largeTextFile
	rtest.OK(t, testModifyFile(largeTextFile))

	testRunBackup(t, "", []string{largeDir}, opts, env.gopts)
	_, secondSnapshotID := lastSnapshot(snapshots, loadSnapshotMap(t, env.gopts))

	return env, cleanup, firstSnapshotID, secondSnapshotID
}

func TestDiff(t *testing.T) {
	env, cleanup, firstSnapshotID, secondSnapshotID := setupDiffRepo(t)
	defer cleanup()

	// quiet suppresses the diff output except for the summary
	env.gopts.Quiet = false
	_, err := testRunDiffOutput(t, env.gopts, "", secondSnapshotID, false)
	rtest.Assert(t, err != nil, "expected error on invalid snapshot id")

	out, err := testRunDiffOutput(t, env.gopts, firstSnapshotID, secondSnapshotID, false)
	rtest.OK(t, err)

	for i, pattern := range diffOutputRegexPatterns {
		r, err := regexp.Compile(pattern)
		rtest.Assert(t, err == nil, "failed to compile regexp %v", pattern)
		rtest.Assert(t, r.MatchString(out), "expected pattern(%d) %v in output, got\n%v", i, pattern, out)
	}

	// check quiet output
	env.gopts.Quiet = true
	outQuiet, err := testRunDiffOutput(t, env.gopts, firstSnapshotID, secondSnapshotID, false)
	rtest.OK(t, err)

	rtest.Assert(t, len(outQuiet) < len(out), "expected shorter output on quiet mode %v vs. %v", len(outQuiet), len(out))
}

type typeSniffer struct {
	MessageType string `json:"message_type"`
}

func TestDiffJSON(t *testing.T) {
	env, cleanup, firstSnapshotID, secondSnapshotID := setupDiffRepo(t)
	defer cleanup()

	// quiet suppresses the diff output except for the summary
	env.gopts.Quiet = false
	env.gopts.JSON = true
	out, err := testRunDiffOutput(t, env.gopts, firstSnapshotID, secondSnapshotID, false)
	rtest.OK(t, err)

	var stat DiffStatsContainer
	var changes int

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		var sniffer typeSniffer
		rtest.OK(t, json.Unmarshal([]byte(line), &sniffer))
		switch sniffer.MessageType {
		case "change":
			changes++
		case "statistics":
			rtest.OK(t, json.Unmarshal([]byte(line), &stat))
		default:
			t.Fatalf("unexpected message type %v", sniffer.MessageType)
		}
	}
	rtest.Equals(t, 11, changes)
	rtest.Assert(t,
		stat.Added.Files == 2 && stat.Added.Dirs == 3 && stat.Added.DataBlobs == 4 &&
			stat.Removed.Files == 1 && stat.Removed.Dirs == 2 && stat.Removed.DataBlobs == 3 &&
			stat.ChangedFiles == 3,
		"unexpected statistics")

	// check quiet output
	env.gopts.Quiet = true
	outQuiet, err := testRunDiffOutput(t, env.gopts, firstSnapshotID, secondSnapshotID, false)
	rtest.OK(t, err)

	stat = DiffStatsContainer{}
	rtest.OK(t, json.Unmarshal([]byte(outQuiet), &stat))
	rtest.Assert(t,
		stat.Added.Files == 2 && stat.Added.Dirs == 3 && stat.Added.DataBlobs == 4 &&
			stat.Removed.Files == 1 && stat.Removed.Dirs == 2 && stat.Removed.DataBlobs == 3 &&
			stat.ChangedFiles == 3,
		"unexpected statistics")
	rtest.Assert(t, stat.SourceSnapshot == firstSnapshotID && stat.TargetSnapshot == secondSnapshotID, "unexpected snapshot ids")
}

func TestDiffContent(t *testing.T) {
	env, cleanup, firstSnapshotID, secondSnapshotID := setupDiffRepo(t)
	defer cleanup()

	// check quiet output
	env.gopts.Quiet = false
	env.gopts.Verbose = 2
	env.gopts.Verbosity = 2
	opts := DiffOptions{
		ShowContentDiff: true,
	}
	out, err := testRunDiffWithOpts(t, opts, env.gopts, firstSnapshotID, secondSnapshotID)
	rtest.OK(t, err)

	checks := []string{
		`(?ms).+modfile1.+is a binary file and the two file differ`,
		`(?ms).+\+\+\+.+DouglasAdams.+\+Orbiting this at a distance of roughly ninety-two million miles`,
	}

	for i, pattern := range checks {
		r, err := regexp.Compile(pattern)
		rtest.Assert(t, err == nil, "failed to compile regexp %v", pattern)
		rtest.Assert(t, r.MatchString(out), "expected pattern(%d) %q in output, got\n%v", i, pattern, out)
	}
}

func testRunDiffWithOpts(t testing.TB, opts DiffOptions, gopts global.Options, firstSnapshotID string, secondSnapshotID string) (string, error) {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runDiff(ctx, opts, gopts, []string{firstSnapshotID, secondSnapshotID}, gopts.Term)
	})
	return buf.String(), err
}

// TestDiffContentError: check for errors
func TestDiffContentError(t *testing.T) {
	// test some error exit points
	env, cleanup, firstSnapshotID, secondSnapshotID := setupDiffRepo(t)
	defer cleanup()

	// --JSON and --content don't agree with one another
	env.gopts.JSON = true
	_, err := testRunDiffOutput(t, env.gopts, firstSnapshotID, secondSnapshotID, true)
	rtest.Assert(t, err != nil && err.Error() == "Fatal: options --JSON and --content are incompatible. Try without --JSON",
		"expected error messages options --JSON and --content are incompatible. Try without --JSON, got %v", err)

	env, cleanup2, firstSnapshotID, secondSnapshotID := setupDiffRepo(t)
	defer cleanup2()
	//
	opts := DiffOptions{
		ShowContentDiff: true,
		diffSizeMax:     "1234h",
	}
	_, err = testRunDiffWithOpts(t, opts, env.gopts, firstSnapshotID, secondSnapshotID)
	rtest.Assert(t, err != nil && err.Error() ==
		`Fatal: invalid number of bytes "1234h" for --diff-max-size: strconv.ParseInt: parsing "1234h": invalid syntax`, "expected error messsage %v", err)
}

// TestDiffContentLargeFileCutoff: force an oversized File and check message
func TestDiffContentLargeFileCutoff(t *testing.T) {
	env, cleanup, firstSnapshotID, secondSnapshotID := setupDiffRepoContent(t)
	defer cleanup()

	// check quiet output
	env.gopts.Quiet = false
	env.gopts.Verbose = 2
	env.gopts.Verbosity = 2
	opts := DiffOptions{
		ShowContentDiff: true,
		diffSizeMax:     "1024",
	}
	out, err := testRunDiffWithOpts(t, opts, env.gopts, firstSnapshotID, secondSnapshotID)
	rtest.OK(t, err)
	rtest.Assert(t, strings.Contains(string(out), OversizedMessage),
		"expected file truncate message, got none!")
}
