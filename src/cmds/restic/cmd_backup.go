package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"restic"
	"strings"
	"time"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/spf13/cobra"

	"restic/archiver"
	"restic/debug"
	"restic/errors"
	"restic/filter"
	"restic/fs"
)

var cmdBackup = &cobra.Command{
	Use:   "backup [flags] FILE/DIR [FILE/DIR] ...",
	Short: "create a new backup of files and/or directories",
	Long: `
The "backup" command creates a new snapshot and saves the files and directories
given as the arguments.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if backupOptions.Stdin {
			return readBackupFromStdin(backupOptions, globalOptions, args)
		}

		return runBackup(backupOptions, globalOptions, args)
	},
}

// BackupOptions bundles all options for the backup command.
type BackupOptions struct {
	Parent         string
	Force          bool
	Excludes       []string
	ExcludeFile    string
	ExcludeOtherFS bool
	Stdin          bool
	StdinFilename  string
	Tags           []string
}

var backupOptions BackupOptions

func init() {
	cmdRoot.AddCommand(cmdBackup)

	f := cmdBackup.Flags()
	f.StringVar(&backupOptions.Parent, "parent", "", "use this parent snapshot (default: last snapshot in the repo that has the same target files/directories)")
	f.BoolVarP(&backupOptions.Force, "force", "f", false, `force re-reading the target files/directories. Overrides the "parent" flag`)
	f.StringSliceVarP(&backupOptions.Excludes, "exclude", "e", []string{}, "exclude a `pattern` (can be specified multiple times)")
	f.StringVar(&backupOptions.ExcludeFile, "exclude-file", "", "read exclude patterns from a file")
	f.BoolVarP(&backupOptions.ExcludeOtherFS, "one-file-system", "x", false, "Exclude other file systems")
	f.BoolVar(&backupOptions.Stdin, "stdin", false, "read backup from stdin")
	f.StringVar(&backupOptions.StdinFilename, "stdin-filename", "", "file name to use when reading from stdin")
	f.StringSliceVar(&backupOptions.Tags, "tag", []string{}, "add a `tag` for the new snapshot (can be specified multiple times)")
}

func newScanProgress(gopts GlobalOptions) *restic.Progress {
	if gopts.Quiet {
		return nil
	}

	p := restic.NewProgress()
	p.OnUpdate = func(s restic.Stat, d time.Duration, ticker bool) {
		PrintProgress("[%s] %d directories, %d files, %s", formatDuration(d), s.Dirs, s.Files, formatBytes(s.Bytes))
	}
	p.OnDone = func(s restic.Stat, d time.Duration, ticker bool) {
		PrintProgress("scanned %d directories, %d files in %s\n", s.Dirs, s.Files, formatDuration(d))
	}

	return p
}

func newArchiveProgress(gopts GlobalOptions, todo restic.Stat) *restic.Progress {
	if gopts.Quiet {
		return nil
	}

	archiveProgress := restic.NewProgress()

	var bps, eta uint64
	itemsTodo := todo.Files + todo.Dirs

	archiveProgress.OnUpdate = func(s restic.Stat, d time.Duration, ticker bool) {
		sec := uint64(d / time.Second)
		if todo.Bytes > 0 && sec > 0 && ticker {
			bps = s.Bytes / sec
			if s.Bytes >= todo.Bytes {
				eta = 0
			} else if bps > 0 {
				eta = (todo.Bytes - s.Bytes) / bps
			}
		}

		itemsDone := s.Files + s.Dirs

		status1 := fmt.Sprintf("[%s] %s  %s/s  %s / %s  %d / %d items  %d errors  ",
			formatDuration(d),
			formatPercent(s.Bytes, todo.Bytes),
			formatBytes(bps),
			formatBytes(s.Bytes), formatBytes(todo.Bytes),
			itemsDone, itemsTodo,
			s.Errors)
		status2 := fmt.Sprintf("ETA %s ", formatSeconds(eta))

		w, _, err := terminal.GetSize(int(os.Stdout.Fd()))
		if err == nil {
			maxlen := w - len(status2) - 1

			if maxlen < 4 {
				status1 = ""
			} else if len(status1) > maxlen {
				status1 = status1[:maxlen-4]
				status1 += "... "
			}
		}

		PrintProgress("%s%s", status1, status2)
	}

	archiveProgress.OnDone = func(s restic.Stat, d time.Duration, ticker bool) {
		fmt.Printf("\nduration: %s, %s\n", formatDuration(d), formatRate(todo.Bytes, d))
	}

	return archiveProgress
}

func newArchiveStdinProgress(gopts GlobalOptions) *restic.Progress {
	if gopts.Quiet {
		return nil
	}

	archiveProgress := restic.NewProgress()

	var bps uint64

	archiveProgress.OnUpdate = func(s restic.Stat, d time.Duration, ticker bool) {
		sec := uint64(d / time.Second)
		if s.Bytes > 0 && sec > 0 && ticker {
			bps = s.Bytes / sec
		}

		status1 := fmt.Sprintf("[%s] %s  %s/s", formatDuration(d),
			formatBytes(s.Bytes),
			formatBytes(bps))

		w, _, err := terminal.GetSize(int(os.Stdout.Fd()))
		if err == nil {
			maxlen := w - len(status1)

			if maxlen < 4 {
				status1 = ""
			} else if len(status1) > maxlen {
				status1 = status1[:maxlen-4]
				status1 += "... "
			}
		}

		PrintProgress("%s", status1)
	}

	archiveProgress.OnDone = func(s restic.Stat, d time.Duration, ticker bool) {
		fmt.Printf("\nduration: %s, %s\n", formatDuration(d), formatRate(s.Bytes, d))
	}

	return archiveProgress
}

// filterExisting returns a slice of all existing items, or an error if no
// items exist at all.
func filterExisting(items []string) (result []string, err error) {
	for _, item := range items {
		_, err := fs.Lstat(item)
		if err != nil && os.IsNotExist(errors.Cause(err)) {
			continue
		}

		result = append(result, item)
	}

	if len(result) == 0 {
		return nil, errors.Fatal("all target directories/files do not exist")
	}

	return
}

// gatherDevices returns the set of unique device ids of the files and/or
// directory paths listed in "items".
func gatherDevices(items []string) (deviceMap map[uint64]struct{}, err error) {
	deviceMap = make(map[uint64]struct{})
	for _, item := range items {
		fi, err := fs.Lstat(item)
		if err != nil {
			return nil, err
		}
		id, err := fs.DeviceID(fi)
		if err != nil {
			return nil, err
		}
		deviceMap[id] = struct{}{}
	}
	if len(deviceMap) == 0 {
		return nil, errors.New("zero allowed devices")
	}
	return deviceMap, nil
}

func readBackupFromStdin(opts BackupOptions, gopts GlobalOptions, args []string) error {
	if len(args) != 0 {
		return errors.Fatalf("when reading from stdin, no additional files can be specified")
	}

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	lock, err := lockRepo(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	err = repo.LoadIndex()
	if err != nil {
		return err
	}

	_, id, err := archiver.ArchiveReader(repo, newArchiveStdinProgress(gopts), os.Stdin, opts.StdinFilename, opts.Tags)
	if err != nil {
		return err
	}

	fmt.Printf("archived as %v\n", id.Str())
	return nil
}

func runBackup(opts BackupOptions, gopts GlobalOptions, args []string) error {
	if len(args) == 0 {
		return errors.Fatalf("wrong number of parameters")
	}

	target := make([]string, 0, len(args))
	for _, d := range args {
		if a, err := filepath.Abs(d); err == nil {
			d = a
		}
		target = append(target, d)
	}

	target, err := filterExisting(target)
	if err != nil {
		return err
	}

	// allowed devices
	var allowedDevs map[uint64]struct{}
	if opts.ExcludeOtherFS {
		allowedDevs, err = gatherDevices(target)
		if err != nil {
			return err
		}
		debug.Log("allowed devices: %v\n", allowedDevs)
	}

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	lock, err := lockRepo(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	err = repo.LoadIndex()
	if err != nil {
		return err
	}

	var parentSnapshotID *restic.ID

	// Force using a parent
	if !opts.Force && opts.Parent != "" {
		id, err := restic.FindSnapshot(repo, opts.Parent)
		if err != nil {
			return errors.Fatalf("invalid id %q: %v", opts.Parent, err)
		}

		parentSnapshotID = &id
	}

	// Find last snapshot to set it as parent, if not already set
	if !opts.Force && parentSnapshotID == nil {
		id, err := restic.FindLatestSnapshot(repo, target, "")
		if err == nil {
			parentSnapshotID = &id
		} else if err != restic.ErrNoSnapshotFound {
			return err
		}
	}

	if parentSnapshotID != nil {
		Verbosef("using parent snapshot %v\n", parentSnapshotID.Str())
	}

	Verbosef("scan %v\n", target)

	// add patterns from file
	if opts.ExcludeFile != "" {
		file, err := fs.Open(opts.ExcludeFile)
		if err != nil {
			Warnf("error reading exclude patterns: %v", err)
			return nil
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "#") {
				line = os.ExpandEnv(line)
				opts.Excludes = append(opts.Excludes, line)
			}
		}
	}

	selectFilter := func(item string, fi os.FileInfo) bool {
		matched, err := filter.List(opts.Excludes, item)
		if err != nil {
			Warnf("error for exclude pattern: %v", err)
		}

		if matched {
			debug.Log("path %q excluded by a filter", item)
			return false
		}

		if !opts.ExcludeOtherFS {
			return true
		}

		id, err := fs.DeviceID(fi)
		if err != nil {
			// This should never happen because gatherDevices() would have
			// errored out earlier. If it still does that's a reason to panic.
			panic(err)
		}
		_, found := allowedDevs[id]
		if !found {
			debug.Log("path %q on disallowed device %d", item, id)
			return false
		}

		return true
	}

	stat, err := archiver.Scan(target, selectFilter, newScanProgress(gopts))
	if err != nil {
		return err
	}

	arch := archiver.New(repo)
	arch.Excludes = opts.Excludes
	arch.SelectFilter = selectFilter

	arch.Error = func(dir string, fi os.FileInfo, err error) error {
		// TODO: make ignoring errors configurable
		Warnf("%s\rerror for %s: %v\n", ClearLine(), dir, err)
		return nil
	}

	_, id, err := arch.Snapshot(newArchiveProgress(gopts, stat), target, opts.Tags, parentSnapshotID)
	if err != nil {
		return err
	}

	Verbosef("snapshot %s saved\n", id.Str())

	return nil
}
