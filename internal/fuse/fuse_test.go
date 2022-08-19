//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package fuse

import (
	"bytes"
	"context"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/restic/restic/internal/bloblru"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"

	rtest "github.com/restic/restic/internal/test"
)

func testRead(t testing.TB, f fs.Handle, offset, length int, data []byte) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := &fuse.ReadRequest{
		Offset: int64(offset),
		Size:   length,
	}
	resp := &fuse.ReadResponse{
		Data: data,
	}
	fr := f.(fs.HandleReader)
	rtest.OK(t, fr.Read(ctx, req, resp))
}

func firstSnapshotID(t testing.TB, repo restic.Repository) (first restic.ID) {
	err := repo.List(context.TODO(), restic.SnapshotFile, func(id restic.ID, size int64) error {
		if first.IsNull() {
			first = id
		}
		return nil
	})

	if err != nil {
		t.Fatal(err)
	}

	return first
}

func loadFirstSnapshot(t testing.TB, repo restic.Repository) *restic.Snapshot {
	id := firstSnapshotID(t, repo)
	sn, err := restic.LoadSnapshot(context.TODO(), repo, id)
	rtest.OK(t, err)
	return sn
}

func loadTree(t testing.TB, repo restic.Repository, id restic.ID) *restic.Tree {
	tree, err := restic.LoadTree(context.TODO(), repo, id)
	rtest.OK(t, err)
	return tree
}

func TestFuseFile(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	timestamp, err := time.Parse(time.RFC3339, "2017-01-24T10:42:56+01:00")
	rtest.OK(t, err)
	restic.TestCreateSnapshot(t, repo, timestamp, 2, 0.1)

	sn := loadFirstSnapshot(t, repo)
	tree := loadTree(t, repo, *sn.Tree)

	var content restic.IDs
	for _, node := range tree.Nodes {
		content = append(content, node.Content...)
	}
	t.Logf("tree loaded, content: %v", content)

	var (
		filesize uint64
		memfile  []byte
	)
	for _, id := range content {
		size, found := repo.LookupBlobSize(id, restic.DataBlob)
		rtest.Assert(t, found, "Expected to find blob id %v", id)
		filesize += uint64(size)

		buf, err := repo.LoadBlob(context.TODO(), restic.DataBlob, id, nil)
		rtest.OK(t, err)

		if len(buf) != int(size) {
			t.Fatalf("not enough bytes read for id %v: want %v, got %v", id.Str(), size, len(buf))
		}

		if uint(len(buf)) != size {
			t.Fatalf("buffer has wrong length for id %v: want %v, got %v", id.Str(), size, len(buf))
		}

		memfile = append(memfile, buf...)
	}

	t.Logf("filesize is %v, memfile has size %v", filesize, len(memfile))

	node := &restic.Node{
		Name:    "foo",
		Inode:   23,
		Mode:    0742,
		Size:    filesize,
		Content: content,
	}
	root := &Root{repo: repo, blobCache: bloblru.New(blobCacheSize)}

	inode := fs.GenerateDynamicInode(1, "foo")
	f, err := newFile(root, inode, node)
	rtest.OK(t, err)
	of, err := f.Open(context.TODO(), nil, nil)
	rtest.OK(t, err)

	attr := fuse.Attr{}
	rtest.OK(t, f.Attr(ctx, &attr))

	rtest.Equals(t, inode, attr.Inode)
	rtest.Equals(t, node.Mode, attr.Mode)
	rtest.Equals(t, node.Size, attr.Size)
	rtest.Equals(t, (node.Size/uint64(attr.BlockSize))+1, attr.Blocks)

	for i := 0; i < 200; i++ {
		offset := rand.Intn(int(filesize))
		length := rand.Intn(int(filesize)-offset) + 100

		b := memfile[offset : offset+length]

		buf := make([]byte, length)

		testRead(t, of, offset, length, buf)
		if !bytes.Equal(b, buf) {
			t.Errorf("test %d failed, wrong data returned (offset %v, length %v)", i, offset, length)
		}
	}
}

func TestFuseDir(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	root := &Root{repo: repo, blobCache: bloblru.New(blobCacheSize)}

	node := &restic.Node{
		Mode:       0755,
		UID:        42,
		GID:        43,
		AccessTime: time.Unix(1606773731, 0),
		ChangeTime: time.Unix(1606773732, 0),
		ModTime:    time.Unix(1606773733, 0),
	}
	parentInode := fs.GenerateDynamicInode(0, "parent")
	inode := fs.GenerateDynamicInode(1, "foo")
	d, err := newDir(root, inode, parentInode, node)
	rtest.OK(t, err)

	// don't open the directory as that would require setting up a proper tree blob
	attr := fuse.Attr{}
	rtest.OK(t, d.Attr(context.TODO(), &attr))

	rtest.Equals(t, inode, attr.Inode)
	rtest.Equals(t, node.UID, attr.Uid)
	rtest.Equals(t, node.GID, attr.Gid)
	rtest.Equals(t, node.AccessTime, attr.Atime)
	rtest.Equals(t, node.ChangeTime, attr.Ctime)
	rtest.Equals(t, node.ModTime, attr.Mtime)
}

// Test top-level directories for their UID and GID.
func TestTopUIDGID(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	restic.TestCreateSnapshot(t, repo, time.Unix(1460289341, 207401672), 0, 0)

	testTopUIDGID(t, Config{}, repo, uint32(os.Getuid()), uint32(os.Getgid()))
	testTopUIDGID(t, Config{OwnerIsRoot: true}, repo, 0, 0)
}

func testTopUIDGID(t *testing.T, cfg Config, repo restic.Repository, uid, gid uint32) {
	t.Helper()

	ctx := context.Background()
	root := NewRoot(repo, cfg)

	var attr fuse.Attr
	err := root.Attr(ctx, &attr)
	rtest.OK(t, err)
	rtest.Equals(t, uid, attr.Uid)
	rtest.Equals(t, gid, attr.Gid)

	idsdir, err := root.Lookup(ctx, "ids")
	rtest.OK(t, err)

	err = idsdir.Attr(ctx, &attr)
	rtest.OK(t, err)
	rtest.Equals(t, uid, attr.Uid)
	rtest.Equals(t, gid, attr.Gid)

	snapID := loadFirstSnapshot(t, repo).ID().Str()
	snapshotdir, err := idsdir.(fs.NodeStringLookuper).Lookup(ctx, snapID)
	rtest.OK(t, err)

	// restic.TestCreateSnapshot does not set the UID/GID thus it must be always zero
	err = snapshotdir.Attr(ctx, &attr)
	rtest.OK(t, err)
	rtest.Equals(t, uint32(0), attr.Uid)
	rtest.Equals(t, uint32(0), attr.Gid)
}
