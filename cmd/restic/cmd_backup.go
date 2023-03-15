package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/textfile"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/backup"
	"github.com/restic/restic/internal/ui/termstatus"
)

var cmdBackup = &cobra.Command{
	Use:   "backup [flags] [FILE/DIR] ...",
	Short: "Create a new backup of files and/or directories",
	Long: `
The "backup" command creates a new snapshot and saves the files and directories
given as the arguments.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was a fatal error (no snapshot created).
Exit status is 3 if some source data could not be read (incomplete snapshot created).
`,
	PreRun: func(cmd *cobra.Command, args []string) {
		if backupOptions.Host == "" {
			hostname, err := os.Hostname()
			if err != nil {
				debug.Log("os.Hostname() returned err: %v", err)
				return
			}
			backupOptions.Host = hostname
		}
	},
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		var wg sync.WaitGroup
		cancelCtx, cancel := context.WithCancel(ctx)
		defer func() {
			// shutdown termstatus
			cancel()
			wg.Wait()
		}()

		term := termstatus.New(globalOptions.stdout, globalOptions.stderr, globalOptions.Quiet)
		wg.Add(1)
		go func() {
			defer wg.Done()
			term.Run(cancelCtx)
		}()

		// use the terminal for stdout/stderr
		prevStdout, prevStderr := globalOptions.stdout, globalOptions.stderr
		defer func() {
			globalOptions.stdout, globalOptions.stderr = prevStdout, prevStderr
		}()
		stdioWrapper := ui.NewStdioWrapper(term)
		globalOptions.stdout, globalOptions.stderr = stdioWrapper.Stdout(), stdioWrapper.Stderr()

		return runBackup(ctx, backupOptions, globalOptions, term, args)
	},
}

// BackupOptions bundles all options for the backup command.
type BackupOptions struct {
	excludePatternOptions

	Parent            string
	GroupBy           restic.SnapshotGroupByOptions
	Force             bool
	ExcludeOtherFS    bool
	ExcludeIfPresent  []string
	ExcludeCaches     bool
	ExcludeLargerThan string
	Stdin             bool
	StdinFilename     string
	Tags              restic.TagLists
	Host              string
	FilesFrom         []string
	FilesFromVerbatim []string
	FilesFromRaw      []string
	TimeStamp         string
	WithAtime         bool
	IgnoreInode       bool
	IgnoreCtime       bool
	UseFsSnapshot     bool
	DryRun            bool
	ReadConcurrency   uint
	NoScan            bool
}

var backupOptions BackupOptions

// ErrInvalidSourceData is used to report an incomplete backup
var ErrInvalidSourceData = errors.New("at least one source file could not be read")

