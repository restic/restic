// +build !openbsd
// +build !windows

package main

import (
	"net/http"

	"github.com/spf13/cobra"

	"restic/debug"
	"restic/errors"

	//resticfs "restic/fs"
	"restic/fuse"
	resticWebdav "restic/webdav"

	"golang.org/x/net/webdav"
)

var cmdWeb = &cobra.Command{
	Use:   "web [flags] [[hostname]:port]",
	Short: "mount the repository as a WebDAV server",
	Long: `
The "web" command mounts the repository as a WebDAV server. This is a
read-only interface.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWeb(webOptions, globalOptions, args)
	},
}

// WebOptions collects all options for the mount command.
type WebOptions struct{}

var webOptions WebOptions

func init() {
	cmdRoot.AddCommand(cmdWeb)
}

func web(opts WebOptions, gopts GlobalOptions, address string) error {
	debug.Log("start web")
	defer debug.Log("finish web")

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	err = repo.LoadIndex()
	if err != nil {
		return err
	}

	fsHandler := &webdav.Handler{
		FileSystem: resticWebdav.NewWebdavFS(fuse.NewSnapshotsDir(repo, true)),
		LockSystem: webdav.NewMemLS(),
		Logger:     nil,
	}

	debug.Log("serving mount at %v", address)
	return http.ListenAndServe(address, fsHandler)
}

func runWeb(opts WebOptions, gopts GlobalOptions, args []string) error {
	if len(args) == 0 {
		return errors.Fatalf("wrong number of parameters")
	}

	address := args[0]

	return web(opts, gopts, address)
}
