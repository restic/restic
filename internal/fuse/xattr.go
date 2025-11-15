//go:build darwin || freebsd || linux || windows
// +build darwin freebsd linux windows

package fuse

import (
	"syscall"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
)

func nodeToXattrList(node *data.Node, req *ListxattrRequest, resp *ListxattrResponse) {
	debug.Log("Listxattr(%v, %v)", node.Name, req.Size)
	for _, attr := range node.ExtendedAttributes {
		resp.Append(attr.Name)
	}
}

func nodeGetXattr(node *data.Node, req *GetxattrRequest, resp *GetxattrResponse) error {
	debug.Log("Getxattr(%v, %v, %v)", node.Name, req.Name, req.Size)
	attrval := node.GetExtendedAttribute(req.Name)
	if attrval != nil {
		resp.Xattr = attrval
		return nil
	}
	return syscall.ENODATA
}
