package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/net/webdav"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/server/rofs"
)

var cmdWebDAV = &cobra.Command{
	Use:   "webdav [flags]",
	Short: "runs a WebDAV server for the repository",
	Long: `
The webdav command runs a WebDAV server for the reposiotry that you can then access via a WebDAV client.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWebDAV(cmd.Context(), webdavOptions, globalOptions, args)
	},
}

// WebDAVOptions collects all options for the webdav command.
type WebDAVOptions struct {
	Listen string

	restic.SnapshotFilter
	TimeTemplate  string
	PathTemplates []string
}

var webdavOptions WebDAVOptions

func init() {
	cmdRoot.AddCommand(cmdWebDAV)

	fs := cmdWebDAV.Flags()
	fs.StringVarP(&webdavOptions.Listen, "listen", "l", "localhost:3080", "set the listen host name and `address`")

	initMultiSnapshotFilter(fs, &webdavOptions.SnapshotFilter, true)

	fs.StringArrayVar(&webdavOptions.PathTemplates, "path-template", nil, "set `template` for path names (can be specified multiple times)")
	fs.StringVar(&webdavOptions.TimeTemplate, "time-template", time.RFC3339, "set `template` to use for times")
}

func runWebDAV(ctx context.Context, opts WebDAVOptions, gopts GlobalOptions, args []string) error {
	if len(args) > 0 {
		return errors.Fatal("this command does not accept additional arguments")
	}

	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	lock, ctx, err := lockRepo(ctx, repo, gopts.RetryLock, gopts.JSON)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	bar := newIndexProgress(gopts.Quiet, gopts.JSON)
	err = repo.LoadIndex(ctx, bar)
	if err != nil {
		return err
	}

	errorLogger := log.New(os.Stderr, "error log: ", log.Flags())

	cfg := rofs.Config{
		Filter:        opts.SnapshotFilter,
		TimeTemplate:  opts.TimeTemplate,
		PathTemplates: opts.PathTemplates,
	}

	root, err := rofs.New(ctx, repo, cfg)
	if err != nil {
		return err
	}

	// root := os.DirFS(".")

	// h, err := webdav.NewWebDAV(ctx, root)
	// if err != nil {
	// 	return err
	// }

	// root := fstest.MapFS{
	// 	"foobar": &fstest.MapFile{
	// 		Data:    []byte("foobar test content"),
	// 		Mode:    0644,
	// 		ModTime: time.Now(),
	// 	},
	// 	"test.txt": &fstest.MapFile{
	// 		Data:    []byte("other file"),
	// 		Mode:    0640,
	// 		ModTime: time.Now(),
	// 	},
	// }

	logRequest := func(req *http.Request, err error) {
		errorLogger.Printf("req %v %v -> %v\n", req.Method, req.URL.Path, err)
	}

	srv := &http.Server{
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		Addr:         opts.Listen,
		// Handler:      http.FileServer(http.FS(root)),
		Handler: &webdav.Handler{
			FileSystem: rofs.WebDAVFS(root),
			LockSystem: webdav.NewMemLS(),
			Logger:     logRequest,
		},
		ErrorLog: errorLogger,
	}

	return srv.ListenAndServe()
}
