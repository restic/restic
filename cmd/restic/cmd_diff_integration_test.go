package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/restic/restic/internal/global"
	rtest "github.com/restic/restic/internal/test"
)

func testRunDiffOutput(t testing.TB, gopts global.Options, firstSnapshotID string, secondSnapshotID string) (string, error) {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		opts := DiffOptions{
			ShowMetadata: false,
		}
		return runDiff(ctx, opts, gopts, []string{firstSnapshotID, secondSnapshotID}, gopts.Term)
	})
	return buf.String(), err
}

func testRunDiffOutputWithOpts(t testing.TB, opts DiffOptions, gopts global.Options, hostA string, hostB string,
) (*bytes.Buffer, error) {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runDiff(ctx, opts, gopts, []string{hostA, hostB}, gopts.Term)
	})
	return buf, err
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
	_, err := testRunDiffOutput(t, env.gopts, "", secondSnapshotID)
	rtest.Assert(t, err != nil, "expected error on invalid snapshot id")

	out, err := testRunDiffOutput(t, env.gopts, firstSnapshotID, secondSnapshotID)
	rtest.OK(t, err)

	for _, pattern := range diffOutputRegexPatterns {
		r, err := regexp.Compile(pattern)
		rtest.Assert(t, err == nil, "failed to compile regexp %v", pattern)
		rtest.Assert(t, r.MatchString(out), "expected pattern %v in output, got\n%v", pattern, out)
	}

	// check quiet output
	env.gopts.Quiet = true
	outQuiet, err := testRunDiffOutput(t, env.gopts, firstSnapshotID, secondSnapshotID)
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
	out, err := testRunDiffOutput(t, env.gopts, firstSnapshotID, secondSnapshotID)
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
	outQuiet, err := testRunDiffOutput(t, env.gopts, firstSnapshotID, secondSnapshotID)
	rtest.OK(t, err)

	stat = DiffStatsContainer{}
	rtest.OK(t, json.Unmarshal([]byte(outQuiet), &stat))
	rtest.Assert(t, stat.Added.Files == 2 && stat.Added.Dirs == 3 && stat.Added.DataBlobs == 2 &&
		stat.Removed.Files == 1 && stat.Removed.Dirs == 2 && stat.Removed.DataBlobs == 1 &&
		stat.ChangedFiles == 1, "unexpected statistics")
	rtest.Assert(t, stat.SourceSnapshot == firstSnapshotID && stat.TargetSnapshot == secondSnapshotID, "unexpected snapshot ids")
}

func TestDiffHostsJSON(t *testing.T) {
	env, cleanup, firstSnapshotID, secondSnapshotID := setupDiffRepo(t)
	defer cleanup()

	// rename both hosts, since the default seems to be ""
	optsRewrite := RewriteOptions{
		Metadata: snapshotMetadataArgs{
			Hostname: "end-of-universe",
		},
		Forget: true,
	}
	rtest.OK(t, testRunRewriteWithOpts(t, optsRewrite, env.gopts, []string{firstSnapshotID}))

	optsRewrite = RewriteOptions{
		Metadata: snapshotMetadataArgs{
			Hostname: "Douglas-Adams",
		},
		Forget: true,
	}
	rtest.OK(t, testRunRewriteWithOpts(t, optsRewrite, env.gopts, []string{secondSnapshotID}))

	// extract hostnames from first and second snapshot
	snapshotIDs := testListSnapshots(t, env.gopts, 2)
	hostnameA := testLoadSnapshot(t, env.gopts, snapshotIDs[0]).Hostname
	hostnameB := testLoadSnapshot(t, env.gopts, snapshotIDs[1]).Hostname

	env.gopts.Quiet = false
	env.gopts.JSON = true
	optsDiff := DiffOptions{
		diffHosts: true,
	}
	// the only file which has not changed between the two snapshots is
	// "testdir/testfile", which 256KiB long
	output, err := testRunDiffOutputWithOpts(t, optsDiff, env.gopts, hostnameA, hostnameB)
	rtest.OK(t, err)

	// get hold of JSON output
	statsDiffHosts := StatDiffHosts{}
	rtest.OK(t, json.Unmarshal(output.Bytes(), &statsDiffHosts))

	rtest.Assert(t, statsDiffHosts.MessageType == "host_differences", "expected `host_differences`, got %q",
		statsDiffHosts.MessageType)
	rtest.Assert(t, statsDiffHosts.LeftStats.SnapshotCount == 1 && statsDiffHosts.RightStats.SnapshotCount == 1,
		"expected one snapshot per host, got left host=%d and right host=%d snapshots",
		statsDiffHosts.LeftStats.SnapshotCount, statsDiffHosts.RightStats.SnapshotCount)
	rtest.Assert(t, statsDiffHosts.CommonStats.DataBlobCount == 1 && statsDiffHosts.CommonStats.DataBlobSize == 1<<18,
		"expected common data blob count == 1 and datablobsize == 256KiB, got %d and %d",
		statsDiffHosts.CommonStats.DataBlobCount, statsDiffHosts.CommonStats.DataBlobSize)
}
