package dump

import (
	"archive/tar"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

func (d *Dumper) dumpTar(ctx context.Context, ch <-chan *restic.Node) (err error) {
	w := tar.NewWriter(d.w)

	defer func() {
		if err == nil {
			err = w.Close()
			err = errors.Wrap(err, "Close")
		}
	}()

	for node := range ch {
		if err := d.dumpNodeTar(ctx, node, w); err != nil {
			return err
		}
	}
	return nil
}

// copied from archive/tar.FileInfoHeader
const (
	// Mode constants from the USTAR spec:
	// See http://pubs.opengroup.org/onlinepubs/9699919799/utilities/pax.html#tag_20_92_13_06
	cISUID = 0o4000 // Set uid
	cISGID = 0o2000 // Set gid
	cISVTX = 0o1000 // Save text (sticky bit)
)

// in a 32-bit build of restic:
// substitute a uid or gid of -1 (which was converted to 2^32 - 1) with 0
func tarIdentifier(id uint32) int {
	if int(id) == -1 {
		return 0
	}
	return int(id)
}

func (d *Dumper) dumpNodeTar(ctx context.Context, node *restic.Node, w *tar.Writer) error {
	relPath, err := filepath.Rel("/", node.Path)
	if err != nil {
		return err
	}

	header := &tar.Header{
		Name:       filepath.ToSlash(relPath),
		Size:       int64(node.Size),
		Mode:       int64(node.Mode.Perm()), // cIS* constants are added later
		Uid:        tarIdentifier(node.UID),
		Gid:        tarIdentifier(node.GID),
		Uname:      node.User,
		Gname:      node.Group,
		ModTime:    node.ModTime,
		AccessTime: node.AccessTime,
		ChangeTime: node.ChangeTime,
		PAXRecords: parseXattrs(node.ExtendedAttributes),
	}

	// adapted from archive/tar.FileInfoHeader
	if node.Mode&os.ModeSetuid != 0 {
		header.Mode |= cISUID
	}
	if node.Mode&os.ModeSetgid != 0 {
		header.Mode |= cISGID
	}
	if node.Mode&os.ModeSticky != 0 {
		header.Mode |= cISVTX
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

	err = w.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("writing header for %q: %w", node.Path, err)
	}
	return d.writeNode(ctx, w, node)
}

func parseXattrs(xattrs []restic.ExtendedAttribute) map[string]string {
	tmpMap := make(map[string]string)

	for _, attr := range xattrs {
		// Check for Linux POSIX.1e ACLs.
		//
		// TODO support ACLs from other operating systems.
		// FreeBSD ACLs have names "posix1e.acl_(access|default)",
		// but their binary format may not match the Linux format.
		aclKey := ""
		switch attr.Name {
		case "system.posix_acl_access":
			aclKey = "SCHILY.acl.access"
		case "system.posix_acl_default":
			aclKey = "SCHILY.acl.default"
		}

		if aclKey != "" {
			text, err := formatLinuxACL(attr.Value)
			if err != nil {
				debug.Log("parsing Linux ACL: %v, skipping", err)
				continue
			}
			tmpMap[aclKey] = text
		} else {
			tmpMap["SCHILY.xattr."+attr.Name] = string(attr.Value)
		}
	}

	return tmpMap
}
