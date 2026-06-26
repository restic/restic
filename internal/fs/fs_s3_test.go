package fs

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	testS3Endpoint  = "145.170.34.56:9000"
	testS3AccessKey = "minioadmin"
	testS3SecretKey = "minioadmin"
	testS3UseHTTP   = true
)

func setupTestBucket(t *testing.T, bucket string) (*minio.Client, func()) {
	t.Helper()

	client, err := minio.New(testS3Endpoint, &minio.Options{
		Creds:  credentials.NewStatic(testS3AccessKey, testS3SecretKey, "", credentials.SignatureV4),
		Secure: !testS3UseHTTP,
	})
	if err != nil {
		t.Fatalf("minio.New: %v", err)
	}

	ctx := context.Background()

	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		t.Fatalf("BucketExists: %v", err)
	}
	if !exists {
		err = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
		if err != nil {
			t.Fatalf("MakeBucket: %v", err)
		}
	}

	cleanup := func() {
		// remove all objects then the bucket
		objects := client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true})
		for obj := range objects {
			if obj.Err == nil {
				_ = client.RemoveObject(ctx, bucket, obj.Key, minio.RemoveObjectOptions{})
			}
		}
		_ = client.RemoveBucket(ctx, bucket)
	}

	return client, cleanup
}

func uploadText(t *testing.T, client *minio.Client, bucket, key, content string) {
	t.Helper()
	ctx := context.Background()
	_, err := client.PutObject(ctx, bucket, key,
		io.NopCloser(strings.NewReader(content)),
		int64(len(content)),
		minio.PutObjectOptions{ContentType: "application/octet-stream"},
	)
	if err != nil {
		t.Fatalf("PutObject %s: %v", key, err)
	}
}

func TestS3FS_Lstat(t *testing.T) {
	bucket := "test-lfs-stat-" + time.Now().Format("20060102150405")
	client, cleanup := setupTestBucket(t, bucket)
	defer cleanup()

	uploadText(t, client, bucket, "dir/file.txt", "hello world")

	s3fs, err := NewS3FS(S3Config{
		Endpoint:  testS3Endpoint,
		UseHTTP:   testS3UseHTTP,
		Bucket:    bucket,
		Prefix:    "dir",
		AccessKey: testS3AccessKey,
		SecretKey: testS3SecretKey,
	})
	if err != nil {
		t.Fatalf("NewS3FS: %v", err)
	}

	// Lstat root
	fi, err := s3fs.Lstat("/")
	if err != nil {
		t.Fatalf("Lstat /: %v", err)
	}
	if fi.Mode.IsDir() != true {
		t.Errorf("root should be a directory")
	}

	// Lstat file
	fi, err = s3fs.Lstat("/file.txt")
	if err != nil {
		t.Fatalf("Lstat /file.txt: %v", err)
	}
	if fi.Size != 11 {
		t.Errorf("expected size 11, got %d", fi.Size)
	}
	if fi.Name != "file.txt" {
		t.Errorf("expected name 'file.txt', got %q", fi.Name)
	}

	// Lstat nonexistent
	_, err = s3fs.Lstat("/nonexistent.txt")
	if err == nil {
		t.Errorf("expected error for nonexistent file")
	}
}

func TestS3FS_OpenFile_Read(t *testing.T) {
	bucket := "test-lfs-open-" + time.Now().Format("20060102150405")
	client, cleanup := setupTestBucket(t, bucket)
	defer cleanup()

	uploadText(t, client, bucket, "data.txt", "hello s3 backup")

	s3fs, err := NewS3FS(S3Config{
		Endpoint:  testS3Endpoint,
		UseHTTP:   testS3UseHTTP,
		Bucket:    bucket,
		Prefix:    "",
		AccessKey: testS3AccessKey,
		SecretKey: testS3SecretKey,
	})
	if err != nil {
		t.Fatalf("NewS3FS: %v", err)
	}

	// Open file metadata only
	f, err := s3fs.OpenFile("/data.txt", O_RDONLY|O_NOFOLLOW, true)
	if err != nil {
		t.Fatalf("OpenFile metadataOnly: %v", err)
	}

	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if fi.Size != 15 {
		t.Errorf("expected size 15, got %d", fi.Size)
	}

	// MakeReadable and read content
	err = f.MakeReadable()
	if err != nil {
		t.Fatalf("MakeReadable: %v", err)
	}

	buf := make([]byte, 100)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read: %v", err)
	}
	content := string(buf[:n])
	if content != "hello s3 backup" {
		t.Errorf("expected 'hello s3 backup', got %q", content)
	}

	_ = f.Close()
}