func init() {
	cmdRoot.AddCommand(cmdBackup)

	f := cmdBackup.Flags()
	f.StringVar(&backupOptions.Parent, "parent", "", "use this parent `snapshot` (default: latest snapshot in the group determined by --group-by and not newer than the timestamp determined by --time)")
	backupOptions.GroupBy = restic.SnapshotGroupByOptions{Host: true, Path: true}
	f.VarP(&backupOptions.GroupBy, "group-by", "g", "`group` snapshots by host, paths and/or tags, separated by comma (disable grouping with '')")
	f.BoolVarP(&backupOptions.Force, "force", "f", false, `force re-reading the target files/directories (overrides the "parent" flag)`)

	initExcludePatternOptions(f, &backupOptions.excludePatternOptions)

	f.BoolVarP(&backupOptions.ExcludeOtherFS, "one-file-system", "x", false, "exclude other file systems, don't cross filesystem boundaries and subvolumes")
	f.StringArrayVar(&backupOptions.ExcludeIfPresent, "exclude-if-present", nil, "takes `filename[:header]`, exclude contents of directories containing filename (except filename itself) if header of that file is as provided (can be specified multiple times)")
	f.BoolVar(&backupOptions.ExcludeCaches, "exclude-caches", false, `excludes cache directories that are marked with a CACHEDIR.TAG file. See https://bford.info/cachedir/ for the Cache Directory Tagging Standard`)
	f.StringVar(&backupOptions.ExcludeLargerThan, "exclude-larger-than", "", "max `size` of the files to be backed up (allowed suffixes: k/K, m/M, g/G, t/T)")
	f.BoolVar(&backupOptions.Stdin, "stdin", false, "read backup from stdin")
	f.StringVar(&backupOptions.StdinFilename, "stdin-filename", "stdin", "`filename` to use when reading from stdin")
	f.Var(&backupOptions.Tags, "tag", "add `tags` for the new snapshot in the format `tag[,tag,...]` (can be specified multiple times)")
	f.UintVar(&backupOptions.ReadConcurrency, "read-concurrency", 0, "read `n` files concurrently (default: $RESTIC_READ_CONCURRENCY or 2)")
	f.StringVarP(&backupOptions.Host, "host", "H", "", "set the `hostname` for the snapshot manually. To prevent an expensive rescan use the \"parent\" flag")
	f.StringVar(&backupOptions.Host, "hostname", "", "set the `hostname` for the snapshot manually")
	err := f.MarkDeprecated("hostname", "use --host")
	if err != nil {
		// MarkDeprecated only returns an error when the flag could not be found
		panic(err)
	}
	f.StringArrayVar(&backupOptions.FilesFrom, "files-from", nil, "read the files to backup from `file` (can be combined with file args; can be specified multiple times)")
	f.StringArrayVar(&backupOptions.FilesFromVerbatim, "files-from-verbatim", nil, "read the files to backup from `file` (can be combined with file args; can be specified multiple times)")
	f.StringArrayVar(&backupOptions.FilesFromRaw, "files-from-raw", nil, "read the files to backup from `file` (can be combined with file args; can be specified multiple times)")
	f.StringVar(&backupOptions.TimeStamp, "time", "", "`time` of the backup (ex. '2012-11-01 22:08:41') (default: now)")
	f.BoolVar(&backupOptions.WithAtime, "with-atime", false, "store the atime for all files and directories")
	f.BoolVar(&backupOptions.IgnoreInode, "ignore-inode", false, "ignore inode number changes when checking for modified files")
	f.BoolVar(&backupOptions.IgnoreCtime, "ignore-ctime", false, "ignore ctime changes when checking for modified files")
	f.BoolVarP(&backupOptions.DryRun, "dry-run", "n", false, "do not upload or write any data, just show what would be done")
	f.BoolVar(&backupOptions.NoScan, "no-scan", false, "do not run scanner to estimate size of backup")
	if runtime.GOOS == "windows" {
		f.BoolVar(&backupOptions.UseFsSnapshot, "use-fs-snapshot", false, "use filesystem snapshot where possible (currently only Windows VSS)")
	}

	// parse read concurrency from env, on error the default value will be used
	readConcurrency, _ := strconv.ParseUint(os.Getenv("RESTIC_READ_CONCURRENCY"), 10, 32)
	backupOptions.ReadConcurrency = uint(readConcurrency)
}

