// +build selfupdate

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
The command "update-restic" downloads the latest stable release of restic from
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
	flags.StringVar(&selfUpdateOptions.Output, "output", os.Args[0], "Save the downloaded file as `filename`")
}

func runSelfUpdate(opts SelfUpdateOptions, gopts GlobalOptions, args []string) error {
	v, err := selfupdate.DownloadLatestStableRelease(gopts.ctx, opts.Output, Verbosef)
	if err != nil {
		return errors.Fatalf("unable to update restic: %v", err)
	}

	Printf("successfully updated restic to version %v\n", v)

	return nil
}
