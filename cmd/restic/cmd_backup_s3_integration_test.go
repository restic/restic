package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/restic/restic/internal/global"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/test/s3testutil"
)

// TestS3BackupIntegration starts a local minio server, seeds it with objects,
// and runs a set of end-to-end backup tests that use S3 as the source and a
// local directory as the repository.
func TestS3BackupIntegration(t *testing.T) {
	if _, err := exec.LookPath("minio"); err != nil {
		t.Skip(err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := s3testutil.FreeAddr(t)
	key, secret := s3testutil.NewCredentials(t)
	cleanup := s3testutil.RunMinio(ctx, t, rtest.TempDir(t), key, secret, addr)
	defer cleanup()

	bucketName := fmt.Sprintf("backup-test-%d", time.Now().UnixNano())
	client := s3testutil.NewClient(t, addr, key, secret)
	rtest.OK(t, client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{}))

	testObjects := map[string][]byte{
		"file1.txt":           []byte("hello world"),
		"dir1/file2.txt":      []byte("content of file2"),
		"dir1/file3.txt":      []byte("content of file3"),
		"dir1/sub/file4.txt":  []byte("content of file4"),
		"dir2/sub2/file5.txt": []byte("content of file5"),
	}
	s3testutil.UploadObjects(t, ctx, client, bucketName, testObjects)

	t.Setenv("AWS_ENDPOINT_URL", "http://"+addr)
	t.Setenv("AWS_ACCESS_KEY_ID", key)
	t.Setenv("AWS_SECRET_ACCESS_KEY", secret)

	s3Target := "s3://" + bucketName

	// newRepo returns gopts for a fresh local repository. It disables the
	// list-once backend hook because several tests list a filetype more than once.
	newRepo := func(t *testing.T) global.Options {
		env, cleanup := withTestEnvironment(t)
		t.Cleanup(cleanup)
		env.gopts.BackendTestHook = nil
		testRunInit(t, env.gopts)
		return env.gopts
	}

	t.Run("RestoreBackup", func(t *testing.T) {
		gopts := newRepo(t)
		testRunBackup(t, "", []string{s3Target}, BackupOptions{}, gopts)

		restoreDir := filepath.Join(rtest.TempDir(t), "restore")
		testRunRestore(t, gopts, restoreDir, "latest")

		for key, want := range testObjects {
			got, err := os.ReadFile(filepath.Join(restoreDir, bucketName, filepath.FromSlash(key)))
			rtest.OK(t, err)
			rtest.Equals(t, want, got, fmt.Sprintf("content mismatch for %s", key))
		}
	})

	t.Run("AddNewObject", func(t *testing.T) {
		gopts := newRepo(t)
		testRunBackup(t, "", []string{s3Target}, BackupOptions{}, gopts)
		packs1 := listPacks(gopts, t)

		testRunBackup(t, "", []string{s3Target}, BackupOptions{}, gopts)
		testListSnapshots(t, gopts, 2)
		testRunCheck(t, gopts)

		// Data packs must not grow: identical content should be fully deduplicated.
		packs2 := listPacks(gopts, t)
		rtest.Assert(t, len(packs2) == len(packs1),
			"expected same number of packs after second identical backup, got %d (was %d)", len(packs2), len(packs1))

		// Adding a new object with fresh content must produce more packs.
		newKey := "dir1/file5.txt"
		newContent := []byte("brand new content that has never been backed up before")
		s3testutil.UploadObjects(t, ctx, client, bucketName, map[string][]byte{newKey: newContent})
		// Remove the extra object again so the shared bucket is left untouched.
		defer func() {
			rtest.OK(t, client.RemoveObject(ctx, bucketName, newKey, minio.RemoveObjectOptions{}))
		}()

		testRunBackup(t, "", []string{s3Target}, BackupOptions{}, gopts)
		testListSnapshots(t, gopts, 3)
		testRunCheck(t, gopts)

		packs3 := listPacks(gopts, t)
		rtest.Assert(t, len(packs3) > len(packs2),
			"expected more packs after backing up a new object, got %d (was %d)", len(packs3), len(packs2))

		//check all file
		restoreDir := filepath.Join(rtest.TempDir(t), "restore")
		testRunRestore(t, gopts, restoreDir, "latest")

		localTestObjects := maps.Clone(testObjects)
		localTestObjects[newKey] = newContent
		for key, want := range localTestObjects {
			got, err := os.ReadFile(filepath.Join(restoreDir, bucketName, filepath.FromSlash(key)))
			rtest.OK(t, err)
			rtest.Equals(t, want, got, fmt.Sprintf("content mismatch for %s", key))
		}
	})
}
