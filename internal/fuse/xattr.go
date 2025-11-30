//go:build darwin || freebsd || linux

package fuse

import (
	"github.com/anacrolix/fuse"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
)

func nodeToXattrList(node *data.Node, req *fuse.ListxattrRequest, resp *fuse.ListxattrResponse) {
	debug.Log("Listxattr(%v, %v)", node.Name, req.Size)
	for _, attr := range node.ExtendedAttributes {
		resp.Append(attr.Name)
	}
}

func nodeGetXattr(node *data.Node, req *fuse.GetxattrRequest, resp *fuse.GetxattrResponse) error {
	debug.Log("Getxattr(%v, %v, %v)", node.Name, req.Name, req.Size)
	attrval := node.GetExtendedAttribute(req.Name)
	if attrval != nil {
		resp.Xattr = attrval
		return nil
	}
	return fuse.ErrNoXattr
}
