package dump

import (
	"archive/zip"
	"context"
	"path/filepath"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/errors"
)

func (d *Dumper) dumpZip(ctx context.Context, ch <-chan *data.Node) (err error) {
	w := zip.NewWriter(d.w)

	defer func() {
		if err == nil {
			err = w.Close()
			err = errors.Wrap(err, "Close")
		}
	}()

	for node := range ch {
		if err := d.dumpNodeZip(ctx, node, w); err != nil {
			return err
		}
		if d.progress != nil {
			d.progress.AddProgress(node.Path, node.Size, node.Type)
		}
	}
	return nil
}

func (d *Dumper) dumpNodeZip(ctx context.Context, node *data.Node, zw *zip.Writer) error {
	relPath, err := filepath.Rel("/", node.Path)
	if err != nil {
		if d.progress != nil {
			_ = d.progress.Error(node.Path, err)
		}
		return err
	}

	header := &zip.FileHeader{
		Name:               filepath.ToSlash(relPath),
		UncompressedSize64: node.Size,
		Modified:           node.ModTime,
	}
	header.SetMode(node.Mode)
	if node.Type == data.NodeTypeFile {
		header.Method = zip.Deflate
	}

	if node.Type == data.NodeTypeDir {
		header.Name += "/"
	}

	w, err := zw.CreateHeader(header)
	if err != nil {
		if d.progress != nil {
			_ = d.progress.Error(node.Path, err)
		}
		return errors.Wrap(err, "ZipHeader")
	}

	if node.Type == data.NodeTypeSymlink {
		if _, err = w.Write([]byte(node.LinkTarget)); err != nil {
			if d.progress != nil {
				_ = d.progress.Error(node.Path, err)
			}
			return errors.Wrap(err, "Write")
		}

		// Report progress for symlink nodes
		if d.progress != nil {
			// Pass the node type as well
			d.progress.AddProgress(node.Path, uint64(len(node.LinkTarget)), node.Type)
		}

		return nil
	}

	return d.writeNode(ctx, w, node)
}
