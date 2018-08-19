package web

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/restic/restic/internal/repository"
)

// FIXME (kitone)
var webRepository *repository.Repository

func renderJSON(w http.ResponseWriter, data interface{}) {
	log.Printf("%v", data)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// CreateRouterAPI ...
func CreateRouterAPI(ctx context.Context, repo *repository.Repository) *mux.Router {
	webRepository = repo

	r := mux.NewRouter()
	// snapshots
	r.HandleFunc("/api/snapshots/", apiSnapshotsList)
	// nodes
	r.HandleFunc("/api/snapshots/{snapshot_id}/nodes/", apiSnapshotNodesList)
	r.HandleFunc("/api/snapshots/{snapshot_id}/nodes/{id}", apiSnapshotNodeShow)

	return r
}
