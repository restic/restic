//go:build selfupdate

package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/selfupdate"
	"github.com/restic/restic/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func registerSelfUpdateCommand(cmd *cobra.Command, globalOptions *GlobalOptions) {
	cmd.AddCommand(
		newSelfUpdateCommand(globalOptions),
	)
}

func newSelfUpdateCommand(globalOptions *GlobalOptions) *cobra.Command {
	var opts SelfUpdateOptions

	cmd := &cobra.Command{
		Use:   "self-update [flags]",
		Short: "Update the restic binary",
		Long: `
The command "self-update" downloads the latest stable release of restic from
GitHub and replaces the currently running binary. After download, the
authenticity of the binary is verified using the GPG signature on the release
files.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
`,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSelfUpdate(cmd.Context(), opts, *globalOptions, args, globalOptions.term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// SelfUpdateOptions collects all options for the update-restic command.
type SelfUpdateOptions struct {
	Output string
}

func (opts *SelfUpdateOptions) AddFlags(f *pflag.FlagSet) {
	f.StringVar(&opts.Output, "output", "", "Save the downloaded file as `filename` (default: running binary itself)")
}

func runSelfUpdate(ctx context.Context, opts SelfUpdateOptions, gopts GlobalOptions, args []string, term ui.Terminal) error {
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

	printer := ui.NewProgressPrinter(false, gopts.verbosity, term)
	printer.P("writing restic to %v", opts.Output)

	v, err := selfupdate.DownloadLatestStableRelease(ctx, opts.Output, version, printer.P)
	if err != nil {
		return errors.Fatalf("unable to update restic: %v", err)
	}

	if v != version {
		printer.S("successfully updated restic to version %v", v)
	}

	return nil
}
