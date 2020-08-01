package fuse

import (
	"context"
	"sort"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

// FsNodeSnapshotDir represents a single directory of a snapshot inside the
// virtual filesystem.
type FsNodeSnapshotDir struct {
	root        *FsNodeRoot
	files       map[string]*restic.Node
	directories map[string]*FsNodeSnapshotDir
}

var _ = FsNode(&FsNodeSnapshotDir{})

// NewFsNodeSnapshotDirFromSnapshot creates a new FsNodeSnapshotDir for the
// given snapshot.
func NewFsNodeSnapshotDirFromSnapshot(
	ctx context.Context, root *FsNodeRoot, snapshot *restic.Snapshot,
) (*FsNodeSnapshotDir, error) {

	debug.Log("id %v (tree %v)", snapshot.ID(), snapshot.Tree)

	tree, err := root.repo.LoadTree(ctx, *snapshot.Tree)
	if err != nil {
		debug.Log("loadTree(%v) failed: %v", snapshot.ID(), err)
		return nil, err
	}

	files := make(map[string]*restic.Node)
	directories := make(map[string]*FsNodeSnapshotDir)

	for _, n := range tree.Nodes {

		treeNodes, err := replaceSpecialNodes(ctx, root.repo, n)
		if err != nil {
			debug.Log("replaceSpecialNodes(%v) failed: %v", n, err)
			return nil, err
		}

		for _, node := range treeNodes {

			nodeName := cleanupNodeName(node.Name)

			switch node.Type {
			case "file":
				files[nodeName] = node
			case "dir":
				child, err := newFsNodeSnapshotDir(ctx, root, node)

				if err != nil {
					return nil, err
				}

				directories[nodeName] = child
			}
		}
	}

	return &FsNodeSnapshotDir{
		root:        root,
		files:       files,
		directories: directories,
	}, nil
}

// newFsNodeSnapshotDir creates a new FsNodeSnapshotDir for the given node.
func newFsNodeSnapshotDir(
	ctx context.Context, root *FsNodeRoot, node *restic.Node,
) (*FsNodeSnapshotDir, error) {

	debug.Log("newFsNodeSnapshotDir %v (%v)", node.Name, node.Subtree)

	tree, err := root.repo.LoadTree(ctx, *node.Subtree)
	if err != nil {
		debug.Log("newFsNodeSnapshotDir error loading tree %v: %v", node.Subtree, err)
		return nil, err
	}

	files := make(map[string]*restic.Node)
	directories := make(map[string]*FsNodeSnapshotDir)

	for _, node := range tree.Nodes {

		debug.Log("handling node %v", node.Name)

		nodeName := cleanupNodeName(node.Name)

		switch node.Type {
		case "file":
			files[nodeName] = node
		case "dir":
			child, err := newFsNodeSnapshotDir(ctx, root, node)

			if err != nil {
				return nil, err
			}

			directories[nodeName] = child
		}
	}

	return &FsNodeSnapshotDir{
		root:        root,
		files:       files,
		directories: directories,
	}, nil
}

// Readdir lists all items in the specified path. Results are returned
// through the given callback function.
func (self *FsNodeSnapshotDir) Readdir(path []string, fill FsListItemCallback) {

	debug.Log("Readdir(%v)", path)

	fill(".", nil, 0)
	fill("..", nil, 0)

	if len(path) == 0 {
		for name, _ := range self.directories {
			fill(name, &defaultDirectoryStat, 0)
		}

		for name, file := range self.files {
			fileStat := fuse.Stat_t{}
			nodeToStat(file, &fileStat)
			fill(name, &fileStat, 0)
		}
	} else {
		head := path[0]
		tail := path[1:]

		if dir, found := self.directories[head]; found {
			dir.Readdir(tail, fill)
		}
	}
}

// GetAttributes fetches the attributes of the specified file or directory.
func (self *FsNodeSnapshotDir) GetAttributes(path []string, stat *fuse.Stat_t) bool {

	debug.Log("ListDirectories(%v)", path)

	lenPath := len(path)

	if lenPath == 0 {
		*stat = defaultDirectoryStat
		return true
	}

	head := path[0]

	if file, found := self.files[head]; lenPath == 1 && found {
		nodeToStat(file, stat)
		return true
	}

	if dir, found := self.directories[head]; found {
		tail := path[1:]
		return dir.GetAttributes(tail, stat)
	}

	return false
}

// Open opens the file for the given path.
func (self *FsNodeSnapshotDir) Open(path []string, flags int) (errc int, fh uint64) {

	lenPath := len(path)

	if lenPath == 0 {
		return -fuse.EISDIR, ^uint64(0)
	}

	head := path[0]

	if _, found := self.files[head]; lenPath == 1 && found {
		return 0, 0
	}

	if dir, found := self.directories[head]; found {
		tail := path[1:]
		return dir.Open(tail, flags)
	}

	return -fuse.ENOENT, ^uint64(0)
}

// Read reads data to the given buffer from the specified file.
func (self *FsNodeSnapshotDir) Read(path []string, buff []byte, ofst int64, fh uint64) (n int) {

	lenPath := len(path)

	if lenPath == 0 {
		return -fuse.EISDIR
	}

	head := path[0]

	if dir, found := self.directories[head]; found {
		tail := path[1:]
		return dir.Read(tail, buff, ofst, fh)
	}

	if node, found := self.files[head]; lenPath == 1 && found {
		debug.Log("Read(%v, %v, %v), file size %v", node.Name, len(buff), ofst, node.Size)
		offset := uint64(ofst)

		if offset > node.Size {
			debug.Log("Read(%v): offset is greater than file size: %v > %v",
				node.Name, ofst, node.Size)

			// return no data
			return 0
		}

		// handle special case: file is empty
		if node.Size == 0 {
			return 0
		}

		cumsize, err := self.cumsize(node)

		if err != nil {
			return -fuse.EIO
		}

		// Skip blobs before the offset
		startContent := -1 + sort.Search(len(cumsize), func(i int) bool {
			return cumsize[i] > offset
		})
		offset -= cumsize[startContent]

		readBytes := 0
		remainingBytes := len(buff)

		for i := startContent; remainingBytes > 0 && i < len(cumsize)-1; i++ {

			blob, err := self.getBlobAt(self.root.ctx, node, i)
			if err != nil {
				return -fuse.EIO
			}

			if offset > 0 {
				blob = blob[offset:]
				offset = 0
			}

			copied := copy(buff[readBytes:], blob)

			remainingBytes -= copied
			readBytes += copied
		}

		return readBytes
	}

	return -fuse.ENOENT
}

// cumsize calculates the size of all blobs for the given node.
func (self *FsNodeSnapshotDir) cumsize(node *restic.Node) ([]uint64, error) {

	var bytes uint64
	cumsize := make([]uint64, 1+len(node.Content))

	for i, id := range node.Content {
		size, found := self.root.repo.LookupBlobSize(id, restic.DataBlob)

		if !found {
			return nil, errors.Errorf("id %v not found in repository", id)
		}

		bytes += uint64(size)
		cumsize[i+1] = bytes
	}

	return cumsize, nil
}

// getBlobAt fetches a specific blob for a given node from the repository.
func (self *FsNodeSnapshotDir) getBlobAt(ctx context.Context, node *restic.Node, i int) (blob []byte, err error) {
	debug.Log("getBlobAt(%v, %v)", node.Name, i)

	blob, ok := self.root.blobCache.get(node.Content[i])
	if ok {
		return blob, nil
	}

	blob, err = self.root.repo.LoadBlob(ctx, restic.DataBlob, node.Content[i], nil)
	if err != nil {
		debug.Log("LoadBlob(%v, %v) failed: %v", node.Name, node.Content[i], err)
		return nil, err
	}

	self.root.blobCache.add(node.Content[i], blob)

	return blob, nil
}

// nodeToStat convert node ifnromation to filesystem stat information.
func nodeToStat(node *restic.Node, stat *fuse.Stat_t) {

	stat.Atim = fuse.NewTimespec(node.AccessTime)
	stat.Mtim = fuse.NewTimespec(node.ModTime)
	stat.Ctim = fuse.NewTimespec(node.ChangeTime)

	switch node.Type {
	case "dir":
		stat.Mode = fuse.S_IFDIR | 0555
	case "file":
		stat.Mode = fuse.S_IFREG | 0444
		stat.Size = int64(node.Size)
	}
}
