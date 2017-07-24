package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

var cmdBackup = &cobra.Command{
	Use:   "backup [flags] FILE/DIR [FILE/DIR] ...",
	Short: "create a new backup of files and/or directories",
	Long: `
The "backup" command creates a new snapshot and saves the files and directories
given as the arguments.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if backupOptions.Stdin && backupOptions.FilesFrom == "-" {
			return errors.Fatal("cannot use both `--stdin` and `--files-from -`")
		}

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
	ExcludeFiles   []string
	ExcludeOtherFS bool
	Stdin          bool
	StdinFilename  string
	Tags           []string
	Hostname       string
	FilesFrom      string
}

var backupOptions BackupOptions

func init() {
	cmdRoot.AddCommand(cmdBackup)

	hostname, err := os.Hostname()
	if err != nil {
		debug.Log("os.Hostname() returned err: %v", err)
		hostname = ""
	}

	f := cmdBackup.Flags()
	f.StringVar(&backupOptions.Parent, "parent", "", "use this parent snapshot (default: last snapshot in the repo that has the same target files/directories)")
	f.BoolVarP(&backupOptions.Force, "force", "f", false, `force re-reading the target files/directories (overrides the "parent" flag)`)
	f.StringArrayVarP(&backupOptions.Excludes, "exclude", "e", nil, "exclude a `pattern` (can be specified multiple times)")
	f.StringArrayVar(&backupOptions.ExcludeFiles, "exclude-file", nil, "read exclude patterns from a `file` (can be specified multiple times)")
	f.BoolVarP(&backupOptions.ExcludeOtherFS, "one-file-system", "x", false, "exclude other file systems")
	f.BoolVar(&backupOptions.Stdin, "stdin", false, "read backup from stdin")
	f.StringVar(&backupOptions.StdinFilename, "stdin-filename", "stdin", "file name to use when reading from stdin")
	f.StringArrayVar(&backupOptions.Tags, "tag", nil, "add a `tag` for the new snapshot (can be specified multiple times)")
	f.StringVar(&backupOptions.Hostname, "hostname", hostname, "set the `hostname` for the snapshot manually")
	f.StringVar(&backupOptions.FilesFrom, "files-from", "", "read the files to backup from file (can be combined with file args)")
}

func newScanProgress(gopts GlobalOptions) *restic.Progress {
	if gopts.Quiet {
		return nil
	}

	p := restic.NewProgress()
	p.OnUpdate = func(s restic.Stat, d time.Duration, ticker bool) {
		if IsProcessBackground() {
			return
		}

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
		if IsProcessBackground() {
			return
		}

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

		if w := stdoutTerminalWidth(); w > 0 {
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
		if IsProcessBackground() {
			return
		}

		sec := uint64(d / time.Second)
		if s.Bytes > 0 && sec > 0 && ticker {
			bps = s.Bytes / sec
		}

		status1 := fmt.Sprintf("[%s] %s  %s/s", formatDuration(d),
			formatBytes(s.Bytes),
			formatBytes(bps))

		if w := stdoutTerminalWidth(); w > 0 {
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
func gatherDevices(items []string) (deviceMap map[string]uint64, err error) {
	deviceMap = make(map[string]uint64)
	for _, item := range items {
		fi, err := fs.Lstat(item)
		if err != nil {
			return nil, err
		}
		id, err := fs.DeviceID(fi)
		if err != nil {
			return nil, err
		}
		deviceMap[item] = id
	}
	if len(deviceMap) == 0 {
		return nil, errors.New("zero allowed devices")
	}
	return deviceMap, nil
}

func readBackupFromStdin(opts BackupOptions, gopts GlobalOptions, args []string) error {
	if len(args) != 0 {
		return errors.Fatal("when reading from stdin, no additional files can be specified")
	}

	if opts.StdinFilename == "" {
		return errors.Fatal("filename for backup from stdin must not be empty")
	}

	if gopts.password == "" && gopts.PasswordFile == "" {
		return errors.Fatal("unable to read password from stdin when data is to be read from stdin, use --password-file or $RESTIC_PASSWORD")
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

	err = repo.LoadIndex(context.TODO())
	if err != nil {
		return err
	}

	r := &archiver.Reader{
		Repository: repo,
		Tags:       opts.Tags,
		Hostname:   opts.Hostname,
	}

	_, id, err := r.Archive(context.TODO(), opts.StdinFilename, os.Stdin, newArchiveStdinProgress(gopts))
	if err != nil {
		return err
	}

	Verbosef("archived as %v\n", id.Str())
	return nil
}

// readFromFile will read all lines from the given filename and write them to a
// string array, if filename is empty readFromFile returns and empty string
// array. If filename is a dash (-), readFromFile will read the lines from
// the standard input.
func readLinesFromFile(filename string) ([]string, error) {
	if filename == "" {
		return nil, nil
	}

	var r io.Reader = os.Stdin
	if filename != "-" {
		f, err := os.Open(filename)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	}

	var lines []string

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

func runBackup(opts BackupOptions, gopts GlobalOptions, args []string) error {
	if opts.FilesFrom == "-" && gopts.password == "" && gopts.PasswordFile == "" {
		return errors.Fatal("no password; either use `--password-file` option or put the password into the RESTIC_PASSWORD environment variable")
	}

	fromfile, err := readLinesFromFile(opts.FilesFrom)
	if err != nil {
		return err
	}

	// merge files from files-from into normal args so we can reuse the normal
	// args checks and have the ability to use both files-from and args at the
	// same time
	args = append(args, fromfile...)
	if len(args) == 0 {
		return errors.Fatal("wrong number of parameters")
	}

	target := make([]string, 0, len(args))
	for _, d := range args {
		if a, err := filepath.Abs(d); err == nil {
			d = a
		}
		target = append(target, d)
	}

	target, err = filterExisting(target)
	if err != nil {
		return err
	}

	// allowed devices
	var allowedDevs map[string]uint64
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

	err = repo.LoadIndex(context.TODO())
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
		id, err := restic.FindLatestSnapshot(context.TODO(), repo, target, []restic.TagList{opts.Tags}, opts.Hostname)
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
	if len(opts.ExcludeFiles) > 0 {
		for _, filename := range opts.ExcludeFiles {
			file, err := fs.Open(filename)
			if err != nil {
				Warnf("error reading exclude patterns: %v", err)
				return nil
			}

			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())

				// ignore empty lines
				if line == "" {
					continue
				}

				// strip comments
				if strings.HasPrefix(line, "#") {
					continue
				}

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

		if !opts.ExcludeOtherFS || fi == nil {
			return true
		}

		id, err := fs.DeviceID(fi)
		if err != nil {
			// This should never happen because gatherDevices() would have
			// errored out earlier. If it still does that's a reason to panic.
			panic(err)
		}

		for dir := item; dir != ""; dir = filepath.Dir(dir) {
			debug.Log("item %v, test dir %v", item, dir)

			allowedID, ok := allowedDevs[dir]
			if !ok {
				continue
			}

			if allowedID != id {
				debug.Log("path %q on disallowed device %d", item, id)
				return false
			}

			return true
		}

		panic(fmt.Sprintf("item %v, device id %v not found, allowedDevs: %v", item, id, allowedDevs))
	}

	stat, err := archiver.Scan(target, selectFilter, newScanProgress(gopts))
	if err != nil {
		return err
	}

	arch := archiver.New(repo)
	arch.Excludes = opts.Excludes
	arch.SelectFilter = selectFilter

	arch.Warn = func(dir string, fi os.FileInfo, err error) {
		// TODO: make ignoring errors configurable
		Warnf("%s\rwarning for %s: %v\n", ClearLine(), dir, err)
	}

	_, id, err := arch.Snapshot(context.TODO(), newArchiveProgress(gopts, stat), target, opts.Tags, opts.Hostname, parentSnapshotID)
	if err != nil {
		return err
	}

	Verbosef("snapshot %s saved\n", id.Str())

	return nil
}
