package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/restic/restic/internal/backend/cache"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/table"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newCacheCommand(globalOptions *global.Options) *cobra.Command {
	var opts CacheOptions

	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Operate on local cache directories",
		Long: `
The "cache" command allows listing and cleaning local cache directories.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
`,
		GroupID:           cmdGroupDefault,
		DisableAutoGenTag: true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runCache(opts, *globalOptions, args, globalOptions.Term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// CacheOptions bundles all options for the snapshots command.
type CacheOptions struct {
	Cleanup bool
	MaxAge  uint
	NoSize  bool
}

func (opts *CacheOptions) AddFlags(f *pflag.FlagSet) {
	f.BoolVar(&opts.Cleanup, "cleanup", false, "remove old cache directories")
	f.UintVar(&opts.MaxAge, "max-age", 30, "max age in `days` for cache directories to be considered old")
	f.BoolVar(&opts.NoSize, "no-size", false, "do not output the size of the cache directories")
}

func runCache(opts CacheOptions, gopts global.Options, args []string, term ui.Terminal) error {
	printer := ui.NewProgressPrinter(false, gopts.Verbosity, term)

	if len(args) > 0 {
		return errors.Fatal("the cache command expects no arguments, only options - please see `restic help cache` for usage and flags")
	}

	if gopts.NoCache {
		return errors.Fatal("Refusing to do anything, the cache is disabled")
	}

	var (
		cachedir = gopts.CacheDir
		err      error
	)

	if cachedir == "" {
		cachedir, err = cache.DefaultDir()
		if err != nil {
			return err
		}
	}

	if opts.Cleanup || gopts.CleanupCache {
		oldDirs, err := cache.OlderThan(cachedir, time.Duration(opts.MaxAge)*24*time.Hour)
		if err != nil {
			return err
		}

		if len(oldDirs) == 0 {
			printer.P("no old cache dirs found")
			return nil
		}

		printer.P("remove %d old cache directories", len(oldDirs))

		for _, item := range oldDirs {
			dir := filepath.Join(cachedir, item.Name())
			err = os.RemoveAll(dir)
			if err != nil {
				printer.E("unable to remove %v: %v", dir, err)
			}
		}

		return nil
	}

	tab := table.New()

	type data struct {
		ID   string
		Last string
		Old  string
		Size string
	}

	tab.AddColumn("Repo ID", "{{ .ID }}")
	tab.AddColumn("Last Used", "{{ .Last }}")
	tab.AddColumn("Old", "{{ .Old }}")

	if !opts.NoSize {
		tab.AddColumn("Size", "{{ .Size }}")
	}

	dirs, err := cache.All(cachedir)
	if err != nil {
		return err
	}

	if len(dirs) == 0 {
		printer.S("no cache dirs found, basedir is %v", cachedir)
		return nil
	}

	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].ModTime().Before(dirs[j].ModTime())
	})

	for _, entry := range dirs {
		var old string
		if cache.IsOld(entry.ModTime(), time.Duration(opts.MaxAge)*24*time.Hour) {
			old = "yes"
		}

		var size string
		if !opts.NoSize {
			bytes, err := dirSize(filepath.Join(cachedir, entry.Name()))
			if err != nil {
				return err
			}
			size = fmt.Sprintf("%11s", ui.FormatBytes(uint64(bytes)))
		}

		name := entry.Name()
		if !strings.HasPrefix(name, "restic-check-cache-") {
			name = name[:10]
		}

		tab.AddRow(data{
			name,
			fmt.Sprintf("%d days ago", uint(time.Since(entry.ModTime()).Hours()/24)),
			old,
			size,
		})
	}

	_ = tab.Write(gopts.Term.OutputWriter())
	printer.S("%d cache dirs in %s", len(dirs), cachedir)

	return nil
}

func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return err
		}

		if !info.IsDir() {
			size += info.Size()
		}

		return nil
	})
	return size, err
}
