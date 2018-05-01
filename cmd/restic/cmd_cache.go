package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/restic/restic/internal/cache"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/spf13/cobra"
)

var cmdCache = &cobra.Command{
	Use:   "cache",
	Short: "Operate on local cache directories",
	Long: `
The "cache" command allows listing and cleaning local cache directories.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCache(cacheOptions, globalOptions, args)
	},
}

// CacheOptions bundles all options for the snapshots command.
type CacheOptions struct {
	Cleanup bool
	MaxAge  uint
}

var cacheOptions CacheOptions

func init() {
	cmdRoot.AddCommand(cmdCache)

	f := cmdCache.Flags()
	f.BoolVar(&cacheOptions.Cleanup, "cleanup", false, "remove old cache directories")
	f.UintVar(&cacheOptions.MaxAge, "max-age", 30, "max age in `days` for cache directories to be considered old")
}

func runCache(opts CacheOptions, gopts GlobalOptions, args []string) error {
	if len(args) > 0 {
		return errors.Fatal("the cache command has no arguments")
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
			Verbosef("no old cache dirs found\n")
			return nil
		}

		Verbosef("remove %d old cache directories\n", len(oldDirs))

		for _, item := range oldDirs {
			dir := filepath.Join(cachedir, item.Name())
			err = fs.RemoveAll(dir)
			if err != nil {
				Warnf("unable to remove %v: %v\n", dir, err)
			}
		}

		return nil
	}

	tab := NewTable()
	tab.Header = fmt.Sprintf("%-14s   %-16s %s", "Repository ID", "Last Used", "Old")
	tab.RowFormat = "%-14s   %-16s %s"

	dirs, err := cache.All(cachedir)
	if err != nil {
		return err
	}

	if len(dirs) == 0 {
		Printf("no cache dirs found, basedir is %v\n", cachedir)
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

		tab.Rows = append(tab.Rows, []interface{}{
			entry.Name()[:10],
			fmt.Sprintf("%d days ago", uint(time.Since(entry.ModTime()).Hours()/24)),
			old,
		})
	}

	tab.Write(gopts.stdout)

	return nil
}
