package dump

import (
	"archive/zip"
	"context"
	"path/filepath"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

func (d *Dumper) dumpZip(ctx context.Context, ch <-chan *restic.Node) (err error) {
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

func (d *Dumper) dumpNodeZip(ctx context.Context, node *restic.Node, zw *zip.Writer) error {
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

	if IsDir(node) {
		header.Name += "/"
	}

	w, err := zw.CreateHeader(header)
	if err != nil {
		return errors.Wrap(err, "ZipHeader")
	}

	if IsLink(node) {
		if _, err = w.Write([]byte(node.LinkTarget)); err != nil {
			return errors.Wrap(err, "Write")
		}

		return nil
	}

	return d.writeNode(ctx, w, node)
}
