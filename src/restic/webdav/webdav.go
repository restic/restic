// +build !openbsd
// +build !windows

package webdav

import (
	"os"
	"strings"

	"restic/errors"
	resticfuse "restic/fuse"

	"golang.org/x/net/webdav"
	"golang.org/x/net/context"

	"restic/debug"
	"bazil.org/fuse/fs"
	"bazil.org/fuse"
	"path"
)

var ctx context.Context = context.Background()

// Implements webdav.FileSystem interface. This is a wrapper to the
// fuse.Filesystem which handles the discrepancies.
type WebdavFS struct {
	root *resticfuse.SnapshotsDir
}

// Create a new webdav.FileSystem for the given repository
func NewWebdavFS(snapshotDir *resticfuse.SnapshotsDir) webdav.FileSystem {
	if snapshotDir == nil {
		return nil
	}

	return &WebdavFS{
		root: snapshotDir,
	}
}

func (this *WebdavFS) Mkdir(name string, perm os.FileMode) error {
	debug.Log(name)
	return errors.New("WebdavFS is read only. MkDir is not supported.")
}

func (this *WebdavFS) OpenFile(name string, flag int, perm os.FileMode) (webdav.File, error) {
	filename, resolvedNode, err := this.resolve(name)
	if err != nil {
		debug.Log("resolved failed")
		return nil, err
	}

	// Determine what type of node we landed on.
	switch node := resolvedNode.(type) {
	// Is is a directory type?
	case fsDirType:
		debug.Log("dir: %v", name)
		return NewDir(filename, node), nil
	case fsFileType:
		debug.Log("file: %v", name)
		return NewFile(filename, node), nil
	case fsLinkType:
		debug.Log("link: %v", name)
		// We need to return not found *here* if we try to open a broken symlink.
		// Of course, nothing above us can handle this anyway, but its the right
		// thing to do.
		target, err := this.resolveLinkNode(filename, node)
		if err != nil {
			return nil, err
		}
		return NewLink(filename, node, target), nil
	default:
		debug.Log("UNKNOWN: %v", name)
		return nil, os.ErrInvalid
	}
}

func (this *WebdavFS) RemoveAll(name string) error {
	debug.Log("RemoveAll(%v)", name)
	return errors.New("Unimplemented")
}

func (this *WebdavFS) Rename(oldName, newName string) error {
	debug.Log("Rename(%v,%v)", oldName, newName)
	return errors.New("Unimplemented")
}

func (this *WebdavFS) Stat(name string) (os.FileInfo, error) {
	debug.Log(name)

	filename, resolvedNode, err := this.resolve(name)
	if err != nil {
		return nil, err
	}

	// Return the file info as a fuse -> os.FileInfo mapper
	fileInfo := FileInfo{
		Filename: filename,
	}

	if err := resolvedNode.Attr(ctx, &fileInfo.Attr); err != nil {
		return nil, os.ErrInvalid
	}

	return fileInfo, nil
}

func (this *WebdavFS) resolve(name string) (string, fs.Node, error) {
	// Clean the path
	name = path.Clean(name)

	// Remove any leading slash, since it makes path sense logic weird
	name = strings.TrimPrefix(name, "/")

	// Split the path by slashes, remove 0-length tail
	pathlist := strings.Split(name, "/")
	if len(pathlist) == 1 && pathlist[0] == "" {
		pathlist = []string{}
	}

	// The first node is always the snapshot root, so set it implicitly.
	var filename string
	var currentNode fs.Node

	filename = "/"
	currentNode = this.root

	// Recursively resolve each path component till we arrive at the file.
	// Skip the first node since it's the snapshot directory
	var err error
	for _, pathElem := range pathlist {
		debug.Log("resolving: %v", pathElem)
		// Set the current path element filename
		filename = pathElem

		// Check that we are still able to lookup nodes
		oldNode, ok := currentNode.(fs.NodeStringLookuper)
		if !ok {
			return filename, nil, os.ErrNotExist
		}

		if currentNode, err = oldNode.Lookup(ctx, pathElem) ; err != nil {
			return filename, nil, err
		}
	}

	return filename, currentNode, err
}

// Helper function to safely resolve link nodes.
func (this *WebdavFS) resolveLinkNode(name string, node fsLinkType) (webdav.File, error) {
	// Keep track of inodes we've seen before. If they start to recur, then
	// we're in a look and should fail.
	//inodes := make(map[uint64]interface{})

	// Loop resolution until finished.
	resolvedNode := fs.Node(node)
	realname := name
	for {
		// TODO: do we need multi-level resolution (probably)
		switch v := resolvedNode.(type) {
		case fsDirType:
			return NewDir(realname, v), nil
		case fsFileType:
			return NewFile(realname, v), nil
		case fsLinkType:
			// Still a symlink - continue resolving.
			req := &fuse.ReadlinkRequest{}
			nextname, err := v.Readlink(ctx, req) ;
			if err != nil {
				return nil, os.ErrNotExist
			}

			debug.Log("resolved: %s => %s", realname, nextname)
			realname = nextname
			// TODO: rewrite symlinks to be snapshot-relative

			// Lookup the next node...
			targetNode, err := this.root.Lookup(ctx, realname)
			if err != nil {
				return nil, os.ErrNotExist
			}
			resolvedNode = targetNode

		default:
			return nil, os.ErrInvalid
		}
	}
}