func TestS3FS_Directory(t *testing.T) {
	bucket := "test-lfs-dir-" + time.Now().Format("20060102150405")
	client, cleanup := setupTestBucket(t, bucket)
	defer cleanup()

	uploadText(t, client, bucket, "a.txt", "file a")
	uploadText(t, client, bucket, "sub/b.txt", "file b")
	uploadText(t, client, bucket, "sub/c.txt", "file c")
	uploadText(t, client, bucket, "sub/deep/d.txt", "file d")

	s3fs, err := NewS3FS(S3Config{
		Endpoint:  testS3Endpoint,
		UseHTTP:   testS3UseHTTP,
		Bucket:    bucket,
		Prefix:    "",
		AccessKey: testS3AccessKey,
		SecretKey: testS3SecretKey,
	})
	if err != nil {
		t.Fatalf("NewS3FS: %v", err)
	}

	// List root directory
	f, err := s3fs.OpenFile("/", O_RDONLY|O_DIRECTORY, false)
	if err != nil {
		t.Fatalf("OpenFile /: %v", err)
	}
	entries, err := f.Readdirnames(-1)
	if err != nil {
		t.Fatalf("Readdirnames: %v", err)
	}
	_ = f.Close()

	// should have a.txt and sub
	foundFile := false
	foundDir := false
	for _, e := range entries {
		if e == "a.txt" {
			foundFile = true
		}
		if e == "sub" {
			foundDir = true
		}
	}
	if !foundFile {
		t.Errorf("expected to find a.txt in root entries: %v", entries)
	}
	if !foundDir {
		t.Errorf("expected to find sub/ in root entries: %v", entries)
	}

	// List sub directory
	f2, err := s3fs.OpenFile("/sub", O_RDONLY|O_DIRECTORY, false)
	if err != nil {
		t.Fatalf("OpenFile /sub: %v", err)
	}
	subEntries, err := f2.Readdirnames(-1)
	if err != nil {
		t.Fatalf("Readdirnames sub: %v", err)
	}
	_ = f2.Close()

	foundB := false
	foundC := false
	foundDeep := false
	for _, e := range subEntries {
		if e == "b.txt" {
			foundB = true
		}
		if e == "c.txt" {
			foundC = true
		}
		if e == "deep" {
			foundDeep = true
		}
	}
	if !foundB {
		t.Errorf("expected b.txt in sub entries: %v", subEntries)
	}
	if !foundC {
		t.Errorf("expected c.txt in sub entries: %v", subEntries)
	}
	if !foundDeep {
		t.Errorf("expected deep/ in sub entries: %v", subEntries)
	}
}

func TestS3FS_EmptyBucket(t *testing.T) {
	bucket := "test-lfs-empty-" + time.Now().Format("20060102150405")
	_, cleanup := setupTestBucket(t, bucket)
	defer cleanup()

	s3fs, err := NewS3FS(S3Config{
		Endpoint:  testS3Endpoint,
		UseHTTP:   testS3UseHTTP,
		Bucket:    bucket,
		Prefix:    "",
		AccessKey: testS3AccessKey,
		SecretKey: testS3SecretKey,
	})
	if err != nil {
		t.Fatalf("NewS3FS: %v", err)
	}

	// root should still work
	fi, err := s3fs.Lstat("/")
	if err != nil {
		t.Fatalf("Lstat /: %v", err)
	}
	if !fi.Mode.IsDir() {
		t.Errorf("root should be a directory")
	}

	// any file should not exist
	_, err = s3fs.Lstat("/nonexistent")
	if err == nil {
		t.Errorf("expected error for nonexistent file in empty bucket")
	}
}

