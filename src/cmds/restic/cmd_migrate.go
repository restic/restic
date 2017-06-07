package main

import (
	"restic"
	"restic/migrations"

	"github.com/spf13/cobra"
)

var cmdMigrate = &cobra.Command{
	Use:   "migrate [name]",
	Short: "apply migrations",
	Long: `
The "migrate" command applies migrations to a repository. When no migration
name is explicitely given, a list of migrations that can be applied is printed.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMigrate(migrateOptions, globalOptions, args)
	},
}

// MigrateOptions bundles all options for the 'check' command.
type MigrateOptions struct {
}

var migrateOptions MigrateOptions

func init() {
	cmdRoot.AddCommand(cmdMigrate)
}

func checkMigrations(opts MigrateOptions, gopts GlobalOptions, repo restic.Repository) error {
	ctx := gopts.ctx
	Printf("available migrations:\n")
	for _, m := range migrations.All {
		ok, err := m.Check(ctx, repo)
		if err != nil {
			return err
		}

		if ok {
			Printf("  %v\n", m.Name())
		}
	}

	return nil
}

func applyMigrations(opts MigrateOptions, gopts GlobalOptions, repo restic.Repository, args []string) error {
	ctx := gopts.ctx

	var firsterr error
	for _, name := range args {
		for _, m := range migrations.All {
			if m.Name() == name {
				ok, err := m.Check(ctx, repo)
				if err != nil {
					return err
				}

				if !ok {
					Warnf("migration %v cannot be applied: check failed\n")
					continue
				}

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

func runMigrate(opts MigrateOptions, gopts GlobalOptions, args []string) error {
	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	lock, err := lockRepoExclusive(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return checkMigrations(opts, gopts, repo)
	}

	return applyMigrations(opts, gopts, repo, args)
}
