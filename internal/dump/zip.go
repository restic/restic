package dump

import (
	"archive/zip"
	"context"
	"path/filepath"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

func (d *Dumper) dumpZip(ctx context.Context, ch <-chan *restic.Node) (err error) {
	if d.zipWriter == nil {
		d.zipWriter = zip.NewWriter(d.w)
	}

	for node := range ch {
		if err := d.dumpNodeZip(ctx, node); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dumper) dumpNodeZip(ctx context.Context, node *restic.Node) error {
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
	if node.Type == restic.NodeTypeFile {
		header.Method = zip.Deflate
	}

	if node.Type == restic.NodeTypeDir {
		header.Name += "/"
	}

	w, err := d.zipWriter.CreateHeader(header)
	if err != nil {
		return errors.Wrap(err, "ZipHeader")
	}

	if node.Type == restic.NodeTypeSymlink {
		if _, err = w.Write([]byte(node.LinkTarget)); err != nil {
			return errors.Wrap(err, "Write")
		}

		return nil
	}

	return d.writeNode(ctx, w, node)
}
