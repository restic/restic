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
	}
	return nil
}

func (d *Dumper) dumpNodeZip(ctx context.Context, node *data.Node, zw *zip.Writer) error {
	relPath, err := filepath.Rel("/", node.Path)
	if err != nil {
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
		return errors.Wrap(err, "ZipHeader")
	}

	if node.Type == data.NodeTypeSymlink {
		if _, err = w.Write([]byte(node.LinkTarget)); err != nil {
			return errors.Wrap(err, "Write")
		}

		return nil
	}

	return d.writeNode(ctx, w, node)
}
