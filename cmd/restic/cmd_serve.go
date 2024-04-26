package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/server"
)

var cmdServe = &cobra.Command{
	Use:   "serve",
	Short: "runs a web server to browse a repository",
	Long: `
The serve command runs a web server to browse a repository.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWebServer(cmd.Context(), serveOptions, globalOptions, args)
	},
}

type ServeOptions struct {
	Listen string
}

var serveOptions ServeOptions

func init() {
	cmdRoot.AddCommand(cmdServe)
	cmdFlags := cmdServe.Flags()
	cmdFlags.StringVarP(&serveOptions.Listen, "listen", "l", "localhost:3080", "set the listen host name and `address`")
}

const serverShutdownTimeout = 30 * time.Second

func runWebServer(ctx context.Context, opts ServeOptions, gopts GlobalOptions, args []string) error {
	if len(args) > 0 {
		return errors.Fatal("this command does not accept additional arguments")
	}

	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, gopts.NoLock)
	if err != nil {
		return err
	}
	defer unlock()

	snapshotLister, err := restic.MemorizeList(ctx, repo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	bar := newIndexProgress(gopts.Quiet, gopts.JSON)
	err = repo.LoadIndex(ctx, bar)
	if err != nil {
		return err
	}

	srv := http.Server{
		BaseContext: func(l net.Listener) context.Context {
			// just return the global context
			return ctx
		},
		Handler: server.New(repo, snapshotLister, TimeFormat),
	}

	listener, err := net.Listen("tcp", opts.Listen)
	if err != nil {
		return fmt.Errorf("start listener: %v", err)
	}

	// wait until context is cancelled, then close listener
	go func() {
		<-ctx.Done()
		Printf("gracefully shutting down server\n")

		ctxTimeout, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
		defer cancel()

		_ = srv.Shutdown(ctxTimeout)
	}()

	Printf("Now serving the repository at http://%s\n", opts.Listen)
	Printf("When finished, quit with Ctrl-c here.\n")

	err = srv.Serve(listener)

	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}

	if err != nil {
		return fmt.Errorf("serve: %v", err)
	}

	return nil
}
