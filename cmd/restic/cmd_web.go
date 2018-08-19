package main

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/spf13/cobra"

	"github.com/restic/restic/cmd/restic/web"
	"github.com/restic/restic/internal/repository"
)

var cmdWeb = &cobra.Command{
	Use:               "web",
	Short:             "FIXME",
	Long:              "FIXME",
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWeb(webOptions, globalOptions, args)
	},
}

// WebOptions collects all options for the ls command.
type WebOptions struct {
}

var webOptions WebOptions
var webRepository *repository.Repository

func init() {
	cmdRoot.AddCommand(cmdWeb)
}

func loadRepository() *repository.Repository {
	repo, err := OpenRepository(globalOptions)
	if err != nil {
		Exitf(1, "Failed to open the repository: %v", err)
	}
	err = repo.LoadIndex(globalOptions.ctx)
	if err != nil {
		Exitf(1, "Failed to load the index repository: %v", err)
	}
	return repo
}

func runWeb(opts WebOptions, gopts GlobalOptions, args []string) error {
	webRepository = loadRepository()

	router := mux.NewRouter()
	// API
	router.PathPrefix("/api/").Handler(web.CreateRouterAPI(globalOptions.ctx, webRepository))
	// web
	router.PathPrefix("/web/").Handler(web.CreateRouterWeb(globalOptions.ctx, webRepository))
	// static files like js, css for now...
	fs := http.FileServer(http.Dir("templates/web/static"))
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", fs))

	Verbosef("http://localhost:8080/web/\n")

	return http.ListenAndServe(":8080", router)
}
