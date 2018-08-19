package web

import (
	"context"
	"net/http"

	"github.com/restic/restic/internal/restic"
)

type apiSnapshot struct {
	*restic.Snapshot
	ShortID string `json:"short_id"`
}

func apiSnapshotsList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(context.TODO()) //globalOptions.ctx)
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