func TestS3FS_Join(t *testing.T) {
	s3fs, err := NewS3FS(S3Config{
		Endpoint:  testS3Endpoint,
		UseHTTP:   testS3UseHTTP,
		Bucket:    "dummy",
		AccessKey: testS3AccessKey,
		SecretKey: testS3SecretKey,
	})
	if err != nil {
		t.Fatalf("NewS3FS: %v", err)
	}

	got := s3fs.Join("/a", "b", "c")
	if got != "/a/b/c" {
		t.Errorf("Join: expected '/a/b/c', got %q", got)
	}

	if s3fs.Separator() != "/" {
		t.Errorf("Separator: expected '/', got %q", s3fs.Separator())
	}

	if s3fs.IsAbs("/foo") != true {
		t.Errorf("IsAbs should return true")
	}

	cleaned := s3fs.Clean("/a/../b")
	if cleaned != "/b" {
		t.Errorf("Clean: expected '/b', got %q", cleaned)
	}
}

func TestS3FS_WithPrefix(t *testing.T) {
	bucket := "test-lfs-prefix-" + time.Now().Format("20060102150405")
	client, cleanup := setupTestBucket(t, bucket)
	defer cleanup()

	uploadText(t, client, bucket, "myprefix/file1.txt", "content1")
	uploadText(t, client, bucket, "myprefix/sub/file2.txt", "content2")

	s3fs, err := NewS3FS(S3Config{
		Endpoint:  testS3Endpoint,
		UseHTTP:   testS3UseHTTP,
		Bucket:    bucket,
		Prefix:    "myprefix",
		AccessKey: testS3AccessKey,
		SecretKey: testS3SecretKey,
	})
	if err != nil {
		t.Fatalf("NewS3FS: %v", err)
	}

	// List root with prefix
	f, err := s3fs.OpenFile("/", O_RDONLY|O_DIRECTORY, false)
	if err != nil {
		t.Fatalf("OpenFile /: %v", err)
	}
	entries, err := f.Readdirnames(-1)
	if err != nil {
		t.Fatalf("Readdirnames: %v", err)
	}
	_ = f.Close()

	foundFile1 := false
	foundSub := false
	for _, e := range entries {
		if e == "file1.txt" {
			foundFile1 = true
		}
		if e == "sub" {
			foundSub = true
		}
	}
	if !foundFile1 {
		t.Errorf("expected file1.txt: %v", entries)
	}
	if !foundSub {
		t.Errorf("expected sub/: %v", entries)
	}

	// Read file with prefix
	f2, err := s3fs.OpenFile("/file1.txt", O_RDONLY, false)
	if err != nil {
		t.Fatalf("OpenFile file1.txt: %v", err)
	}
	buf := make([]byte, 100)
	n, err := f2.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read: %v", err)
	}
	_ = f2.Close()

	if string(buf[:n]) != "content1" {
		t.Errorf("expected 'content1', got %q", string(buf[:n]))
	}
}

func TestS3FS_ToNode(t *testing.T) {
	bucket := "test-lfs-tonode-" + time.Now().Format("20060102150405")
	client, cleanup := setupTestBucket(t, bucket)
	defer cleanup()

	uploadText(t, client, bucket, "nodefile.txt", "test content")

	s3fs, err := NewS3FS(S3Config{
		Endpoint:  testS3Endpoint,
		UseHTTP:   testS3UseHTTP,
		Bucket:    bucket,
		Prefix:    "",
		AccessKey: testS3AccessKey,
		SecretKey: testS3SecretKey,
	})
	if err != nil {
		t.Fatalf("NewS3FS: %v", err)
	}

	f, err := s3fs.OpenFile("/nodefile.txt", O_RDONLY|O_NOFOLLOW, true)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}

	node, err := f.ToNode(false, func(string, ...any) {})
	if err != nil {
		t.Fatalf("ToNode: %v", err)
	}

	if node.Name != "nodefile.txt" {
		t.Errorf("expected name 'nodefile.txt', got %q", node.Name)
	}
	if node.Size != 12 {
		t.Errorf("expected size 12, got %d", node.Size)
	}
	if node.UID != uint32(os.Getuid()) {
		t.Errorf("expected UID %d, got %d", os.Getuid(), node.UID)
	}

	_ = f.Close()
}