// filterExisting returns a slice of all existing items, or an error if no
// items exist at all.
func filterExisting(items []string) (result []string, err error) {
	for _, item := range items {
		_, err := fs.Lstat(item)
		if errors.Is(err, os.ErrNotExist) {
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

// readLines reads all lines from the named file and returns them as a
// string slice.
//
// If filename is empty, readPatternsFromFile returns an empty slice.
// If filename is a dash (-), readPatternsFromFile will read the lines from the
// standard input.
func readLines(filename string) ([]string, error) {
	if filename == "" {
		return nil, nil
	}

	var (
		data []byte
		err  error
	)

	if filename == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = textfile.Read(filename)
	}

	if err != nil {
		return nil, err
	}

	var lines []string

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

// readFilenamesFromFileRaw reads a list of filenames from the given file,
// or stdin if filename is "-". Each filename is terminated by a zero byte,
// which is stripped off.
func readFilenamesFromFileRaw(filename string) (names []string, err error) {
	f := os.Stdin
	if filename != "-" {
		if f, err = os.Open(filename); err != nil {
			return nil, err
		}
	}

	names, err = readFilenamesRaw(f)
	if err != nil {
		// ignore subsequent errors
		_ = f.Close()
		return nil, err
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	return names, nil
}

func readFilenamesRaw(r io.Reader) (names []string, err error) {
	br := bufio.NewReader(r)
	for {
		name, err := br.ReadString(0)
		switch err {
		case nil:
		case io.EOF:
			if name == "" {
				return names, nil
			}
			return nil, errors.Fatal("--files-from-raw: trailing zero byte missing")
		default:
			return nil, err
		}

		name = name[:len(name)-1]
		if name == "" {
			// The empty filename is never valid. Handle this now to
			// prevent downstream code from erroneously backing up
			// filepath.Clean("") == ".".
			return nil, errors.Fatal("--files-from-raw: empty filename in listing")
		}
		names = append(names, name)
	}
}

// Check returns an error when an invalid combination of options was set.
func (opts BackupOptions) Check(gopts GlobalOptions, args []string) error {
	if gopts.password == "" {
		if opts.Stdin {
			return errors.Fatal("cannot read both password and data from stdin")
		}

		filesFrom := append(append(opts.FilesFrom, opts.FilesFromVerbatim...), opts.FilesFromRaw...)
		for _, filename := range filesFrom {
			if filename == "-" {
				return errors.Fatal("unable to read password from stdin when data is to be read from stdin, use --password-file or $RESTIC_PASSWORD")
			}
		}
	}

	if opts.Stdin {
		if len(opts.FilesFrom) > 0 {
			return errors.Fatal("--stdin and --files-from cannot be used together")
		}
		if len(opts.FilesFromVerbatim) > 0 {
			return errors.Fatal("--stdin and --files-from-verbatim cannot be used together")
		}
		if len(opts.FilesFromRaw) > 0 {
			return errors.Fatal("--stdin and --files-from-raw cannot be used together")
		}

		if len(args) > 0 {
			return errors.Fatal("--stdin was specified and files/dirs were listed as arguments")
		}
	}

	return nil
}

// collectRejectByNameFuncs returns a list of all functions which may reject data
// from being saved in a snapshot based on path only
func collectRejectByNameFuncs(opts BackupOptions, repo *repository.Repository, targets []string) (fs []RejectByNameFunc, err error) {
	// exclude restic cache
	if repo.Cache != nil {
		f, err := rejectResticCache(repo)
		if err != nil {
			return nil, err
		}

		fs = append(fs, f)
	}

	fsPatterns, err := opts.excludePatternOptions.CollectPatterns()
	if err != nil {
		return nil, err
	}
	fs = append(fs, fsPatterns...)

	if opts.ExcludeCaches {
		opts.ExcludeIfPresent = append(opts.ExcludeIfPresent, "CACHEDIR.TAG:Signature: 8a477f597d28d172789f06886806bc55")
	}

	for _, spec := range opts.ExcludeIfPresent {
		f, err := rejectIfPresent(spec)
		if err != nil {
			return nil, err
		}

		fs = append(fs, f)
	}

	return fs, nil
}

// collectRejectFuncs returns a list of all functions which may reject data
// from being saved in a snapshot based on path and file info
func collectRejectFuncs(opts BackupOptions, repo *repository.Repository, targets []string) (fs []RejectFunc, err error) {
	// allowed devices
	if opts.ExcludeOtherFS && !opts.Stdin {
		f, err := rejectByDevice(targets)
		if err != nil {
			return nil, err
		}
		fs = append(fs, f)
	}

	if len(opts.ExcludeLargerThan) != 0 && !opts.Stdin {
		f, err := rejectBySize(opts.ExcludeLargerThan)
		if err != nil {
			return nil, err
		}
		fs = append(fs, f)
	}

	return fs, nil
}

// collectTargets returns a list of target files/dirs from several sources.
func collectTargets(opts BackupOptions, args []string) (targets []string, err error) {
	if opts.Stdin {
		return nil, nil
	}

	for _, file := range opts.FilesFrom {
		fromfile, err := readLines(file)
		if err != nil {
			return nil, err
		}

		// expand wildcards
		for _, line := range fromfile {
			line = strings.TrimSpace(line)
			if line == "" || line[0] == '#' { // '#' marks a comment.
				continue
			}

			var expanded []string
			expanded, err := filepath.Glob(line)
			if err != nil {
				return nil, fmt.Errorf("pattern: %s: %w", line, err)
			}
			if len(expanded) == 0 {
				Warnf("pattern %q does not match any files, skipping\n", line)
			}
			targets = append(targets, expanded...)
		}
	}

	for _, file := range opts.FilesFromVerbatim {
		fromfile, err := readLines(file)
		if err != nil {
			return nil, err
		}
		for _, line := range fromfile {
			if line == "" {
				continue
			}
			targets = append(targets, line)
		}
	}

	for _, file := range opts.FilesFromRaw {
		fromfile, err := readFilenamesFromFileRaw(file)
		if err != nil {
			return nil, err
		}
		targets = append(targets, fromfile...)
	}

	// Merge args into files-from so we can reuse the normal args checks
	// and have the ability to use both files-from and args at the same time.
	targets = append(targets, args...)
	if len(targets) == 0 && !opts.Stdin {
		return nil, errors.Fatal("nothing to backup, please specify target files/dirs")
	}

	targets, err = filterExisting(targets)
	if err != nil {
		return nil, err
	}

	return targets, nil
}

// parent returns the ID of the parent snapshot. If there is none, nil is
// returned.
func findParentSnapshot(ctx context.Context, repo restic.Repository, opts BackupOptions, targets []string, timeStampLimit time.Time) (*restic.Snapshot, error) {
	if opts.Force {
		return nil, nil
	}

	snName := opts.Parent
	if snName == "" {
		snName = "latest"
	}
	f := restic.SnapshotFilter{TimestampLimit: timeStampLimit}
	if opts.GroupBy.Host {
		f.Hosts = []string{opts.Host}
	}
	if opts.GroupBy.Path {
		f.Paths = targets
	}
	if opts.GroupBy.Tag {
		f.Tags = []restic.TagList{opts.Tags.Flatten()}
	}

	sn, err := f.FindLatest(ctx, repo.Backend(), repo, snName)
	// Snapshot not found is ok if no explicit parent was set
	if opts.Parent == "" && errors.Is(err, restic.ErrNoSnapshotFound) {
		err = nil
	}
	return sn, err
}

func runBackup(ctx context.Context, opts BackupOptions, gopts GlobalOptions, term *termstatus.Terminal, args []string) error {
	err := opts.Check(gopts, args)
	if err != nil {
		return err
	}

	targets, err := collectTargets(opts, args)
	if err != nil {
		return err
	}

	timeStamp := time.Now()
	if opts.TimeStamp != "" {
		timeStamp, err = time.ParseInLocation(TimeFormat, opts.TimeStamp, time.Local)
		if err != nil {
			return errors.Fatalf("error in time option: %v\n", err)
		}
	}

	if gopts.verbosity >= 2 && !gopts.JSON {
		Verbosef("open repository\n")
	}

	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	var progressPrinter backup.ProgressPrinter
	if gopts.JSON {
		progressPrinter = backup.NewJSONProgress(term, gopts.verbosity)
	} else {
		progressPrinter = backup.NewTextProgress(term, gopts.verbosity)
	}
	progressReporter := backup.NewProgress(progressPrinter,
		calculateProgressInterval(!gopts.Quiet, gopts.JSON))
	defer progressReporter.Done()

	if opts.DryRun {
		repo.SetDryRun()
	}

	if !gopts.JSON {
		progressPrinter.V("lock repository")
	}
	lock, ctx, err := lockRepo(ctx, repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	// rejectByNameFuncs collect functions that can reject items from the backup based on path only
	rejectByNameFuncs, err := collectRejectByNameFuncs(opts, repo, targets)
	if err != nil {
		return err
	}

	// rejectFuncs collect functions that can reject items from the backup based on path and file info
	rejectFuncs, err := collectRejectFuncs(opts, repo, targets)
	if err != nil {
		return err
	}

	var parentSnapshot *restic.Snapshot
	if !opts.Stdin {
		parentSnapshot, err = findParentSnapshot(ctx, repo, opts, targets, timeStamp)
		if err != nil {
			return err
		}

		if !gopts.JSON {
			if parentSnapshot != nil {
				progressPrinter.P("using parent snapshot %v\n", parentSnapshot.ID().Str())
			} else {
				progressPrinter.P("no parent snapshot found, will read all files\n")
			}
		}
	}

	if !gopts.JSON {
		progressPrinter.V("load index files")
	}
	err = repo.LoadIndex(ctx)
	if err != nil {
		return err
	}

	selectByNameFilter := func(item string) bool {
		for _, reject := range rejectByNameFuncs {
			if reject(item) {
				return false
			}
		}
		return true
	}

	selectFilter := func(item string, fi os.FileInfo) bool {
		for _, reject := range rejectFuncs {
			if reject(item, fi) {
				return false
			}
		}
		return true
	}

	var targetFS fs.FS = fs.Local{}
	if runtime.GOOS == "windows" && opts.UseFsSnapshot {
		if err = fs.HasSufficientPrivilegesForVSS(); err != nil {
			return err
		}

		errorHandler := func(item string, err error) error {
			return progressReporter.Error(item, err)
		}

		messageHandler := func(msg string, args ...interface{}) {
			if !gopts.JSON {
				progressPrinter.P(msg, args...)
			}
		}

		localVss := fs.NewLocalVss(errorHandler, messageHandler)
		defer localVss.DeleteSnapshots()
		targetFS = localVss
	}
	if opts.Stdin {
		if !gopts.JSON {
			progressPrinter.V("read data from stdin")
		}
		filename := path.Join("/", opts.StdinFilename)
		targetFS = &fs.Reader{
			ModTime:    timeStamp,
			Name:       filename,
			Mode:       0644,
			ReadCloser: os.Stdin,
		}
		targets = []string{filename}
	}

	wg, wgCtx := errgroup.WithContext(ctx)
	cancelCtx, cancel := context.WithCancel(wgCtx)
	defer cancel()

	if !opts.NoScan {
		sc := archiver.NewScanner(targetFS)
		sc.SelectByName = selectByNameFilter
		sc.Select = selectFilter
		sc.Error = progressPrinter.ScannerError
		sc.Result = progressReporter.ReportTotal

		if !gopts.JSON {
			progressPrinter.V("start scan on %v", targets)
		}
		wg.Go(func() error { return sc.Scan(cancelCtx, targets) })
	}

	arch := archiver.New(repo, targetFS, archiver.Options{ReadConcurrency: backupOptions.ReadConcurrency})
	arch.SelectByName = selectByNameFilter
	arch.Select = selectFilter
	arch.WithAtime = opts.WithAtime
	success := true
	arch.Error = func(item string, err error) error {
		success = false
		return progressReporter.Error(item, err)
	}
	arch.CompleteItem = progressReporter.CompleteItem
	arch.StartFile = progressReporter.StartFile
	arch.CompleteBlob = progressReporter.CompleteBlob

	if opts.IgnoreInode {
		// --ignore-inode implies --ignore-ctime: on FUSE, the ctime is not
		// reliable either.
		arch.ChangeIgnoreFlags |= archiver.ChangeIgnoreCtime | archiver.ChangeIgnoreInode
	}
	if opts.IgnoreCtime {
		arch.ChangeIgnoreFlags |= archiver.ChangeIgnoreCtime
	}

	snapshotOpts := archiver.SnapshotOptions{
		Excludes:       opts.Excludes,
		Tags:           opts.Tags.Flatten(),
		Time:           timeStamp,
		Hostname:       opts.Host,
		ParentSnapshot: parentSnapshot,
	}

	if !gopts.JSON {
		progressPrinter.V("start backup on %v", targets)
	}
	_, id, err := arch.Snapshot(ctx, targets, snapshotOpts)

	// cleanly shutdown all running goroutines
	cancel()

	// let's see if one returned an error
	werr := wg.Wait()

	// return original error
	if err != nil {
		return errors.Fatalf("unable to save snapshot: %v", err)
	}

	// Report finished execution
	progressReporter.Finish(id, opts.DryRun)
	if !gopts.JSON && !opts.DryRun {
		progressPrinter.P("snapshot %s saved\n", id.Str())
	}
	if !success {
		return ErrInvalidSourceData
	}

	// Return error if any
	return werr
}
