package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

// FIXME (kitone)
var webRepository *repository.Repository

func renderJSON(w http.ResponseWriter, data interface{}) {
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
	r.HandleFunc("/api/snapshots/", getSnapshots).Methods("GET")
	r.HandleFunc("/api/snapshots/{id}/restore/", createSnapshotRestore).Methods("POST")
	// nodes
	r.HandleFunc("/api/snapshots/{snapshot_id}/nodes/", getSnapshotNodes).Methods("GET")
	r.HandleFunc("/api/snapshots/{snapshot_id}/nodes/{id}", getSnapshotNode).Methods("GET")

	return r
}

// Snapshots

type apiSnapshot struct {
	*restic.Snapshot
	ShortID string `json:"short_id"`
}

func getSnapshots(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	snapshots, err := restic.LoadAllSnapshots(ctx, webRepository)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var data []apiSnapshot
	for _, sn := range snapshots {
		data = append(data, apiSnapshot{sn, sn.ID().Str()})
	}
	renderJSON(w, data)
}

func createSnapshotRestore(w http.ResponseWriter, r *http.Request) {
	type restoreTask struct {
		Target string   `json:"target"`
		Files  []string `json:"files"`
	}
	var task restoreTask
	err := json.NewDecoder(r.Body).Decode(&task)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if task.Target == "" {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		http.Error(w, "Target should not be empty", http.StatusInternalServerError)
		return
	}
	if len(task.Files) == 0 {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		http.Error(w, "no files to restore", http.StatusInternalServerError)
		return
	}
	params := mux.Vars(r)
	snapshotID, err := restic.FindSnapshot(webRepository, params["id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	totalErrors, err := restore(webRepository, snapshotID, task.Files, task.Target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		renderJSON(w, fmt.Sprintf("Successfully restored files, there were %d errors", totalErrors))
	}
}

// Nodes

func getSnapshotNodes(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)

	id, err := restic.FindSnapshot(webRepository, params["snapshot_id"])
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
	renderJSON(w, tree)
}

func getSnapshotNode(w http.ResponseWriter, r *http.Request) {
	// FIXME (kitone): should we check if the node id belongs to the snapshot_id first ?
	params := mux.Vars(r)

	tr, err := restic.ParseID(params["id"])
	if err != nil {
		fmt.Fprintf(w, "failed to parseID of the tree, %v", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	tree, err := webRepository.LoadTree(ctx, tr)
	if err != nil {
		fmt.Fprintf(w, "failed to tree, %v", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	renderJSON(w, tree)
}
