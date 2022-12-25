//go:build selfupdate

package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/selfupdate"
	"github.com/spf13/cobra"
)

var cmdSelfUpdate = &cobra.Command{
	Use:   "self-update [flags]",
	Short: "Update the restic binary",
	Long: `
The command "self-update" downloads the latest stable release of restic from
GitHub and replaces the currently running binary. After download, the
authenticity of the binary is verified using the GPG signature on the release
files.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSelfUpdate(cmd.Context(), selfUpdateOptions, globalOptions, args)
	},
}

// SelfUpdateOptions collects all options for the update-restic command.
type SelfUpdateOptions struct {
	Output string
}

var selfUpdateOptions SelfUpdateOptions

func init() {
	cmdRoot.AddCommand(cmdSelfUpdate)

	flags := cmdSelfUpdate.Flags()
	flags.StringVar(&selfUpdateOptions.Output, "output", "", "Save the downloaded file as `filename` (default: running binary itself)")
}

func runSelfUpdate(ctx context.Context, opts SelfUpdateOptions, gopts GlobalOptions, args []string) error {
	if opts.Output == "" {
		file, err := os.Executable()
		if err != nil {
			return errors.Wrap(err, "unable to find executable")
		}

		opts.Output = file
	}

	fi, err := os.Lstat(opts.Output)
	if err != nil {
		dirname := filepath.Dir(opts.Output)
		di, err := os.Lstat(dirname)
		if err != nil {
			return err
		}
		if !di.Mode().IsDir() {
			return errors.Fatalf("output parent path %v is not a directory, use --output to specify a different file path", dirname)
		}
	} else {
		if !fi.Mode().IsRegular() {
			return errors.Fatalf("output path %v is not a normal file, use --output to specify a different file path", opts.Output)
		}
	}

	Verbosef("writing restic to %v\n", opts.Output)

	v, err := selfupdate.DownloadLatestStableRelease(ctx, opts.Output, version, Verbosef)
	if err != nil {
		return errors.Fatalf("unable to update restic: %v", err)
	}

	if v != version {
		Printf("successfully updated restic to version %v\n", v)
	}

	return nil
}
