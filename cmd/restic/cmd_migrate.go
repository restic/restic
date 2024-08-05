package main

import (
	"context"

	"github.com/restic/restic/internal/migrations"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/restic/restic/internal/ui/termstatus"

	"github.com/spf13/cobra"
)

var cmdMigrate = &cobra.Command{
	Use:   "migrate [flags] [migration name] [...]",
	Short: "Apply migrations",
	Long: `
The "migrate" command checks which migrations can be applied for a repository
and prints a list with available migration names. If one or more migration
names are specified, these migrations are applied.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		term, cancel := setupTermstatus()
		defer cancel()
		return runMigrate(cmd.Context(), migrateOptions, globalOptions, args, term)
	},
}

// MigrateOptions bundles all options for the 'check' command.
type MigrateOptions struct {
	Force bool
}

var migrateOptions MigrateOptions

func init() {
	cmdRoot.AddCommand(cmdMigrate)
	f := cmdMigrate.Flags()
	f.BoolVarP(&migrateOptions.Force, "force", "f", false, `apply a migration a second time`)
}

func checkMigrations(ctx context.Context, repo restic.Repository, printer progress.Printer) error {
	printer.P("available migrations:\n")
	found := false

	for _, m := range migrations.All {
		ok, _, err := m.Check(ctx, repo)
		if err != nil {
			return err
		}

		if ok {
			printer.P("  %v\t%v\n", m.Name(), m.Desc())
			found = true
		}
	}

	if !found {
		printer.P("no migrations found\n")
	}

	return nil
}

func applyMigrations(ctx context.Context, opts MigrateOptions, gopts GlobalOptions, repo restic.Repository, args []string, term *termstatus.Terminal, printer progress.Printer) error {
	var firsterr error
	for _, name := range args {
		found := false
		for _, m := range migrations.All {
			if m.Name() == name {
				found = true
				ok, reason, err := m.Check(ctx, repo)
				if err != nil {
					return err
				}

				if !ok {
					if !opts.Force {
						if reason == "" {
							reason = "check failed"
						}
						printer.E("migration %v cannot be applied: %v\nIf you want to apply this migration anyway, re-run with option --force\n", m.Name(), reason)
						continue
					}

					printer.E("check for migration %v failed, continuing anyway\n", m.Name())
				}

				if m.RepoCheck() {
					printer.P("checking repository integrity...\n")

					checkOptions := CheckOptions{}
					checkGopts := gopts
					// the repository is already locked
					checkGopts.NoLock = true

					err = runCheck(ctx, checkOptions, checkGopts, []string{}, term)
					if err != nil {
						return err
					}
				}

				printer.P("applying migration %v...\n", m.Name())
				if err = m.Apply(ctx, repo); err != nil {
					printer.E("migration %v failed: %v\n", m.Name(), err)
					if firsterr == nil {
						firsterr = err
					}
					continue
				}

				printer.P("migration %v: success\n", m.Name())
			}
		}
		if !found {
			printer.E("unknown migration %v", name)
		}
	}

	return firsterr
}

func runMigrate(ctx context.Context, opts MigrateOptions, gopts GlobalOptions, args []string, term *termstatus.Terminal) error {
	printer := newTerminalProgressPrinter(gopts.verbosity, term)

	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, false)
	if err != nil {
		return err
	}
	defer unlock()

	if len(args) == 0 {
		return checkMigrations(ctx, repo, printer)
	}

	return applyMigrations(ctx, opts, gopts, repo, args, term, printer)
}
