//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package fuse

import (
	"bytes"
	"context"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/bloblru"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"

	"github.com/anacrolix/fuse"
	"github.com/anacrolix/fuse/fs"

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

func firstSnapshotID(t testing.TB, repo restic.Lister) (first restic.ID) {
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

func loadFirstSnapshot(t testing.TB, repo restic.ListerLoaderUnpacked) *data.Snapshot {
	id := firstSnapshotID(t, repo)
	sn, err := data.LoadSnapshot(context.TODO(), repo, id)
	rtest.OK(t, err)
	return sn
}

func loadTree(t testing.TB, repo restic.Loader, id restic.ID) *data.Tree {
	tree, err := data.LoadTree(context.TODO(), repo, id)
	rtest.OK(t, err)
	return tree
}

func TestFuseFile(t *testing.T) {
	repo := repository.TestRepository(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	timestamp, err := time.Parse(time.RFC3339, "2017-01-24T10:42:56+01:00")
	rtest.OK(t, err)
	data.TestCreateSnapshot(t, repo, timestamp, 2)

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
		size, found := repo.LookupBlobSize(restic.DataBlob, id)
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

	node := &data.Node{
		Name:    "foo",
		Inode:   23,
		Mode:    0742,
		Size:    filesize,
		Content: content,
	}
	root := &Root{repo: repo, blobCache: bloblru.New(blobCacheSize)}

	inode := inodeFromNode(1, node)
	f, err := newFile(root, func() {}, inode, node)
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
	repo := repository.TestRepository(t)

	root := &Root{repo: repo, blobCache: bloblru.New(blobCacheSize)}

	node := &data.Node{
		Mode:       0755,
		UID:        42,
		GID:        43,
		AccessTime: time.Unix(1606773731, 0),
		ChangeTime: time.Unix(1606773732, 0),
		ModTime:    time.Unix(1606773733, 0),
	}
	parentInode := inodeFromName(0, "parent")
	inode := inodeFromName(1, "foo")
	d, err := newDir(root, func() {}, inode, parentInode, node)
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
	repo := repository.TestRepository(t)
	data.TestCreateSnapshot(t, repo, time.Unix(1460289341, 207401672), 0)

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

	// data.TestCreateSnapshot does not set the UID/GID thus it must be always zero
	err = snapshotdir.Attr(ctx, &attr)
	rtest.OK(t, err)
	rtest.Equals(t, uint32(0), attr.Uid)
	rtest.Equals(t, uint32(0), attr.Gid)
}

// The Lookup method must return the same Node object unless it was forgotten in the meantime
func testStableLookup(t *testing.T, node fs.Node, path string) fs.Node {
	t.Helper()
	result, err := node.(fs.NodeStringLookuper).Lookup(context.TODO(), path)
	rtest.OK(t, err)
	result2, err := node.(fs.NodeStringLookuper).Lookup(context.TODO(), path)
	rtest.OK(t, err)
	rtest.Assert(t, result == result2, "%v are not the same object", path)

	result2.(fs.NodeForgetter).Forget()
	result2, err = node.(fs.NodeStringLookuper).Lookup(context.TODO(), path)
	rtest.OK(t, err)
	rtest.Assert(t, result != result2, "object for %v should change after forget", path)
	return result
}

func TestStableNodeObjects(t *testing.T) {
	repo := repository.TestRepository(t)
	data.TestCreateSnapshot(t, repo, time.Unix(1460289341, 207401672), 2)
	root := NewRoot(repo, Config{})

	idsdir := testStableLookup(t, root, "ids")
	snapID := loadFirstSnapshot(t, repo).ID().Str()
	snapshotdir := testStableLookup(t, idsdir, snapID)
	dir := testStableLookup(t, snapshotdir, "dir-0")
	testStableLookup(t, dir, "file-2")
}

// Test reporting of fuse.Attr.Blocks in multiples of 512.
func TestBlocks(t *testing.T) {
	root := &Root{}

	for _, c := range []struct {
		size, blocks uint64
	}{
		{0, 0},
		{1, 1},
		{511, 1},
		{512, 1},
		{513, 2},
		{1024, 2},
		{1025, 3},
		{41253, 81},
	} {
		target := strings.Repeat("x", int(c.size))

		for _, n := range []fs.Node{
			&file{root: root, node: &data.Node{Size: uint64(c.size)}},
			&link{root: root, node: &data.Node{LinkTarget: target}},
			&snapshotLink{root: root, snapshot: &data.Snapshot{}, target: target},
		} {
			var a fuse.Attr
			err := n.Attr(context.TODO(), &a)
			rtest.OK(t, err)
			rtest.Equals(t, c.blocks, a.Blocks)
		}
	}
}

func TestInodeFromNode(t *testing.T) {
	node := &data.Node{Name: "foo.txt", Type: data.NodeTypeCharDev, Links: 2}
	ino1 := inodeFromNode(1, node)
	ino2 := inodeFromNode(2, node)
	rtest.Assert(t, ino1 == ino2, "inodes %d, %d of hard links differ", ino1, ino2)

	node.Links = 1
	ino1 = inodeFromNode(1, node)
	ino2 = inodeFromNode(2, node)
	rtest.Assert(t, ino1 != ino2, "same inode %d but different parent", ino1)

	// Regression test: in a path a/b/b, the grandchild should not get the
	// same inode as the grandparent.
	a := &data.Node{Name: "a", Type: data.NodeTypeDir, Links: 2}
	ab := &data.Node{Name: "b", Type: data.NodeTypeDir, Links: 2}
	abb := &data.Node{Name: "b", Type: data.NodeTypeDir, Links: 2}
	inoA := inodeFromNode(1, a)
	inoAb := inodeFromNode(inoA, ab)
	inoAbb := inodeFromNode(inoAb, abb)
	rtest.Assert(t, inoA != inoAb, "inode(a/b) = inode(a)")
	rtest.Assert(t, inoA != inoAbb, "inode(a/b/b) = inode(a)")
}

func TestLink(t *testing.T) {
	node := &data.Node{Name: "foo.txt", Type: data.NodeTypeSymlink, Links: 1, LinkTarget: "dst", ExtendedAttributes: []data.ExtendedAttribute{
		{Name: "foo", Value: []byte("bar")},
	}}

	lnk, err := newLink(&Root{}, func() {}, 42, node)
	rtest.OK(t, err)
	target, err := lnk.Readlink(context.TODO(), nil)
	rtest.OK(t, err)
	rtest.Equals(t, node.LinkTarget, target)

	exp := &fuse.ListxattrResponse{}
	exp.Append("foo")
	resp := &fuse.ListxattrResponse{}
	rtest.OK(t, lnk.Listxattr(context.TODO(), &fuse.ListxattrRequest{}, resp))
	rtest.Equals(t, exp.Xattr, resp.Xattr)

	getResp := &fuse.GetxattrResponse{}
	rtest.OK(t, lnk.Getxattr(context.TODO(), &fuse.GetxattrRequest{Name: "foo"}, getResp))
	rtest.Equals(t, node.ExtendedAttributes[0].Value, getResp.Xattr)

	err = lnk.Getxattr(context.TODO(), &fuse.GetxattrRequest{Name: "invalid"}, nil)
	rtest.Assert(t, err != nil, "missing error on reading invalid xattr")
}

var sink uint64

func BenchmarkInode(b *testing.B) {
	for _, sub := range []struct {
		name string
		node data.Node
	}{
		{
			name: "no_hard_links",
			node: data.Node{Name: "a somewhat long-ish filename.svg.bz2", Type: data.NodeTypeFifo},
		},
		{
			name: "hard_link",
			node: data.Node{Name: "some other filename", Type: data.NodeTypeFile, Links: 2},
		},
	} {
		b.Run(sub.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sink = inodeFromNode(1, &sub.node)
			}
		})
	}
}
