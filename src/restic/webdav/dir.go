package webdav

import (
	"os"

	"restic/debug"

	"bazil.org/fuse/fs"
)

// This is the type for "directory-like" nodes we get from the FUSE layer.
type fsDirType interface {
	fs.HandleReadDirAller
	fs.NodeStringLookuper
	fs.Node
}

// Implements webdav.File for a restic snapshot directory
type dir struct {
	// Name of the root snapshot directory
	name string
	// The root snapshots dir this file is coupled to.
	dirNode fsDirType
}

func NewDir(name string, dirNode fsDirType) *dir {
	if dirNode == nil {
		return nil
	}

	return &dir{
		name:    name,
		dirNode: dirNode,
	}
}

// Read-only system never needs to clean up with restic.
func (this *dir) Close() error {
	debug.Log(this.name)
	return nil
}

// TODO: allow directories to be rendered as an index.
func (this *dir) Read(p []byte) (n int, err error) {
	debug.Log(this.name)
	return 0, nil
}

func (this *dir) Write(p []byte) (n int, err error) {
	debug.Log(this.name)
	return 0, os.ErrInvalid
}

func (this *dir) Seek(offset int64, whence int) (int64, error) {
	debug.Log(this.name)
	return 0, os.ErrInvalid
}

// Return the contents of the current directory, except for symlinks
// (there is no wide-spread support for handling symlinks in webdav).
func (this *dir) Readdir(count int) ([]os.FileInfo, error) {
	debug.Log(this.name)

	fileInfos := []os.FileInfo{}

	dirContents, err := this.dirNode.ReadDirAll(ctx)
	if err != nil {
		return []os.FileInfo{}, err
	}

	for _, dirEnt := range dirContents {
		debug.Log(dirEnt.Name)
		entNode, err := this.dirNode.Lookup(ctx, dirEnt.Name)
		if err != nil {
			debug.Log("error while looking up directory contents: %v %v", dirEnt, err)
			continue
		}

		if _, isLnk := entNode.(fsLinkType); isLnk {
			debug.Log("skipping symlink: ", dirEnt)
			continue
		}

		fileInfo := FileInfo{
			Filename: dirEnt.Name,
		}

		if err := entNode.Attr(ctx, &fileInfo.Attr); err != nil {
			debug.Log("error retrieving attrs for %v : %v", dirEnt.Name, err)
			continue
		}

		fileInfos = append(fileInfos, os.FileInfo(&fileInfo))
	}

	return fileInfos, nil
}

func (this *dir) Stat() (os.FileInfo, error) {
	debug.Log(this.name)
	fileInfo := FileInfo{
		Filename: this.name,
	}

	if err := this.dirNode.Attr(ctx, &fileInfo.Attr); err != nil {
		debug.Log("error retrieving attrs for %v : %v", this.name, err)
		return nil, err
	}

	return os.FileInfo(&fileInfo), nil
}
