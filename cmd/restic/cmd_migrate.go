package main

import (
	"context"

	"github.com/restic/restic/internal/migrations"
	"github.com/restic/restic/internal/restic"

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

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMigrate(cmd.Context(), migrateOptions, globalOptions, args)
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

func checkMigrations(ctx context.Context, repo restic.Repository) error {
	Printf("available migrations:\n")
	found := false

	for _, m := range migrations.All {
		ok, _, err := m.Check(ctx, repo)
		if err != nil {
			return err
		}

		if ok {
			Printf("  %v\t%v\n", m.Name(), m.Desc())
			found = true
		}
	}

	if !found {
		Printf("no migrations found\n")
	}

	return nil
}

func applyMigrations(ctx context.Context, opts MigrateOptions, gopts GlobalOptions, repo restic.Repository, args []string) error {
	var firsterr error
	for _, name := range args {
		for _, m := range migrations.All {
			if m.Name() == name {
				ok, reason, err := m.Check(ctx, repo)
				if err != nil {
					return err
				}

				if !ok {
					if !opts.Force {
						if reason == "" {
							reason = "check failed"
						}
						Warnf("migration %v cannot be applied: %v\nIf you want to apply this migration anyway, re-run with option --force\n", m.Name(), reason)
						continue
					}

					Warnf("check for migration %v failed, continuing anyway\n", m.Name())
				}

				if m.RepoCheck() {
					Printf("checking repository integrity...\n")

					checkOptions := CheckOptions{}
					checkGopts := gopts
					// the repository is already locked
					checkGopts.NoLock = true
					err = runCheck(ctx, checkOptions, checkGopts, []string{})
					if err != nil {
						return err
					}
				}

				Printf("applying migration %v...\n", m.Name())
				if err = m.Apply(ctx, repo); err != nil {
					Warnf("migration %v failed: %v\n", m.Name(), err)
					if firsterr == nil {
						firsterr = err
					}
					continue
				}

				Printf("migration %v: success\n", m.Name())
			}
		}
	}

	return firsterr
}

func runMigrate(ctx context.Context, opts MigrateOptions, gopts GlobalOptions, args []string) error {
	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	lock, ctx, err := lockRepoExclusive(ctx, repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return checkMigrations(ctx, repo)
	}

	return applyMigrations(ctx, opts, gopts, repo, args)
}
