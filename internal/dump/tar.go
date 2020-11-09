package dump

import (
	"archive/tar"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

type tarDumper struct {
	w *tar.Writer
}

// Statically ensure that tarDumper implements dumper.
var _ dumper = tarDumper{}

// WriteTar will write the contents of the given tree, encoded as a tar to the given destination.
func WriteTar(ctx context.Context, repo restic.Repository, tree *restic.Tree, rootPath string, dst io.Writer) error {
	dmp := tarDumper{w: tar.NewWriter(dst)}

	err := writeDump(ctx, repo, tree, rootPath, dmp, dst)
	if err != nil {
		dmp.w.Close()
		return err
	}

	return dmp.w.Close()
}

// copied from archive/tar.FileInfoHeader
const (
	// Mode constants from the USTAR spec:
	// See http://pubs.opengroup.org/onlinepubs/9699919799/utilities/pax.html#tag_20_92_13_06
	c_ISUID = 04000 // Set uid
	c_ISGID = 02000 // Set gid
	c_ISVTX = 01000 // Save text (sticky bit)
)

func (dmp tarDumper) dumpNode(ctx context.Context, node *restic.Node, repo restic.Repository) error {
	relPath, err := filepath.Rel("/", node.Path)
	if err != nil {
		return err
	}

	header := &tar.Header{
		Name:       filepath.ToSlash(relPath),
		Size:       int64(node.Size),
		Mode:       int64(node.Mode.Perm()), // c_IS* constants are added later
		Uid:        int(node.UID),
		Gid:        int(node.GID),
		Uname:      node.User,
		Gname:      node.Group,
		ModTime:    node.ModTime,
		AccessTime: node.AccessTime,
		ChangeTime: node.ChangeTime,
		PAXRecords: parseXattrs(node.ExtendedAttributes),
	}

	// adapted from archive/tar.FileInfoHeader
	if node.Mode&os.ModeSetuid != 0 {
		header.Mode |= c_ISUID
	}
	if node.Mode&os.ModeSetgid != 0 {
		header.Mode |= c_ISGID
	}
	if node.Mode&os.ModeSticky != 0 {
		header.Mode |= c_ISVTX
	}

	if IsFile(node) {
		header.Typeflag = tar.TypeReg
	}

	if IsLink(node) {
		header.Typeflag = tar.TypeSymlink
		header.Linkname = node.LinkTarget
	}

	if IsDir(node) {
		header.Typeflag = tar.TypeDir
		header.Name += "/"
	}

	err = dmp.w.WriteHeader(header)

	if err != nil {
		return errors.Wrap(err, "TarHeader ")
	}

	return GetNodeData(ctx, dmp.w, repo, node)
}

func parseXattrs(xattrs []restic.ExtendedAttribute) map[string]string {
	tmpMap := make(map[string]string)

	for _, attr := range xattrs {
		attrString := string(attr.Value)

		if strings.HasPrefix(attr.Name, "system.posix_acl_") {
			na := acl{}
			na.decode(attr.Value)

			if na.String() != "" {
				if strings.Contains(attr.Name, "system.posix_acl_access") {
					tmpMap["SCHILY.acl.access"] = na.String()
				} else if strings.Contains(attr.Name, "system.posix_acl_default") {
					tmpMap["SCHILY.acl.default"] = na.String()
				}
			}
		} else {
			tmpMap["SCHILY.xattr."+attr.Name] = attrString
		}
	}

	return tmpMap
}
