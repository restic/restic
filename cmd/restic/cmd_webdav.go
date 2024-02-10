package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fuse"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/webdav"
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

	cfg := fuse.Config{
		Filter:        opts.SnapshotFilter,
		TimeTemplate:  opts.TimeTemplate,
		PathTemplates: opts.PathTemplates,
	}
	root := fuse.NewRoot(repo, cfg)

	h, err := webdav.NewWebDAV(ctx, root)
	if err != nil {
		return err
	}

	srv := &http.Server{
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		Addr:         opts.Listen,
		Handler:      h,
		ErrorLog:     errorLogger,
	}

	return srv.ListenAndServe()
}
