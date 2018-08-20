package web

import (
	"context"
	"html/template"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/restic/restic/internal/repository"
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
	r.HandleFunc("/web/", webSnapshotsList)
	r.HandleFunc("/web/snapshots/", webSnapshotsList)
	r.HandleFunc("/web/snapshots/{snapshot_id}/download", webSnapshotDownloadShow)
	// nodes
	r.HandleFunc("/web/snapshots/{snapshot_id}/nodes/", webSnapshotNodesList)

	return r
}
