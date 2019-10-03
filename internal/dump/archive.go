package dump

import (
	"archive/tar"
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
)

// WriteArchive will write the contents of the given tree, encoded as a tar or zip to the given destination.
// It will loop over all nodes in the tree and dump them recursively.
func WriteArchive(ctx context.Context, archiveType string, repo restic.Repository, tree *restic.Tree, rootPath string, dst io.Writer) error {
	var writer interface{}

	switch archiveType {
	case "tar":
		tw := tar.NewWriter(dst)
		defer tw.Close()
		writer = tw
	case "zip":
		zw := zip.NewWriter(dst)
		defer zw.Close()
		writer = zw
	default:
		return fmt.Errorf("unkown archive format %q", archiveType)
	}

	for _, rootNode := range tree.Nodes {
		rootNode.Path = rootPath
		err := archiveTree(ctx, repo, rootNode, rootPath, writer)
		if err != nil {
			return err
		}
	}
	return nil
}

func archiveTree(ctx context.Context, repo restic.Repository, rootNode *restic.Node, rootPath string, w interface{}) error {
	rootNode.Path = path.Join(rootNode.Path, rootNode.Name)
	rootPath = rootNode.Path

	if err := archiveNode(ctx, w, rootNode, repo); err != nil {
		return err
	}

	// If this is no directory we are finished
	if !IsDir(rootNode) {
		return nil
	}

	err := walker.Walk(ctx, repo, *rootNode.Subtree, nil, func(_ restic.ID, nodepath string, node *restic.Node, err error) (bool, error) {
		if err != nil {
			return false, err
		}
		if node == nil {
			return false, nil
		}

		node.Path = path.Join(rootPath, nodepath)

		if IsFile(node) || IsLink(node) || IsDir(node) {
			err := archiveNode(ctx, w, node, repo)
			if err != nil {
				return false, err
			}
		}

		return false, nil
	})

	return err
}

// copied from archive/tar.FileInfoHeader
const (
	// Mode constants from the USTAR spec:
	// See http://pubs.opengroup.org/onlinepubs/9699919799/utilities/pax.html#tag_20_92_13_06
	c_ISUID = 04000 // Set uid
	c_ISGID = 02000 // Set gid
	c_ISVTX = 01000 // Save text (sticky bit)
)

func archiveNode(ctx context.Context, writer interface{}, node *restic.Node, repo restic.Repository) error {
	var w io.Writer
	relPath, err := filepath.Rel("/", node.Path)
	if err != nil {
		return err
	}

	switch writer.(type) {
	case *tar.Writer:
		tw := writer.(*tar.Writer)
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

		err = tw.WriteHeader(header)

		if err != nil {
			return errors.Wrap(err, "TarHeader ")
		}

		w = tw
	case *zip.Writer:
		header := &zip.FileHeader{
			Name:               strings.TrimPrefix(node.Path, "/"),
			UncompressedSize64: uint64(node.Size),
			Modified:           node.ModTime,
		}
		if IsLink(node) {
			header.SetMode(node.Mode)
		}
		if IsDir(node) {
			if !strings.HasSuffix(header.Name, "/") {
				header.Name += "/"
			}
		}

		var err error
		w, err = writer.(*zip.Writer).CreateHeader(header)
		if err != nil {
			return errors.Wrap(err, "ZipHeader ")
		}
		if IsLink(node) {
			if _, err = w.Write([]byte(node.LinkTarget)); err != nil {
				return errors.Wrap(err, "ZipHeader ")
			}
		}
	default:
		return fmt.Errorf("unknown writer %T", writer)

	}

	return GetNodeData(ctx, w, repo, node)
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

// GetNodeData will write the contents of the node to the given output
func GetNodeData(ctx context.Context, output io.Writer, repo restic.Repository, node *restic.Node) error {
	var (
		buf []byte
		err error
	)
	for _, id := range node.Content {
		buf, err = repo.LoadBlob(ctx, restic.DataBlob, id, buf)
		if err != nil {
			return err
		}

		_, err = output.Write(buf)
		if err != nil {
			return errors.Wrap(err, "Write")
		}

	}
	return nil
}

// IsDir checks if the given node is a directory
func IsDir(node *restic.Node) bool {
	return node.Type == "dir"
}

// IsLink checks if the given node as a link
func IsLink(node *restic.Node) bool {
	return node.Type == "symlink"
}

// IsFile checks if the given node is a file
func IsFile(node *restic.Node) bool {
	return node.Type == "file"
}
