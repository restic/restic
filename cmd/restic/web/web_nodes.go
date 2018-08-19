package web

import (
	"net/http"

	"github.com/gorilla/mux"
)

func webSnapshotNodesList(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)

	type data struct {
		SnapshotID string
	}
	renderTemplate(w, "templates/web/nodes/index.gohtml", data{params["snapshot_id"]})
}
