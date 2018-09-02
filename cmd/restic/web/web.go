package web

import (
	"context"
	"fmt"
	"html/template"
	"net/http"

	"github.com/gorilla/mux"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

func renderTemplate(w http.ResponseWriter, path string, data interface{}) {
	t, err := template.ParseFiles("templates/web/layouts/layout.gohtml", path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = t.ExecuteTemplate(w, "layout", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// CreateRouterWeb ...
func CreateRouterWeb(ctx context.Context, repo *repository.Repository) *mux.Router {
	webRepository = repo

	r := mux.NewRouter()
	// snapshots
	r.HandleFunc("/web/", getWebSnapshots).Methods("GET")
	r.HandleFunc("/web/snapshots/", getWebSnapshots).Methods("GET")
	r.HandleFunc("/web/snapshots/{snapshot_id}/download", getWebSnapshotDownload).Methods("GET")
	// nodes
	r.HandleFunc("/web/snapshots/{snapshot_id}/nodes/", getWebSnapshotNodes).Methods("GET")
	return r
}

// Snapshots

func getWebSnapshots(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "templates/web/snapshots/index.gohtml", nil)
}

func getWebSnapshotDownload(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)

	snapshotID := params["snapshot_id"]
	splittedPath := splitPath(r.URL.Query().Get("path"))

	id, err := restic.FindSnapshot(webRepository, snapshotID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithCancel(context.TODO())
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

// Nodes

func getWebSnapshotNodes(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)

	type data struct {
		SnapshotID    string
		DefaultTarget string
	}
	home, _ := homedir.Dir()
	renderTemplate(w, "templates/web/nodes/index.gohtml", data{params["snapshot_id"], home})
}
