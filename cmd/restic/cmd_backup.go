package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/textfile"
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
		var wg sync.WaitGroup
		cancelCtx, cancel := context.WithCancel(globalOptions.ctx)
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

		return runBackup(backupOptions, globalOptions, term, args)
	},
}

// BackupOptions bundles all options for the backup command.
type BackupOptions struct {
	Parent                  string
	Force                   bool
	Excludes                []string
	InsensitiveExcludes     []string
	ExcludeFiles            []string
	InsensitiveExcludeFiles []string
	ExcludeOtherFS          bool
	ExcludeIfPresent        []string
	ExcludeCaches           bool
	ExcludeLargerThan       string
	Stdin                   bool
	StdinFilename           string
	Tags                    restic.TagLists
	Host                    string
	FilesFrom               []string
	FilesFromVerbatim       []string
	FilesFromRaw            []string
	TimeStamp               string
	WithAtime               bool
	IgnoreInode             bool
	IgnoreCtime             bool
	UseFsSnapshot           bool
	DryRun                  bool
}

var backupOptions BackupOptions

// ErrInvalidSourceData is used to report an incomplete backup
var ErrInvalidSourceData = errors.New("at least one source file could not be read")

func init() {
	cmdRoot.AddCommand(cmdBackup)

	f := cmdBackup.Flags()
	f.StringVar(&backupOptions.Parent, "parent", "", "use this parent `snapshot` (default: last snapshot in the repository that has the same target files/directories, and is not newer than the snapshot time)")
	f.BoolVarP(&backupOptions.Force, "force", "f", false, `force re-reading the target files/directories (overrides the "parent" flag)`)
	f.StringArrayVarP(&backupOptions.Excludes, "exclude", "e", nil, "exclude a `pattern` (can be specified multiple times)")
	f.StringArrayVar(&backupOptions.InsensitiveExcludes, "iexclude", nil, "same as --exclude `pattern` but ignores the casing of filenames")
	f.StringArrayVar(&backupOptions.ExcludeFiles, "exclude-file", nil, "read exclude patterns from a `file` (can be specified multiple times)")
	f.StringArrayVar(&backupOptions.InsensitiveExcludeFiles, "iexclude-file", nil, "same as --exclude-file but ignores casing of `file`names in patterns")
	f.BoolVarP(&backupOptions.ExcludeOtherFS, "one-file-system", "x", false, "exclude other file systems, don't cross filesystem boundaries and subvolumes")
	f.StringArrayVar(&backupOptions.ExcludeIfPresent, "exclude-if-present", nil, "takes `filename[:header]`, exclude contents of directories containing filename (except filename itself) if header of that file is as provided (can be specified multiple times)")
	f.BoolVar(&backupOptions.ExcludeCaches, "exclude-caches", false, `excludes cache directories that are marked with a CACHEDIR.TAG file. See https://bford.info/cachedir/ for the Cache Directory Tagging Standard`)
	f.StringVar(&backupOptions.ExcludeLargerThan, "exclude-larger-than", "", "max `size` of the files to be backed up (allowed suffixes: k/K, m/M, g/G, t/T)")
	f.BoolVar(&backupOptions.Stdin, "stdin", false, "read backup from stdin")
	f.StringVar(&backupOptions.StdinFilename, "stdin-filename", "stdin", "`filename` to use when reading from stdin")
	f.Var(&backupOptions.Tags, "tag", "add `tags` for the new snapshot in the format `tag[,tag,...]` (can be specified multiple times)")

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
	if runtime.GOOS == "windows" {
		f.BoolVar(&backupOptions.UseFsSnapshot, "use-fs-snapshot", false, "use filesystem snapshot where possible (currently only Windows VSS)")
	}
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
		data, err = ioutil.ReadAll(os.Stdin)
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

	// add patterns from file
	if len(opts.ExcludeFiles) > 0 {
		excludes, err := readExcludePatternsFromFiles(opts.ExcludeFiles)
		if err != nil {
			return nil, err
		}

		if err := filter.ValidatePatterns(excludes); err != nil {
			return nil, errors.Fatalf("--exclude-file: %s", err)
		}

		opts.Excludes = append(opts.Excludes, excludes...)
	}

	if len(opts.InsensitiveExcludeFiles) > 0 {
		excludes, err := readExcludePatternsFromFiles(opts.InsensitiveExcludeFiles)
		if err != nil {
			return nil, err
		}

		if err := filter.ValidatePatterns(excludes); err != nil {
			return nil, errors.Fatalf("--iexclude-file: %s", err)
		}

		opts.InsensitiveExcludes = append(opts.InsensitiveExcludes, excludes...)
	}

	if len(opts.InsensitiveExcludes) > 0 {
		if err := filter.ValidatePatterns(opts.InsensitiveExcludes); err != nil {
			return nil, errors.Fatalf("--iexclude: %s", err)
		}

		fs = append(fs, rejectByInsensitivePattern(opts.InsensitiveExcludes))
	}

	if len(opts.Excludes) > 0 {
		if err := filter.ValidatePatterns(opts.Excludes); err != nil {
			return nil, errors.Fatalf("--exclude: %s", err)
		}

		fs = append(fs, rejectByPattern(opts.Excludes))
	}

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

// readExcludePatternsFromFiles reads all exclude files and returns the list of
// exclude patterns. For each line, leading and trailing white space is removed
// and comment lines are ignored. For each remaining pattern, environment
// variables are resolved. For adding a literal dollar sign ($), write $$ to
// the file.
func readExcludePatternsFromFiles(excludeFiles []string) ([]string, error) {
	getenvOrDollar := func(s string) string {
		if s == "$" {
			return "$"
		}
		return os.Getenv(s)
	}

	var excludes []string
	for _, filename := range excludeFiles {
		err := func() (err error) {
			data, err := textfile.Read(filename)
			if err != nil {
				return err
			}

			scanner := bufio.NewScanner(bytes.NewReader(data))
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

				line = os.Expand(line, getenvOrDollar)
				excludes = append(excludes, line)
			}
			return scanner.Err()
		}()
		if err != nil {
			return nil, err
		}
	}
	return excludes, nil
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
				return nil, errors.WithMessage(err, fmt.Sprintf("pattern: %s", line))
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
func findParentSnapshot(ctx context.Context, repo restic.Repository, opts BackupOptions, targets []string, timeStampLimit time.Time) (parentID *restic.ID, err error) {
	// Force using a parent
	if !opts.Force && opts.Parent != "" {
		id, err := restic.FindSnapshot(ctx, repo.Backend(), opts.Parent)
		if err != nil {
			return nil, errors.Fatalf("invalid id %q: %v", opts.Parent, err)
		}

		parentID = &id
	}

	// Find last snapshot to set it as parent, if not already set
	if !opts.Force && parentID == nil {
		id, err := restic.FindLatestSnapshot(ctx, repo.Backend(), repo, targets, []restic.TagList{}, []string{opts.Host}, &timeStampLimit)
		if err == nil {
			parentID = &id
		} else if err != restic.ErrNoSnapshotFound {
			return nil, err
		}
	}

	return parentID, nil
}

func runBackup(opts BackupOptions, gopts GlobalOptions, term *termstatus.Terminal, args []string) error {
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

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	var progressPrinter backup.ProgressPrinter
	if gopts.JSON {
		progressPrinter = backup.NewJSONProgress(term, gopts.verbosity)
	} else {
		progressPrinter = backup.NewTextProgress(term, gopts.verbosity)
	}
	progressReporter := backup.NewProgress(progressPrinter)

	if opts.DryRun {
		repo.SetDryRun()
		progressReporter.SetDryRun()
	}

	// use the terminal for stdout/stderr
	prevStdout, prevStderr := gopts.stdout, gopts.stderr
	defer func() {
		gopts.stdout, gopts.stderr = prevStdout, prevStderr
	}()
	gopts.stdout, gopts.stderr = progressPrinter.Stdout(), progressPrinter.Stderr()

	progressReporter.SetMinUpdatePause(calculateProgressInterval(!gopts.Quiet, gopts.JSON))

	wg, wgCtx := errgroup.WithContext(gopts.ctx)
	cancelCtx, cancel := context.WithCancel(wgCtx)
	defer cancel()
	wg.Go(func() error { return progressReporter.Run(cancelCtx) })

	if !gopts.JSON {
		progressPrinter.V("lock repository")
	}
	lock, err := lockRepo(gopts.ctx, repo)
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

	var parentSnapshotID *restic.ID
	if !opts.Stdin {
		parentSnapshotID, err = findParentSnapshot(gopts.ctx, repo, opts, targets, timeStamp)
		if err != nil {
			return err
		}

		if !gopts.JSON {
			if parentSnapshotID != nil {
				progressPrinter.P("using parent snapshot %v\n", parentSnapshotID.Str())
			} else {
				progressPrinter.P("no parent snapshot found, will read all files\n")
			}
		}
	}

	if !gopts.JSON {
		progressPrinter.V("load index files")
	}
	err = repo.LoadIndex(gopts.ctx)
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

	sc := archiver.NewScanner(targetFS)
	sc.SelectByName = selectByNameFilter
	sc.Select = selectFilter
	sc.Error = progressReporter.ScannerError
	sc.Result = progressReporter.ReportTotal

	if !gopts.JSON {
		progressPrinter.V("start scan on %v", targets)
	}
	wg.Go(func() error { return sc.Scan(cancelCtx, targets) })

	arch := archiver.New(repo, targetFS, archiver.Options{})
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

	if parentSnapshotID == nil {
		parentSnapshotID = &restic.ID{}
	}

	snapshotOpts := archiver.SnapshotOptions{
		Excludes:       opts.Excludes,
		Tags:           opts.Tags.Flatten(),
		Time:           timeStamp,
		Hostname:       opts.Host,
		ParentSnapshot: *parentSnapshotID,
	}

	if !gopts.JSON {
		progressPrinter.V("start backup on %v", targets)
	}
	_, id, err := arch.Snapshot(gopts.ctx, targets, snapshotOpts)

	// cleanly shutdown all running goroutines
	cancel()

	// let's see if one returned an error
	werr := wg.Wait()

	// return original error
	if err != nil {
		return errors.Fatalf("unable to save snapshot: %v", err)
	}

	// Report finished execution
	progressReporter.Finish(id)
	if !gopts.JSON && !opts.DryRun {
		progressPrinter.P("snapshot %s saved\n", id.Str())
	}
	if !success {
		return ErrInvalidSourceData
	}

	// Return error if any
	return werr
}
