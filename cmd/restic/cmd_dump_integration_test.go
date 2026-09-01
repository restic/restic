package main

import (
	"archive/tar"
	"archive/zip"
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restic/restic/internal/global"
	rtest "github.com/restic/restic/internal/test"
)

// testRunDumpAssumeFailure runs the dump command and assumes it might fail
func testRunDumpAssumeFailure(t testing.TB, opts DumpOptions, gopts global.Options, args []string) (string, error) {
	output, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runDump(ctx, opts, gopts, args, gopts.Term)
	})
	return output.String(), err
}

// testRunDump runs the dump command and expects it to succeed
func testRunDump(t testing.TB, opts DumpOptions, gopts global.Options, args []string) string {
	output, err := testRunDumpAssumeFailure(t, opts, gopts, args)
	rtest.OK(t, err)
	return output
}

// TestDumpSingleFile tests dumping a single file to stdout and to a target file
func TestDumpSingleFile(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	// Create a test file with known content
	content := "This is a test file for dump command.\n"
	testFilePath := filepath.Join(env.testdata, "testfile.txt")
	rtest.OK(t, os.MkdirAll(filepath.Dir(testFilePath), 0755))
	rtest.OK(t, os.WriteFile(testFilePath, []byte(content), 0644))

	// Create backup
	opts := BackupOptions{}
	testRunBackup(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	// Get snapshot ID
	snapshotID := testListSnapshots(t, env.gopts, 1)[0]

	// Test dumping to stdout
	dumpOpts := DumpOptions{Archive: "tar"} // Set default archive format to 'tar'

	testFileSnapshotPath := path.Join(filepath.Base(env.testdata), "testfile.txt")
	args := []string{snapshotID.String(), testFileSnapshotPath}

	output := testRunDump(t, dumpOpts, env.gopts, args)

	// Verify the content
	rtest.Assert(t, output == content, "Expected file content %q, got %q", content, output)

	// Test dumping to a target file
	targetFile := filepath.Join(env.base, "dump-target.txt")
	dumpOpts.Target = targetFile
	testRunDump(t, dumpOpts, env.gopts, args)

	// Verify the target file content
	targetContent, err := os.ReadFile(targetFile)
	rtest.OK(t, err)
	rtest.Assert(t, string(targetContent) == content, "Expected file content %q, got %q", content, string(targetContent))
}

// TestDumpLatest tests dumping with the "latest" snapshot ID
func TestDumpLatest(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	// Create a test file with known content
	content := "This is a test file for latest snapshot dump.\n"
	testfile := filepath.Join(env.testdata, "testfile.txt")
	rtest.OK(t, os.MkdirAll(filepath.Dir(testfile), 0755))
	rtest.OK(t, os.WriteFile(testfile, []byte(content), 0644))

	// Create backup
	opts := BackupOptions{}
	testRunBackup(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	// Test dumping with "latest" snapshot ID
	dumpOpts := DumpOptions{Archive: "tar"} // Set default archive format to 'tar'

	args := []string{"latest", path.Join(filepath.Base(env.testdata), "testfile.txt")}

	output := testRunDump(t, dumpOpts, env.gopts, args)

	// Verify the content
	rtest.Assert(t, output == content, "Expected file content %q, got %q", content, output)
}

// TestDumpDirArchive tests dumping a directory as an archive
func TestDumpDirArchive(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	// Create a test directory with multiple files
	testdir := filepath.Join(env.testdata, "testdir")
	rtest.OK(t, os.MkdirAll(testdir, 0755))

	files := map[string]string{
		"file1.txt":        "Content of file 1\n",
		"file2.txt":        "Content of file 2\n",
		"subdir/file3.txt": "Content of file 3 in subdirectory\n",
	}

	for fname, content := range files {
		p := filepath.Join(testdir, fname)
		rtest.OK(t, os.MkdirAll(filepath.Dir(p), 0755))
		rtest.OK(t, os.WriteFile(p, []byte(content), 0644))
	}

	// Create backup
	opts := BackupOptions{}
	testRunBackup(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	// Get snapshot ID
	snapshotID := testListSnapshots(t, env.gopts, 1)[0]

	// Test dumping directory as tar archive
	targetFile := filepath.Join(env.base, "dump-dir.tar")
	dumpOpts := DumpOptions{
		Archive: "tar",
		Target:  targetFile,
	}

	args := []string{snapshotID.String(), path.Join(filepath.Base(env.testdata), "testdir")}
	testRunDump(t, dumpOpts, env.gopts, args)

	// Verify tar archive
	f, err := os.Open(targetFile)
	rtest.OK(t, err)
	defer func() {
		_ = f.Close()
	}()

	tr := tar.NewReader(f)
	foundFiles := make(map[string]bool)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		rtest.OK(t, err)

		if hdr.Typeflag == tar.TypeReg {
			content := make([]byte, hdr.Size)
			_, err := io.ReadFull(tr, content)
			rtest.OK(t, err)

			// Extract path for comparison after removing any prefix
			relativePath := hdr.Name
			// Try various prefixes that might be present
			prefixes := []string{"testdir/", "testdata/testdir/", filepath.Base(env.testdata) + "/testdir/"}
			for _, prefix := range prefixes {
				if strings.HasPrefix(relativePath, prefix) {
					relativePath = relativePath[len(prefix):]
					break
				}
			}

			expectedContent, ok := files[relativePath]
			rtest.Assert(t, ok, "Unexpected file in archive: %v", relativePath)
			rtest.Assert(t, string(content) == expectedContent,
				"Content mismatch for file %s: expected %q, got %q",
				relativePath, expectedContent, string(content))

			foundFiles[relativePath] = true
		}
	}

	// Check that all files were found
	for fname := range files {
		rtest.Assert(t, foundFiles[fname], "File %s was not found in the archive", fname)
	}
}

// TestDumpArchiveZip tests dumping a directory as a zip archive
func TestDumpArchiveZip(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	// Create a test directory with multiple files
	testdir := filepath.Join(env.testdata, "testdir")
	rtest.OK(t, os.MkdirAll(testdir, 0755))

	files := map[string]string{
		"file1.txt":        "Content of file 1\n",
		"file2.txt":        "Content of file 2\n",
		"subdir/file3.txt": "Content of file 3 in subdirectory\n",
	}

	for fname, content := range files {
		p := filepath.Join(testdir, fname)
		rtest.OK(t, os.MkdirAll(filepath.Dir(p), 0755))
		rtest.OK(t, os.WriteFile(p, []byte(content), 0644))
	}

	// Create backup
	opts := BackupOptions{}
	testRunBackup(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	// Get snapshot ID
	snapshotID := testListSnapshots(t, env.gopts, 1)[0]

	// Test dumping directory as zip archive
	targetFile := filepath.Join(env.base, "dump-dir.zip")
	dumpOpts := DumpOptions{
		Archive: "zip",
		Target:  targetFile,
	}

	args := []string{snapshotID.String(), path.Join(filepath.Base(env.testdata), "testdir")}
	testRunDump(t, dumpOpts, env.gopts, args)

	// Verify zip archive
	r, err := zip.OpenReader(targetFile)
	rtest.OK(t, err)
	defer func() {
		_ = r.Close()
	}()

	foundFiles := make(map[string]bool)

	for _, f := range r.File {
		if !f.FileInfo().IsDir() {
			rc, err := f.Open()
			rtest.OK(t, err)
			defer func() {
				_ = rc.Close()
			}()

			content, err := io.ReadAll(rc)
			rtest.OK(t, err)

			// Extract path for comparison after removing any prefix
			relativePath := f.Name
			// Try various prefixes that might be present
			prefixes := []string{"testdir/", "testdata/testdir/", filepath.Base(env.testdata) + "/testdir/"}
			for _, prefix := range prefixes {
				if strings.HasPrefix(relativePath, prefix) {
					relativePath = relativePath[len(prefix):]
					break
				}
			}

			expectedContent, ok := files[relativePath]
			rtest.Assert(t, ok, "Unexpected file in archive: %v", relativePath)
			rtest.Assert(t, string(content) == expectedContent,
				"Content mismatch for file %s: expected %q, got %q",
				relativePath, expectedContent, string(content))

			foundFiles[relativePath] = true
		}
	}

	// Check that all files were found
	for fname := range files {
		rtest.Assert(t, foundFiles[fname], "File %s was not found in the archive", fname)
	}
}

// TestDumpSubfolderPath tests dumping with subfolder syntax (snapshotID:subfolder)
func TestDumpSubfolderPath(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	// Create a test directory with nested structure
	testdir := filepath.Join(env.testdata, "rootdir")
	subdir := filepath.Join(testdir, "subdir")
	rtest.OK(t, os.MkdirAll(subdir, 0755))

	subContent := "Content of file in subdirectory\n"
	subFile := filepath.Join(subdir, "subfile.txt")
	rtest.OK(t, os.WriteFile(subFile, []byte(subContent), 0644))

	// Create backup
	opts := BackupOptions{}
	testRunBackup(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	// Get snapshot ID
	snapshotID := testListSnapshots(t, env.gopts, 1)[0]

	// Test dumping using subfolder syntax
	dumpOpts := DumpOptions{Archive: "tar"}
	targetFilePath := filepath.Join(env.base, "subfolder-dump.txt")
	dumpOpts.Target = targetFilePath

	// Create the path with the subfolder syntax
	pathWithSubfolder := snapshotID.String() + ":" + path.Join(filepath.Base(env.testdata), "rootdir", "subdir")
	args := []string{pathWithSubfolder, "subfile.txt"}

	testRunDump(t, dumpOpts, env.gopts, args)

	// Verify the file content
	content, err := os.ReadFile(targetFilePath)
	rtest.OK(t, err)
	rtest.Assert(t, string(content) == subContent,
		"Expected content %q, got %q", subContent, string(content))
}

// TestDumpErrorCases tests error conditions for the dump command
func TestDumpErrorCases(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	// Create and backup a test file
	testfile := filepath.Join(env.testdata, "testfile.txt")
	rtest.OK(t, os.MkdirAll(filepath.Dir(testfile), 0755))
	rtest.OK(t, os.WriteFile(testfile, []byte("test content"), 0644))

	opts := BackupOptions{}
	testRunBackup(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	snapshotID := testListSnapshots(t, env.gopts, 1)[0]

	// Test case 1: Invalid snapshot ID
	dumpOpts := DumpOptions{}
	_, err := testRunDumpAssumeFailure(t, dumpOpts, env.gopts, []string{"invalid-id", "testfile.txt"})
	rtest.Assert(t, err != nil, "Expected error for invalid snapshot ID, got nil")

	// Test case 2: Non-existent file
	_, err = testRunDumpAssumeFailure(t, dumpOpts, env.gopts, []string{snapshotID.String(), "non-existent-file.txt"})
	rtest.Assert(t, err != nil, "Expected error for non-existent file, got nil")

	// Test case 3: Invalid archive format
	dumpOpts.Archive = "invalid"
	_, err = testRunDumpAssumeFailure(t, dumpOpts, env.gopts, []string{snapshotID.String(), filepath.Join(filepath.Base(env.testdata), "testfile.txt")})
	rtest.Assert(t, err != nil, "Expected error for invalid archive format, got nil")

	// Test case 4: No arguments provided
	_, err = testRunDumpAssumeFailure(t, DumpOptions{}, env.gopts, []string{})
	rtest.Assert(t, err != nil, "Expected error for no arguments, got nil")
}
