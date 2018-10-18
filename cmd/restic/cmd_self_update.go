// xbuild selfupdate

package main

import (
	"os"

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
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSelfUpdate(selfUpdateOptions, globalOptions, args)
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

func runSelfUpdate(opts SelfUpdateOptions, gopts GlobalOptions, args []string) error {
	if opts.Output == "" {
		file, err := os.Executable()
		if err != nil {
			return errors.Wrap(err, "unable to find executable")
		}

		opts.Output = file
	}

	fi, err := os.Lstat(opts.Output)
	if err != nil {
		return err
	}

	if !fi.Mode().IsRegular() {
		return errors.Errorf("output file %v is not a normal file, use --output to specify a different file", opts.Output)
	}

	Printf("writing restic to %v\n", opts.Output)

	v, err := selfupdate.DownloadLatestStableRelease(gopts.ctx, opts.Output, version, Verbosef)
	if err != nil {
		return errors.Fatalf("unable to update restic: %v", err)
	}

	if v != version {
		Printf("successfully updated restic to version %v\n", v)
	}

	return nil
}
