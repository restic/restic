package web

import (
	"net/http"
)

func webSnapshotsList(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "templates/web/snapshots/index.gohtml", nil)
}
