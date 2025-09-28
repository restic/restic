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
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/textfile"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/backup"
)

func newBackupCommand(globalOptions *global.Options) *cobra.Command {
	var opts BackupOptions

	cmd := &cobra.Command{
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
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
`,
		PreRun: func(_ *cobra.Command, _ []string) {
			if opts.Host == "" {
				hostname, err := os.Hostname()
				if err != nil {
					debug.Log("os.Hostname() returned err: %v", err)
					return
				}
				opts.Host = hostname
			}
		},
		GroupID:           cmdGroupDefault,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackup(cmd.Context(), opts, *globalOptions, globalOptions.Term, args)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// BackupOptions bundles all options for the backup command.
type BackupOptions struct {
	filter.ExcludePatternOptions

	Parent            string
	GroupBy           data.SnapshotGroupByOptions
	Force             bool
	ExcludeOtherFS    bool
	ExcludeIfPresent  []string
	ExcludeCaches     bool
	ExcludeLargerThan string
	ExcludeCloudFiles bool
	Stdin             bool
	StdinFilename     string
	StdinCommand      bool
	Tags              data.TagLists
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
	SkipIfUnchanged   bool
}

func (opts *BackupOptions) AddFlags(f *pflag.FlagSet) {
	f.StringVar(&opts.Parent, "parent", "", "use this parent `snapshot` (default: latest snapshot in the group determined by --group-by and not newer than the timestamp determined by --time)")
	opts.GroupBy = data.SnapshotGroupByOptions{Host: true, Path: true}
	f.VarP(&opts.GroupBy, "group-by", "g", "`group` snapshots by host, paths and/or tags, separated by comma (disable grouping with '')")
	f.BoolVarP(&opts.Force, "force", "f", false, `force re-reading the source files/directories (overrides the "parent" flag)`)

	opts.ExcludePatternOptions.Add(f)

	f.BoolVarP(&opts.ExcludeOtherFS, "one-file-system", "x", false, "exclude other file systems, don't cross filesystem boundaries and subvolumes")
	f.StringArrayVar(&opts.ExcludeIfPresent, "exclude-if-present", nil, "takes `filename[:header]`, exclude contents of directories containing filename (except filename itself) if header of that file is as provided (can be specified multiple times)")
	f.BoolVar(&opts.ExcludeCaches, "exclude-caches", false, `excludes cache directories that are marked with a CACHEDIR.TAG file. See https://bford.info/cachedir/ for the Cache Directory Tagging Standard`)
	f.StringVar(&opts.ExcludeLargerThan, "exclude-larger-than", "", "max `size` of the files to be backed up (allowed suffixes: k/K, m/M, g/G, t/T)")
	f.BoolVar(&opts.Stdin, "stdin", false, "read backup from stdin")
	f.StringVar(&opts.StdinFilename, "stdin-filename", "stdin", "`filename` to use when reading from stdin")
	f.BoolVar(&opts.StdinCommand, "stdin-from-command", false, "interpret arguments as command to execute and store its stdout")
	f.Var(&opts.Tags, "tag", "add `tags` for the new snapshot in the format `tag[,tag,...]` (can be specified multiple times)")
	f.UintVar(&opts.ReadConcurrency, "read-concurrency", 0, "read `n` files concurrently (default: $RESTIC_READ_CONCURRENCY or 2)")
	f.StringVarP(&opts.Host, "host", "H", "", "set the `hostname` for the snapshot manually (default: $RESTIC_HOST). To prevent an expensive rescan use the \"parent\" flag")
	f.StringVar(&opts.Host, "hostname", "", "set the `hostname` for the snapshot manually")
	err := f.MarkDeprecated("hostname", "use --host")
	if err != nil {
		// MarkDeprecated only returns an error when the flag could not be found
		panic(err)
	}
	f.StringArrayVar(&opts.FilesFrom, "files-from", nil, "read the files to backup from `file` (can be combined with file args; can be specified multiple times)")
	f.StringArrayVar(&opts.FilesFromVerbatim, "files-from-verbatim", nil, "read the files to backup from `file` (can be combined with file args; can be specified multiple times)")
	f.StringArrayVar(&opts.FilesFromRaw, "files-from-raw", nil, "read the files to backup from `file` (can be combined with file args; can be specified multiple times)")
	f.StringVar(&opts.TimeStamp, "time", "", "`time` of the backup (ex. '2012-11-01 22:08:41') (default: now)")
	f.BoolVar(&opts.WithAtime, "with-atime", false, "store the atime for all files and directories")
	f.BoolVar(&opts.IgnoreInode, "ignore-inode", false, "ignore inode number and ctime changes when checking for modified files")
	f.BoolVar(&opts.IgnoreCtime, "ignore-ctime", false, "ignore ctime changes when checking for modified files")
	f.BoolVarP(&opts.DryRun, "dry-run", "n", false, "do not upload or write any data, just show what would be done")
	f.BoolVar(&opts.NoScan, "no-scan", false, "do not run scanner to estimate size of backup")
	if runtime.GOOS == "windows" {
		f.BoolVar(&opts.UseFsSnapshot, "use-fs-snapshot", false, "use filesystem snapshot where possible (currently only Windows VSS)")
		f.BoolVar(&opts.ExcludeCloudFiles, "exclude-cloud-files", false, "excludes online-only cloud files (such as OneDrive Files On-Demand)")
	}
	f.BoolVar(&opts.SkipIfUnchanged, "skip-if-unchanged", false, "skip snapshot creation if identical to parent snapshot")

	// parse read concurrency from env, on error the default value will be used
	readConcurrency, _ := strconv.ParseUint(os.Getenv("RESTIC_READ_CONCURRENCY"), 10, 32)
	opts.ReadConcurrency = uint(readConcurrency)

	// parse host from env, if not exists or empty the default value will be used
	if host := os.Getenv("RESTIC_HOST"); host != "" {
		opts.Host = host
	}
}

var backupFSTestHook func(fs fs.FS) fs.FS

// ErrInvalidSourceData is used to report an incomplete backup
var ErrInvalidSourceData = errors.New("at least one source file could not be read")

// ErrNoSourceData is used to report that no source data was found
var ErrNoSourceData = errors.Fatal("all source directories/files do not exist")

// filterExisting returns a slice of all existing items, or an error if no
// items exist at all.
func filterExisting(items []string, warnf func(msg string, args ...interface{})) (result []string, err error) {
	for _, item := range items {
		_, err := fs.Lstat(item)
		if errors.Is(err, os.ErrNotExist) {
			warnf("%v does not exist, skipping\n", item)
			continue
		}

		result = append(result, item)
	}

	if len(result) == 0 {
		return nil, ErrNoSourceData
	} else if len(result) < len(items) {
		return result, ErrInvalidSourceData
	}

	return result, nil
}

// readLines reads all lines from the named file and returns them as a
// string slice.
//
// If filename is empty, readPatternsFromFile returns an empty slice.
// If filename is a dash (-), readPatternsFromFile will read the lines from the
// standard input.
func readLines(filename string, stdin io.ReadCloser) ([]string, error) {
	if filename == "" {
		return nil, nil
	}

	var (
		data []byte
		err  error
	)

	if filename == "-" {
		data, err = io.ReadAll(stdin)
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
func readFilenamesFromFileRaw(filename string, stdin io.ReadCloser) (names []string, err error) {
	f := stdin
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
func (opts BackupOptions) Check(gopts global.Options, args []string) error {
	if gopts.Password == "" && !gopts.InsecureNoPassword {
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

	if opts.Stdin || opts.StdinCommand {
		if len(opts.FilesFrom) > 0 {
			return errors.Fatal("--stdin and --files-from cannot be used together")
		}
		if len(opts.FilesFromVerbatim) > 0 {
			return errors.Fatal("--stdin and --files-from-verbatim cannot be used together")
		}
		if len(opts.FilesFromRaw) > 0 {
			return errors.Fatal("--stdin and --files-from-raw cannot be used together")
		}

		if len(args) > 0 && !opts.StdinCommand {
			return errors.Fatal("--stdin was specified and files/dirs were listed as arguments")
		}
	}

	return nil
}

// collectRejectByNameFuncs returns a list of all functions which may reject data
// from being saved in a snapshot based on path only
func collectRejectByNameFuncs(opts BackupOptions, repo *repository.Repository, warnf func(msg string, args ...interface{})) (fs []archiver.RejectByNameFunc, err error) {
	// exclude restic cache
	if repo.Cache() != nil {
		f, err := rejectResticCache(repo)
		if err != nil {
			return nil, err
		}

		fs = append(fs, f)
	}

	fsPatterns, err := opts.ExcludePatternOptions.CollectPatterns(warnf)
	if err != nil {
		return nil, err
	}
	for _, pat := range fsPatterns {
		fs = append(fs, archiver.RejectByNameFunc(pat))
	}

	return fs, nil
}

// collectRejectFuncs returns a list of all functions which may reject data
// from being saved in a snapshot based on path and file info
func collectRejectFuncs(opts BackupOptions, targets []string, fs fs.FS, warnf func(msg string, args ...interface{})) (funcs []archiver.RejectFunc, err error) {
	// allowed devices
	if opts.ExcludeOtherFS && !opts.Stdin && !opts.StdinCommand {
		f, err := archiver.RejectByDevice(targets, fs)
		if err != nil {
			return nil, err
		}
		funcs = append(funcs, f)
	}

	if len(opts.ExcludeLargerThan) != 0 && !opts.Stdin && !opts.StdinCommand {
		maxSize, err := ui.ParseBytes(opts.ExcludeLargerThan)
		if err != nil {
			return nil, err
		}

		f, err := archiver.RejectBySize(maxSize)
		if err != nil {
			return nil, err
		}
		funcs = append(funcs, f)
	}

	if opts.ExcludeCloudFiles && !opts.Stdin && !opts.StdinCommand {
		if runtime.GOOS != "windows" {
			return nil, errors.Fatalf("exclude-cloud-files is only supported on Windows")
		}
		f, err := archiver.RejectCloudFiles(warnf)
		if err != nil {
			return nil, err
		}
		funcs = append(funcs, f)
	}

	if opts.ExcludeCaches {
		opts.ExcludeIfPresent = append(opts.ExcludeIfPresent, "CACHEDIR.TAG:Signature: 8a477f597d28d172789f06886806bc55")
	}

	for _, spec := range opts.ExcludeIfPresent {
		f, err := archiver.RejectIfPresent(spec, warnf)
		if err != nil {
			return nil, err
		}

		funcs = append(funcs, f)
	}

	return funcs, nil
}

// collectTargets returns a list of target files/dirs from several sources.
func collectTargets(opts BackupOptions, args []string, warnf func(msg string, args ...interface{}), stdin io.ReadCloser) (targets []string, err error) {
	if opts.Stdin || opts.StdinCommand {
		return nil, nil
	}

	for _, file := range opts.FilesFrom {
		fromfile, err := readLines(file, stdin)
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
				warnf("pattern %q does not match any files, skipping\n", line)
			}
			targets = append(targets, expanded...)
		}
	}

	for _, file := range opts.FilesFromVerbatim {
		fromfile, err := readLines(file, stdin)
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
		fromfile, err := readFilenamesFromFileRaw(file, stdin)
		if err != nil {
			return nil, err
		}
		targets = append(targets, fromfile...)
	}

	// Merge args into files-from so we can reuse the normal args checks
	// and have the ability to use both files-from and args at the same time.
	targets = append(targets, args...)
	if len(targets) == 0 && !opts.Stdin {
		return nil, errors.Fatal("nothing to backup, please specify source files/dirs")
	}

	return filterExisting(targets, warnf)
}

// parent returns the ID of the parent snapshot. If there is none, nil is
// returned.
func findParentSnapshot(ctx context.Context, repo restic.ListerLoaderUnpacked, opts BackupOptions, targets []string, timeStampLimit time.Time) (*data.Snapshot, error) {
	if opts.Force {
		return nil, nil
	}

	snName := opts.Parent
	if snName == "" {
		snName = "latest"
	}
	f := data.SnapshotFilter{TimestampLimit: timeStampLimit}
	if opts.GroupBy.Host {
		f.Hosts = []string{opts.Host}
	}
	if opts.GroupBy.Path {
		f.Paths = targets
	}
	if opts.GroupBy.Tag {
		f.Tags = []data.TagList{opts.Tags.Flatten()}
	}

	sn, _, err := f.FindLatest(ctx, repo, repo, snName)
	// Snapshot not found is ok if no explicit parent was set
	if opts.Parent == "" && errors.Is(err, data.ErrNoSnapshotFound) {
		err = nil
	}
	return sn, err
}

func runBackup(ctx context.Context, opts BackupOptions, gopts global.Options, term ui.Terminal, args []string) error {
	var vsscfg fs.VSSConfig
	var err error

	var printer backup.ProgressPrinter
	if gopts.JSON {
		printer = backup.NewJSONProgress(term, gopts.Verbosity)
	} else {
		printer = backup.NewTextProgress(term, gopts.Verbosity)
	}
	if runtime.GOOS == "windows" {
		if vsscfg, err = fs.ParseVSSConfig(gopts.Extended); err != nil {
			return err
		}
	}

	err = opts.Check(gopts, args)
	if err != nil {
		return err
	}

	success := true
	targets, err := collectTargets(opts, args, printer.E, term.InputRaw())
	if err != nil {
		if errors.Is(err, ErrInvalidSourceData) {
			success = false
		} else {
			return err
		}
	}

	timeStamp := time.Now()
	backupStart := timeStamp
	if opts.TimeStamp != "" {
		timeStamp, err = time.ParseInLocation(global.TimeFormat, opts.TimeStamp, time.Local)
		if err != nil {
			return errors.Fatalf("error in time option: %v", err)
		}
	}

	if gopts.Verbosity >= 2 && !gopts.JSON {
		printer.P("open repository")
	}

	ctx, repo, unlock, err := openWithAppendLock(ctx, gopts, opts.DryRun, printer)
	if err != nil {
		return err
	}
	defer unlock()

	progressReporter := backup.NewProgress(printer,
		ui.CalculateProgressInterval(!gopts.Quiet, gopts.JSON, term.CanUpdateStatus()))
	defer progressReporter.Done()

	// rejectByNameFuncs collect functions that can reject items from the backup based on path only
	rejectByNameFuncs, err := collectRejectByNameFuncs(opts, repo, printer.E)
	if err != nil {
		return err
	}

	var parentSnapshot *data.Snapshot
	if !opts.Stdin {
		parentSnapshot, err = findParentSnapshot(ctx, repo, opts, targets, timeStamp)
		if err != nil {
			return err
		}

		if !gopts.JSON {
			if parentSnapshot != nil {
				printer.P("using parent snapshot %v\n", parentSnapshot.ID().Str())
			} else {
				printer.P("no parent snapshot found, will read all files\n")
			}
		}
	}

	if !gopts.JSON {
		printer.V("load index files")
	}

	err = repo.LoadIndex(ctx, printer)
	if err != nil {
		return err
	}

	var targetFS fs.FS = fs.Local{}
	if runtime.GOOS == "windows" && opts.UseFsSnapshot {
		if err = fs.HasSufficientPrivilegesForVSS(); err != nil {
			return err
		}

		errorHandler := func(item string, err error) {
			_ = progressReporter.Error(item, err)
		}

		messageHandler := func(msg string, args ...interface{}) {
			if !gopts.JSON {
				printer.P(msg, args...)
			}
		}

		localVss := fs.NewLocalVss(errorHandler, messageHandler, vsscfg)
		defer localVss.DeleteSnapshots()
		targetFS = localVss
	}

	if opts.Stdin || opts.StdinCommand {
		if !gopts.JSON {
			printer.V("read data from stdin")
		}
		filename := path.Join("/", opts.StdinFilename)
		source := term.InputRaw()
		if opts.StdinCommand {
			source, err = fs.NewCommandReader(ctx, args, printer.E)
			if err != nil {
				return err
			}
		}
		targetFS, err = fs.NewReader(filename, source, fs.ReaderOptions{
			ModTime: timeStamp,
			Mode:    0644,
		})
		if err != nil {
			return fmt.Errorf("failed to backup from stdin: %w", err)
		}
		targets = []string{filename}
	}

	if backupFSTestHook != nil {
		targetFS = backupFSTestHook(targetFS)
	}

	// rejectFuncs collect functions that can reject items from the backup based on path and file info
	rejectFuncs, err := collectRejectFuncs(opts, targets, targetFS, printer.E)
	if err != nil {
		return err
	}

	selectByNameFilter := archiver.CombineRejectByNames(rejectByNameFuncs)
	selectFilter := archiver.CombineRejects(rejectFuncs)

	wg, wgCtx := errgroup.WithContext(ctx)
	cancelCtx, cancel := context.WithCancel(wgCtx)
	defer cancel()

	if !opts.NoScan {
		sc := archiver.NewScanner(targetFS)
		sc.SelectByName = selectByNameFilter
		sc.Select = selectFilter
		sc.Error = printer.ScannerError
		sc.Result = progressReporter.ReportTotal

		if !gopts.JSON {
			printer.V("start scan on %v", targets)
		}
		wg.Go(func() error { return sc.Scan(cancelCtx, targets) })
	}

	arch := archiver.New(repo, targetFS, archiver.Options{ReadConcurrency: opts.ReadConcurrency})
	arch.SelectByName = selectByNameFilter
	arch.Select = selectFilter
	arch.WithAtime = opts.WithAtime

	arch.Error = func(item string, err error) error {
		success = false
		reterr := progressReporter.Error(item, err)
		// If we receive a fatal error during the execution of the snapshot,
		// we abort the snapshot.
		if reterr == nil && errors.IsFatal(err) {
			reterr = err
		}
		return reterr
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
		Excludes:        opts.Excludes,
		Tags:            opts.Tags.Flatten(),
		BackupStart:     backupStart,
		Time:            timeStamp,
		Hostname:        opts.Host,
		ParentSnapshot:  parentSnapshot,
		ProgramVersion:  "restic " + global.Version,
		SkipIfUnchanged: opts.SkipIfUnchanged,
	}

	if !gopts.JSON {
		printer.V("start backup on %v", targets)
	}
	_, id, summary, err := arch.Snapshot(ctx, targets, snapshotOpts)

	// cleanly shutdown all running goroutines
	cancel()

	// let's see if one returned an error
	werr := wg.Wait()

	// return original error
	if err != nil {
		return errors.Fatalf("unable to save snapshot: %v", err)
	}

	// Report finished execution
	progressReporter.Finish(id, summary, opts.DryRun)
	if !success {
		return ErrInvalidSourceData
	}

	// Return error if any
	return werr
}
