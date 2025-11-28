//go:build darwin || freebsd || linux || windows
// +build darwin freebsd linux windows

package main

import (
	"context"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fuse"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/ui"
	"golang.org/x/net/webdav"
)

func registerWebdavCommand(cmdRoot *cobra.Command, globalOptions *global.Options) {
	cmdRoot.AddCommand(newWebdavCommand(globalOptions))
}

func newWebdavCommand(globalOptions *global.Options) *cobra.Command {
	var opts WebdavOptions

	cmd := &cobra.Command{
		Use:   "webdav [flags]",
		Short: "Run a WebDAV server for browsing the repository",
		Long: `
The "webdav" command runs a WebDAV server, which allows browsing the repository
contents with a WebDAV client. This is a read-only view.

The server will listen on the address specified with --listen.
`,
		DisableAutoGenTag: true,
		GroupID:           cmdGroupDefault,
		RunE: func(cmd *cobra.Command, args []string) error {
			finalizeSnapshotFilter(&opts.SnapshotFilter)
			return runWebdav(cmd.Context(), opts, *globalOptions, args, globalOptions.Term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// WebdavOptions collects all options for the webdav command.
type WebdavOptions struct {
	Listen               string
	WebdavUser           string
	WebdavPass           string
	OwnerRoot            bool
	AllowOther           bool
	NoDefaultPermissions bool
	data.SnapshotFilter
	TimeTemplate  string
	PathTemplates []string
}

func (opts *WebdavOptions) AddFlags(f *pflag.FlagSet) {
	f.StringVar(&opts.Listen, "listen", "localhost:8080", "listen address")
	f.StringVar(&opts.WebdavUser, "webdav-user", "", "username for authentication")
	f.StringVar(&opts.WebdavPass, "webdav-pass", "", "password for authentication")
	f.BoolVar(&opts.OwnerRoot, "owner-root", false, "use 'root' as the owner of files and dirs")

	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)

	f.StringArrayVar(&opts.PathTemplates, "path-template", nil, "set `template` for path names (can be specified multiple times)")
	f.StringVar(&opts.TimeTemplate, "snapshot-template", time.RFC3339, "set `template` to use for snapshot dirs")
	f.StringVar(&opts.TimeTemplate, "time-template", time.RFC3339, "set `template` to use for times")
	_ = f.MarkDeprecated("snapshot-template", "use --time-template")
}

func runWebdav(ctx context.Context, opts WebdavOptions, gopts global.Options, args []string, term ui.Terminal) error {
	if len(args) != 0 {
		return errors.Fatal("the webdav command does not accept any arguments")
	}

	printer := ui.NewProgressPrinter(false, gopts.Verbosity, term)

	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock, printer)
	if err != nil {
		return err
	}
	defer unlock()

	err = repo.LoadIndex(ctx, printer)
	if err != nil {
		return err
	}

	cfg := fuse.Config{
		OwnerIsRoot:   opts.OwnerRoot,
		Filter:        opts.SnapshotFilter,
		TimeTemplate:  opts.TimeTemplate,
		PathTemplates: opts.PathTemplates,
	}
	root := fuse.NewRoot(repo, cfg)
	fs := fuse.NewWebdavFS(root)

	handler := http.Handler(&webdav.Handler{
		FileSystem: fs,
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				printer.E("WEBDAV [%s]: %s, ERROR: %s\n", r.Method, r.URL, err)
			} else {
				printer.V("WEBDAV [%s]: %s\n", r.Method, r.URL)
			}
		},
	})

	if opts.WebdavUser != "" || opts.WebdavPass != "" {
		handler = authHandler(handler, opts.WebdavUser, opts.WebdavPass)
	}

	printer.S("Now serving WebDAV at http://%s\n", opts.Listen)
	printer.S("When finished, quit with Ctrl-c here.")

	srv := &http.Server{
		Addr:    opts.Listen,
		Handler: handler,
	}

	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()

	err = srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return errors.Fatalf("server error: %v", err)
	}

	return nil
}

func authHandler(handler http.Handler, username, password string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != username || pass != password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	})
}