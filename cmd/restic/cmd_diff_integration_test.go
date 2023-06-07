package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func testRunDiffOutput(gopts GlobalOptions, firstSnapshotID string, secondSnapshotID string) (string, error) {
	buf, err := withCaptureStdout(func() error {
		opts := DiffOptions{
			ShowMetadata: false,
		}
		return runDiff(context.TODO(), opts, gopts, []string{firstSnapshotID, secondSnapshotID})
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
	"-.+modfile",
	"M.+modfile1",
	"\\+.+modfile2",
	"\\+.+modfile3",
	"\\+.+modfile4",
	"-.+submoddir",
	"-.+submoddir.subsubmoddir",
	"\\+.+submoddir2",
	"\\+.+submoddir2.subsubmoddir",
	"Files: +2 new, +1 removed, +1 changed",
	"Dirs: +3 new, +2 removed",
	"Data Blobs: +2 new, +1 removed",
	"Added: +7[0-9]{2}\\.[0-9]{3} KiB",
	"Removed: +2[0-9]{2}\\.[0-9]{3} KiB",
}

func setupDiffRepo(t *testing.T) (*testEnvironment, func(), string, string) {
	env, cleanup := withTestEnvironment(t)
	testRunInit(t, env.gopts)

	datadir := filepath.Join(env.base, "testdata")
	testdir := filepath.Join(datadir, "testdir")
	subtestdir := filepath.Join(testdir, "subtestdir")
	testfile := filepath.Join(testdir, "testfile")

	rtest.OK(t, os.Mkdir(testdir, 0755))
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

	testRunBackup(t, "", []string{datadir}, opts, env.gopts)
	_, secondSnapshotID := lastSnapshot(snapshots, loadSnapshotMap(t, env.gopts))

	return env, cleanup, firstSnapshotID, secondSnapshotID
}

func TestDiff(t *testing.T) {
	env, cleanup, firstSnapshotID, secondSnapshotID := setupDiffRepo(t)
	defer cleanup()

	// quiet suppresses the diff output except for the summary
	env.gopts.Quiet = false
	_, err := testRunDiffOutput(env.gopts, "", secondSnapshotID)
	rtest.Assert(t, err != nil, "expected error on invalid snapshot id")

	out, err := testRunDiffOutput(env.gopts, firstSnapshotID, secondSnapshotID)
	rtest.OK(t, err)

	for _, pattern := range diffOutputRegexPatterns {
		r, err := regexp.Compile(pattern)
		rtest.Assert(t, err == nil, "failed to compile regexp %v", pattern)
		rtest.Assert(t, r.MatchString(out), "expected pattern %v in output, got\n%v", pattern, out)
	}

	// check quiet output
	env.gopts.Quiet = true
	outQuiet, err := testRunDiffOutput(env.gopts, firstSnapshotID, secondSnapshotID)
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
	out, err := testRunDiffOutput(env.gopts, firstSnapshotID, secondSnapshotID)
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
	rtest.Equals(t, 9, changes)
	rtest.Assert(t, stat.Added.Files == 2 && stat.Added.Dirs == 3 && stat.Added.DataBlobs == 2 &&
		stat.Removed.Files == 1 && stat.Removed.Dirs == 2 && stat.Removed.DataBlobs == 1 &&
		stat.ChangedFiles == 1, "unexpected statistics")

	// check quiet output
	env.gopts.Quiet = true
	outQuiet, err := testRunDiffOutput(env.gopts, firstSnapshotID, secondSnapshotID)
	rtest.OK(t, err)

	stat = DiffStatsContainer{}
	rtest.OK(t, json.Unmarshal([]byte(outQuiet), &stat))
	rtest.Assert(t, stat.Added.Files == 2 && stat.Added.Dirs == 3 && stat.Added.DataBlobs == 2 &&
		stat.Removed.Files == 1 && stat.Removed.Dirs == 2 && stat.Removed.DataBlobs == 1 &&
		stat.ChangedFiles == 1, "unexpected statistics")
	rtest.Assert(t, stat.SourceSnapshot == firstSnapshotID && stat.TargetSnapshot == secondSnapshotID, "unexpected snapshot ids")
}
