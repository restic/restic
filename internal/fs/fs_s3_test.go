package fs_test

import (
	"context"
	"errors"
	"io"
	"os"
	"sort"
	"testing"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/fs"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/test/s3testutil"
)

func TestS3SourceWithMinio(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/fs.TestS3SourceWithMinio")
		}
	}()

	if s3testutil.SkipIfNotFoundMinio(t) {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testObjects := map[string][]byte{
		"file1.txt":           []byte("hello world"),
		"dir1/file2.txt":      []byte("content of file2"),
		"dir1/file3.txt":      []byte("content of file3"),
		"dir1/sub/file4.txt":  []byte("content of file4"),
		"dir2/sub2/file5.txt": []byte("content of file5"),
	}

	srv := s3testutil.StartMinio(ctx, t, "test-fs-s3", testObjects)
	srv.SetAWSEnv(t)
	bucketName := srv.Bucket

	src := &fs.S3Source{}
	rtest.OK(t, src.WarmingUp([]string{"/" + bucketName}))

	t.Run("Lstat/file", func(t *testing.T) {
		fi, err := src.Lstat("/" + bucketName + "/file1.txt")
		rtest.OK(t, err)
		rtest.Assert(t, !fi.Mode.IsDir(), "expected file, got dir")
		rtest.Assert(t, fi.Size == int64(len(testObjects["file1.txt"])), "wrong size: got %d, want %d", fi.Size, len(testObjects["file1.txt"]))
		rtest.Assert(t, fi.Name == "file1.txt", "wrong name: got %q, want %q", fi.Name, "file1.txt")
	})

	t.Run("Lstat/dir", func(t *testing.T) {
		fi, err := src.Lstat("/" + bucketName + "/dir1")
		rtest.OK(t, err)
		rtest.Assert(t, fi.Mode.IsDir(), "expected dir, got file")
		rtest.Assert(t, fi.Name == "dir1", "wrong name: got %q, want %q", fi.Name, "dir1")
	})
	t.Run("Lstat/deep dir", func(t *testing.T) {
		fi, err := src.Lstat("/" + bucketName + "/dir1/sub")
		rtest.OK(t, err)
		rtest.Assert(t, fi.Mode.IsDir(), "expected dir, got file")
		rtest.Assert(t, fi.Name == "sub", "wrong name: got %q, want %q", fi.Name, "sub")
	})

	t.Run("Lstat/bucket-root", func(t *testing.T) {
		fi, err := src.Lstat("/" + bucketName)
		rtest.OK(t, err)
		rtest.Assert(t, fi.Mode.IsDir(), "bucket root should be a dir")
	})

	t.Run("Lstat/notexist", func(t *testing.T) {
		_, err := src.Lstat("/" + bucketName + "/notexist.txt")
		rtest.Assert(t, errors.Is(err, os.ErrNotExist), "expected ErrNotExist, got %v", err)
	})

	t.Run("OpenFile/read", func(t *testing.T) {
		f, err := src.OpenFile("/"+bucketName+"/file1.txt", 0, false)
		rtest.OK(t, err)
		defer f.Close()

		got, err := io.ReadAll(f)
		rtest.OK(t, err)
		rtest.Equals(t, testObjects["file1.txt"], got)
	})

	t.Run("OpenFile/dir/readdirnames", func(t *testing.T) {
		f, err := src.OpenFile("/"+bucketName+"/dir1", 0, false)
		rtest.OK(t, err)
		defer f.Close()

		names, err := f.Readdirnames(-1)
		rtest.OK(t, err)
		sort.Strings(names)
		rtest.Equals(t, []string{"file2.txt", "file3.txt", "sub"}, names)
	})

	t.Run("OpenFile/notexist", func(t *testing.T) {
		_, err := src.OpenFile("/"+bucketName+"/notexist.txt", 0, false)
		rtest.Assert(t, errors.Is(err, os.ErrNotExist), "expected ErrNotExist, got %v", err)
	})

	t.Run("MakeReadable/file", func(t *testing.T) {
		f, err := src.OpenFile("/"+bucketName+"/file1.txt", 0, true)
		rtest.OK(t, err)
		defer f.Close()

		rtest.OK(t, f.MakeReadable())
		got, err := io.ReadAll(f)
		rtest.OK(t, err)
		rtest.Equals(t, testObjects["file1.txt"], got)
	})

	t.Run("MakeReadable/dir", func(t *testing.T) {
		f, err := src.OpenFile("/"+bucketName+"/dir1", 0, true)
		rtest.OK(t, err)
		defer f.Close()

		rtest.OK(t, f.MakeReadable())
		names, err := f.Readdirnames(-1)
		rtest.OK(t, err)
		sort.Strings(names)
		rtest.Equals(t, []string{"file2.txt", "file3.txt", "sub"}, names)
	})

	t.Run("ToNode/file", func(t *testing.T) {
		f, err := src.OpenFile("/"+bucketName+"/file1.txt", 0, true)
		rtest.OK(t, err)
		defer f.Close()

		node, err := f.ToNode(false, func(string, ...any) {})
		rtest.OK(t, err)
		rtest.Assert(t, node.Type == data.NodeTypeFile, "expected NodeTypeFile, got %v", node.Type)
		rtest.Assert(t, node.Size == uint64(len(testObjects["file1.txt"])), "wrong node size: got %d, want %d", node.Size, len(testObjects["file1.txt"]))
		rtest.Assert(t, node.Name == "file1.txt", "wrong node name: got %q", node.Name)
	})

	t.Run("ToNode/dir", func(t *testing.T) {
		f, err := src.OpenFile("/"+bucketName+"/dir1", 0, true)
		rtest.OK(t, err)
		defer f.Close()

		node, err := f.ToNode(false, func(string, ...any) {})
		rtest.OK(t, err)
		rtest.Assert(t, node.Type == data.NodeTypeDir, "expected NodeTypeDir, got %v", node.Type)
		rtest.Assert(t, node.Name == "dir1", "wrong node name: got %q", node.Name)
	})

	t.Run("Stat/file", func(t *testing.T) {
		f, err := src.OpenFile("/"+bucketName+"/dir1/file2.txt", 0, true)
		rtest.OK(t, err)
		defer f.Close()

		fi, err := f.Stat()
		rtest.OK(t, err)
		rtest.Assert(t, fi.Size == int64(len(testObjects["dir1/file2.txt"])), "wrong stat size")
		rtest.Assert(t, fi.Name == "file2.txt", "wrong stat name: got %q", fi.Name)
		rtest.Assert(t, !fi.ModTime.IsZero(), "ModTime should not be zero for files")
	})

	t.Run("PathMethods", func(t *testing.T) {
		rtest.Equals(t, "/", src.Separator())
		rtest.Equals(t, "/a/b", src.Join("/a", "b"))
		rtest.Assert(t, src.IsAbs("/a/b"), "expected /a/b to be absolute")
		rtest.Assert(t, !src.IsAbs("a/b"), "expected a/b to be relative")

		abs, err := src.Abs("a/b")
		rtest.OK(t, err)
		rtest.Equals(t, "/a/b", abs)

		rtest.Equals(t, "/a/b", src.Clean("/a/b/"))
		rtest.Equals(t, "b", src.Base("/a/b"))
		rtest.Equals(t, "/a", src.Dir("/a/b"))
		rtest.Equals(t, "", src.VolumeName("/a/b"))
	})

	t.Run("WarmingUp/many prefix", func(t *testing.T) {
		s3SrcMany := &fs.S3Source{}
		rtest.OK(t, s3SrcMany.WarmingUp([]string{"/" + bucketName + "/dir1", "/" + bucketName + "/dir2"}))

		_, err := s3SrcMany.Lstat("/" + bucketName + "/dir1/file2.txt")
		rtest.OK(t, err)

		_, err = s3SrcMany.Lstat("/" + bucketName + "/dir1/sub/file4.txt")
		rtest.OK(t, err)

		_, err = s3SrcMany.Lstat("/" + bucketName + "/dir2/sub2/file5.txt")
		rtest.OK(t, err)

		_, err = s3SrcMany.Lstat("/" + bucketName + "/dir2/sub2/")
		rtest.OK(t, err)

		_, err = s3SrcMany.Lstat("/" + bucketName + "/file1.txt")
		rtest.Assert(t, errors.Is(err, os.ErrNotExist), "file1.txt should not be visible with dir1 prefix, got err: %v", err)

		_, err = s3SrcMany.Lstat("/" + bucketName + "/file5.txt")
		rtest.Assert(t, errors.Is(err, os.ErrNotExist), "file5.txt should not be visible with dir2 prefix, got err: %v", err)
	})
}
