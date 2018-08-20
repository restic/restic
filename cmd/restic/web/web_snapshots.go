package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

	"github.com/gorilla/mux"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

func webSnapshotsList(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "templates/web/snapshots/index.gohtml", nil)
}

func webSnapshotDownloadShow(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)

	snapshotID := params["snapshot_id"]
	splittedPath := splitPath(r.URL.Query().Get("path"))

	id, err := restic.FindSnapshot(webRepository, snapshotID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithCancel(context.TODO()) // globalOptions.ctx)
	defer cancel()

	sn, err := restic.LoadSnapshot(ctx, webRepository, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	tree, err := webRepository.LoadTree(ctx, *sn.Tree)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	node, err := findNode(ctx, tree, webRepository, "", splittedPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Add("Content-Disposition", "Attachment; filename="+node.Name)
	w.Header().Set("Content-Length", fmt.Sprintf("%v", node.Size))
	err = dumpNode(ctx, webRepository, node, w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func splitPath(path string) []string {
	d, f := filepath.Split(path)
	if d == "" || d == "/" {
		return []string{f}
	}
	s := splitPath(filepath.Clean(d))
	return append(s, f)
}

func dumpNode(ctx context.Context, repo restic.Repository, node *restic.Node, writer io.Writer) error {
	var buf []byte
	for _, id := range node.Content {
		size, found := repo.LookupBlobSize(id, restic.DataBlob)
		if !found {
			return errors.Errorf("id %v not found in repository", id)
		}

		buf = buf[:cap(buf)]
		if len(buf) < restic.CiphertextLength(int(size)) {
			buf = restic.NewBlobBuffer(int(size))
		}

		n, err := repo.LoadBlob(ctx, restic.DataBlob, id, buf)
		if err != nil {
			return err
		}
		buf = buf[:n]

		_, err = writer.Write(buf)
		if err != nil {
			return errors.Wrap(err, "Write")
		}
	}
	return nil
}

func findNode(ctx context.Context, tree *restic.Tree, repo restic.Repository, prefix string, pathComponents []string) (*restic.Node, error) {
	if tree == nil {
		return nil, fmt.Errorf("called with a nil tree")
	}
	if repo == nil {
		return nil, fmt.Errorf("called with a nil repository")
	}
	l := len(pathComponents)
	if l == 0 {
		return nil, fmt.Errorf("empty path components")
	}
	item := filepath.Join(prefix, pathComponents[0])
	for _, node := range tree.Nodes {
		if node.Name == pathComponents[0] {
			switch {
			case l == 1 && node.Type == "file":
				return node, nil // found
			case l > 1 && node.Type == "dir":
				subtree, err := repo.LoadTree(ctx, *node.Subtree)
				if err != nil {
					return nil, errors.Wrapf(err, "cannot load subtree for %q", item)
				}
				return findNode(ctx, subtree, repo, item, pathComponents[1:])
			case l > 1:
				return nil, fmt.Errorf("%q should be a dir, but s a %q", item, node.Type)
			case node.Type != "file":
				return nil, fmt.Errorf("%q should be a file, but is a %q", item, node.Type)
			}
		}
	}
	return nil, fmt.Errorf("path %q not found in snapshot", item)
}
