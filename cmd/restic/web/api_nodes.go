package web

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/restic/restic/internal/restic"
)

func apiSnapshotNodesList(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)

	id, err := restic.FindSnapshot(webRepository, params["snapshot_id"])
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
	renderJSON(w, tree)
}

func apiSnapshotNodeShow(w http.ResponseWriter, r *http.Request) {
	// FIXME (kitone): should we check if the node id belongs to the snapshot_id first ?
	params := mux.Vars(r)

	tr, err := restic.ParseID(params["id"])
	if err != nil {
		fmt.Fprintf(w, "failed to parseID of the tree, %v", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithCancel(context.TODO()) // globalOptions.ctx)
	defer cancel()

	tree, err := webRepository.LoadTree(ctx, tr)
	if err != nil {
		fmt.Fprintf(w, "failed to tree, %v", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	renderJSON(w, tree)
}
