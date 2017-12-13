package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

var cmdBackup = &cobra.Command{
	Use:   "backup [flags] FILE/DIR [FILE/DIR] ...",
	Short: "Create a new backup of files and/or directories",
	Long: `
The "backup" command creates a new snapshot and saves the files and directories
given as the arguments.
`,
	PreRun: func(cmd *cobra.Command, args []string) {
		if backupOptions.Hostname == "" {
			hostname, err := os.Hostname()
			if err != nil {
				debug.Log("os.Hostname() returned err: %v", err)
				return
			}
			backupOptions.Hostname = hostname
		}
	},
	DisableAutoGenTag: true,
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
	Parent           string
	Force            bool
	Excludes         []string
	ExcludeFiles     []string
	ExcludeOtherFS   bool
	ExcludeIfPresent []string
	ExcludeCaches    bool
	Stdin            bool
	StdinFilename    string
	Tags             []string
	Hostname         string
	FilesFrom        string
	TimeStamp        string
	WithAtime        bool
}

var backupOptions BackupOptions

func init() {
	cmdRoot.AddCommand(cmdBackup)

	f := cmdBackup.Flags()
	f.StringVar(&backupOptions.Parent, "parent", "", "use this parent snapshot (default: last snapshot in the repo that has the same target files/directories)")
	f.BoolVarP(&backupOptions.Force, "force", "f", false, `force re-reading the target files/directories (overrides the "parent" flag)`)
	f.StringArrayVarP(&backupOptions.Excludes, "exclude", "e", nil, "exclude a `pattern` (can be specified multiple times)")
	f.StringArrayVar(&backupOptions.ExcludeFiles, "exclude-file", nil, "read exclude patterns from a `file` (can be specified multiple times)")
	f.BoolVarP(&backupOptions.ExcludeOtherFS, "one-file-system", "x", false, "exclude other file systems")
	f.StringArrayVar(&backupOptions.ExcludeIfPresent, "exclude-if-present", nil, "takes filename[:header], exclude contents of directories containing filename (except filename itself) if header of that file is as provided (can be specified multiple times)")
	f.BoolVar(&backupOptions.ExcludeCaches, "exclude-caches", false, `excludes cache directories that are marked with a CACHEDIR.TAG file`)
	f.BoolVar(&backupOptions.Stdin, "stdin", false, "read backup from stdin")
	f.StringVar(&backupOptions.StdinFilename, "stdin-filename", "stdin", "file name to use when reading from stdin")
	f.StringArrayVar(&backupOptions.Tags, "tag", nil, "add a `tag` for the new snapshot (can be specified multiple times)")
	f.StringVar(&backupOptions.Hostname, "hostname", "", "set the `hostname` for the snapshot manually. To prevent an expensive rescan use the \"parent\" flag")
	f.StringVar(&backupOptions.FilesFrom, "files-from", "", "read the files to backup from file (can be combined with file args)")
	f.StringVar(&backupOptions.TimeStamp, "time", "", "time of the backup (ex. '2012-11-01 22:08:41') (default: now)")
	f.BoolVar(&backupOptions.WithAtime, "with-atime", false, "store the atime for all files and directories")
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
			Warnf("%v does not exist, skipping\n", item)
			continue
		}

		result = append(result, item)
	}

	if len(result) == 0 {
		return nil, errors.Fatal("all target directories/files do not exist")
	}

	return
}

func readBackupFromStdin(opts BackupOptions, gopts GlobalOptions, args []string) error {
	if len(args) != 0 {
		return errors.Fatal("when reading from stdin, no additional files can be specified")
	}

	fn := opts.StdinFilename

	if fn == "" {
		return errors.Fatal("filename for backup from stdin must not be empty")
	}

	if filepath.Base(fn) != fn || path.Base(fn) != fn {
		return errors.Fatal("filename is invalid (may not contain a directory, slash or backslash)")
	}

	if gopts.password == "" {
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

	err = repo.LoadIndex(gopts.ctx)
	if err != nil {
		return err
	}

	r := &archiver.Reader{
		Repository: repo,
		Tags:       opts.Tags,
		Hostname:   opts.Hostname,
	}

	_, id, err := r.Archive(gopts.ctx, fn, os.Stdin, newArchiveStdinProgress(gopts))
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
		// ignore empty lines
		if line == "" {
			continue
		}
		// strip comments
		if strings.HasPrefix(line, "#") {
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
	if opts.FilesFrom == "-" && gopts.password == "" {
		return errors.Fatal("unable to read password from stdin when data is to be read from stdin, use --password-file or $RESTIC_PASSWORD")
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
		return errors.Fatal("nothing to backup, please specify target files/dirs")
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

	// rejectFuncs collect functions that can reject items from the backup
	var rejectFuncs []RejectFunc

	// allowed devices
	if opts.ExcludeOtherFS {
		f, err := rejectByDevice(target)
		if err != nil {
			return err
		}
		rejectFuncs = append(rejectFuncs, f)
	}

	// add patterns from file
	if len(opts.ExcludeFiles) > 0 {
		opts.Excludes = append(opts.Excludes, readExcludePatternsFromFiles(opts.ExcludeFiles)...)
	}

	if len(opts.Excludes) > 0 {
		rejectFuncs = append(rejectFuncs, rejectByPattern(opts.Excludes))
	}

	if opts.ExcludeCaches {
		opts.ExcludeIfPresent = append(opts.ExcludeIfPresent, "CACHEDIR.TAG:Signature: 8a477f597d28d172789f06886806bc55")
	}

	for _, spec := range opts.ExcludeIfPresent {
		f, err := rejectIfPresent(spec)
		if err != nil {
			return err
		}

		rejectFuncs = append(rejectFuncs, f)
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

	// exclude restic cache
	if repo.Cache != nil {
		f, err := rejectResticCache(repo)
		if err != nil {
			return err
		}

		rejectFuncs = append(rejectFuncs, f)
	}

	err = repo.LoadIndex(gopts.ctx)
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
		id, err := restic.FindLatestSnapshot(gopts.ctx, repo, target, []restic.TagList{}, opts.Hostname)
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

	selectFilter := func(item string, fi os.FileInfo) bool {
		for _, reject := range rejectFuncs {
			if reject(item, fi) {
				return false
			}
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
	arch.WithAccessTime = opts.WithAtime

	arch.Warn = func(dir string, fi os.FileInfo, err error) {
		// TODO: make ignoring errors configurable
		Warnf("%s\rwarning for %s: %v\n", ClearLine(), dir, err)
	}

	timeStamp := time.Now()
	if opts.TimeStamp != "" {
		timeStamp, err = time.Parse(TimeFormat, opts.TimeStamp)
		if err != nil {
			return errors.Fatalf("error in time option: %v\n", err)
		}
	}

	_, id, err := arch.Snapshot(gopts.ctx, newArchiveProgress(gopts, stat), target, opts.Tags, opts.Hostname, parentSnapshotID, timeStamp)
	if err != nil {
		return err
	}

	Verbosef("snapshot %s saved\n", id.Str())

	return nil
}

func readExcludePatternsFromFiles(excludeFiles []string) []string {
	var excludes []string
	for _, filename := range excludeFiles {
		err := func() (err error) {
			file, err := fs.Open(filename)
			if err != nil {
				return err
			}
			defer func() {
				// return pre-close error if there was one
				if errClose := file.Close(); err == nil {
					err = errClose
				}
			}()

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
				excludes = append(excludes, line)
			}
			return scanner.Err()
		}()
		if err != nil {
			Warnf("error reading exclude patterns: %v:", err)
			return nil
		}
	}
	return excludes
}
