package main

import (
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
		return runWebDAV(webdavOptions, globalOptions, args)
	},
}

// WebDAVOptions collects all options for the webdav command.
type WebDAVOptions struct {
	Listen string

	Hosts            []string
	Tags             restic.TagLists
	Paths            []string
	SnapshotTemplate string
}

var webdavOptions WebDAVOptions

func init() {
	cmdRoot.AddCommand(cmdWebDAV)

	webdavFlags := cmdWebDAV.Flags()
	webdavFlags.StringVarP(&webdavOptions.Listen, "listen", "l", "localhost:3080", "set the listen host name and `address`")

	webdavFlags.StringArrayVarP(&webdavOptions.Hosts, "host", "H", nil, `only consider snapshots for this host (can be specified multiple times)`)
	webdavFlags.Var(&webdavOptions.Tags, "tag", "only consider snapshots which include this `taglist`")
	webdavFlags.StringArrayVar(&webdavOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path`")
	webdavFlags.StringVar(&webdavOptions.SnapshotTemplate, "snapshot-template", time.RFC3339, "set `template` to use for snapshot dirs")
}

func runWebDAV(opts WebDAVOptions, gopts GlobalOptions, args []string) error {
	if len(args) > 0 {
		return errors.Fatal("this command does not accept additional arguments")
	}

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	lock, err := lockRepo(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	err = repo.LoadIndex(gopts.ctx)
	if err != nil {
		return err
	}

	errorLogger := log.New(os.Stderr, "error log: ", log.Flags())

	cfg := fuse.Config{
		Hosts:            opts.Hosts,
		Tags:             opts.Tags,
		Paths:            opts.Paths,
		SnapshotTemplate: opts.SnapshotTemplate,
	}
	root := fuse.NewRoot(repo, cfg)

	h, err := webdav.NewWebDAV(gopts.ctx, root)
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
