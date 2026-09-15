package fuse

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

const testTimeTemplate = "2006-01-02T15-04-05"

func readdirNames(t testing.TB, fs *WinFS, path string) []string {
	t.Helper()
	f, err := fs.OpenFile(path, os.O_RDONLY, 0)
	rtest.OK(t, err)
	defer func() { rtest.OK(t, f.Close()) }()

	infos, err := f.Readdir(-1)
	rtest.OK(t, err)
	names := make([]string, 0, len(infos))
	for _, fi := range infos {
		names = append(names, fi.Name())
	}
	return names
}

func loadContent(t testing.TB, repo restic.Repository, ids restic.IDs) []byte {
	t.Helper()
	var content []byte
	for _, id := range ids {
		buf, err := repo.LoadBlob(context.TODO(), restic.BlobHandle{Type: restic.DataBlob, ID: id}, nil)
		rtest.OK(t, err)
		content = append(content, buf...)
	}
	return content
}

func TestWinFSDirectoryStructure(t *testing.T) {
	repo := repository.TestRepository(t)
	timestamp, err := time.Parse(time.RFC3339, "2017-01-24T10:42:56+01:00")
	rtest.OK(t, err)
	sn := data.TestCreateSnapshot(t, repo, timestamp, 2)

	fs := NewWinFS(t.Context(), repo, Config{TimeTemplate: testTimeTemplate})

	root, err := fs.Stat(`\`)
	rtest.OK(t, err)
	rtest.Assert(t, root.IsDir(), "root is not a directory")
	rtest.Equals(t, []string{"hosts", "ids", "snapshots", "tags"}, readdirNames(t, fs, `\`))

	snapshotTime := timestamp.Format(testTimeTemplate)
	rtest.Equals(t, []string{snapshotTime, "latest"}, readdirNames(t, fs, `\snapshots`))

	// data.TestCreateSnapshot uses hostname "foo" and tag "test"
	snapshotPath := `\ids\` + sn.ID().Str()
	for _, p := range []string{snapshotPath, `\snapshots\` + snapshotTime, `\snapshots\latest`, `\hosts\foo\latest`, `\tags\test\latest`} {
		fi, err := fs.Stat(p)
		rtest.OK(t, err)
		rtest.Assert(t, fi.IsDir(), "%v is not a directory", p)
		rtest.Assert(t, fi.ModTime().Equal(sn.Time), "%v: expected mod time %v, got %v", p, sn.Time, fi.ModTime())
	}

	tree, err := data.LoadTree(context.TODO(), repo, *sn.Tree)
	rtest.OK(t, err)
	var expNames []string
	var fileNode *data.Node
	for item := range tree {
		rtest.OK(t, item.Error)
		expNames = append(expNames, item.Node.Name)
		if fileNode == nil && item.Node.Type == data.NodeTypeFile && item.Node.Size > 0 {
			fileNode = item.Node
		}
	}
	sort.Strings(expNames)
	rtest.Equals(t, expNames, readdirNames(t, fs, snapshotPath))
	rtest.Equals(t, expNames, readdirNames(t, fs, `\snapshots\latest`))
	rtest.Assert(t, fileNode != nil, "no non-empty file in test snapshot")

	filePath := snapshotPath + `\` + fileNode.Name
	fi, err := fs.Stat(filePath)
	rtest.OK(t, err)
	rtest.Assert(t, !fi.IsDir(), "%v is a directory", filePath)
	rtest.Equals(t, fileNode.Name, fi.Name())
	rtest.Equals(t, int64(fileNode.Size), fi.Size())
	rtest.Equals(t, os.FileMode(0444), fi.Mode())

	content := loadContent(t, repo, fileNode.Content)
	f, err := fs.OpenFile(filePath, os.O_RDONLY, 0)
	rtest.OK(t, err)
	defer func() { rtest.OK(t, f.Close()) }()

	buf := make([]byte, len(content))
	n, err := f.ReadAt(buf, 0)
	rtest.OK(t, err)
	rtest.Equals(t, len(content), n)
	rtest.Assert(t, bytes.Equal(content, buf), "wrong data returned")

	// reading at the end of the file
	n, err = f.ReadAt(buf[:1], int64(len(content)))
	rtest.Equals(t, 0, n)
	rtest.Assert(t, errors.Is(err, io.EOF), "expected io.EOF, got %v", err)

	// short read at the end of the file
	n, err = f.ReadAt(buf[:10], int64(len(content)-3))
	rtest.Equals(t, 3, n)
	rtest.Assert(t, errors.Is(err, io.EOF), "expected io.EOF, got %v", err)
	rtest.Assert(t, bytes.Equal(content[len(content)-3:], buf[:3]), "wrong data returned")

	// files within a directory are not directories
	_, err = fs.Stat(filePath + `\foo`)
	rtest.Assert(t, err != nil, "expected error when using a file as directory")
}

// TestWinFSFileRead verifies reading across blob boundaries.
func TestWinFSFileRead(t *testing.T) {
	repo := repository.TestRepository(t)
	timestamp, err := time.Parse(time.RFC3339, "2017-01-24T10:42:56+01:00")
	rtest.OK(t, err)
	sn := data.TestCreateSnapshot(t, repo, timestamp, 2)

	// build a file from all blobs in the snapshot to have multiple blobs
	tree, err := data.LoadTree(context.TODO(), repo, *sn.Tree)
	rtest.OK(t, err)
	var content restic.IDs
	for item := range tree {
		rtest.OK(t, item.Error)
		content = append(content, item.Node.Content...)
	}
	rtest.Assert(t, len(content) > 1, "expected more than one blob, got %d", len(content))
	memfile := loadContent(t, repo, content)

	fs := NewWinFS(t.Context(), repo, Config{TimeTemplate: testTimeTemplate})
	f, err := fs.openFile(&data.Node{Name: "foo", Type: data.NodeTypeFile, Content: content})
	rtest.OK(t, err)

	fi, err := f.Stat()
	rtest.OK(t, err)
	rtest.Equals(t, int64(len(memfile)), fi.Size())

	rnd := rand.New(rand.NewSource(23))
	for i := range 200 {
		offset := rnd.Intn(len(memfile))
		length := rnd.Intn(len(memfile)-offset) + 1

		buf := make([]byte, length)
		n, err := f.ReadAt(buf, int64(offset))
		rtest.OK(t, err)
		rtest.Equals(t, length, n)
		if !bytes.Equal(memfile[offset:offset+length], buf) {
			t.Errorf("test %d failed, wrong data returned (offset %v, length %v)", i, offset, length)
		}
	}

	// sequential reads
	f.offset = 0
	var seq []byte
	buf := make([]byte, 1000)
	for {
		n, err := f.Read(buf)
		seq = append(seq, buf[:n]...)
		if errors.Is(err, io.EOF) {
			break
		}
		rtest.OK(t, err)
	}
	rtest.Assert(t, bytes.Equal(memfile, seq), "sequential read returned wrong data")
}

func TestWinFSReadOnly(t *testing.T) {
	repo := repository.TestRepository(t)
	data.TestCreateSnapshot(t, repo, time.Unix(1460289341, 207401672), 0)

	fs := NewWinFS(t.Context(), repo, Config{TimeTemplate: testTimeTemplate})

	for _, flag := range []int{os.O_WRONLY, os.O_RDWR, os.O_RDONLY | os.O_CREATE, os.O_RDONLY | os.O_TRUNC, os.O_RDONLY | os.O_APPEND} {
		_, err := fs.OpenFile(`\snapshots`, flag, 0)
		rtest.Assert(t, errors.Is(err, os.ErrPermission), "flag %v: expected permission error, got %v", flag, err)
	}

	rtest.Assert(t, errors.Is(fs.Mkdir(`\foo`, 0755), os.ErrPermission), "expected permission error for mkdir")
	rtest.Assert(t, errors.Is(fs.Rename(`\snapshots`, `\foo`), os.ErrPermission), "expected permission error for rename")
	rtest.Assert(t, errors.Is(fs.Remove(`\snapshots`), os.ErrPermission), "expected permission error for remove")

	f, err := fs.OpenFile(`\snapshots`, os.O_RDONLY, 0)
	rtest.OK(t, err)
	_, err = f.Write([]byte("foo"))
	rtest.Assert(t, errors.Is(err, os.ErrPermission), "expected permission error for write, got %v", err)
	rtest.Assert(t, errors.Is(f.Truncate(0), os.ErrPermission), "expected permission error for truncate")
	rtest.OK(t, f.Close())

	for _, p := range []string{`\does-not-exist`, `\snapshots\does-not-exist`, `\snapshots\latest\does-not-exist`} {
		_, err := fs.Stat(p)
		rtest.Assert(t, errors.Is(err, os.ErrNotExist), "%v: expected not exist error, got %v", p, err)
	}
}
