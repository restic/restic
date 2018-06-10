package main

import (
	"bufio"
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	tomb "gopkg.in/tomb.v2"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/textfile"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/termstatus"
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

		var t tomb.Tomb
		term := termstatus.New(globalOptions.stdout, globalOptions.stderr, globalOptions.Quiet)
		t.Go(func() error { term.Run(t.Context(globalOptions.ctx)); return nil })

		err := runBackup(backupOptions, globalOptions, term, args)
		if err != nil {
			return err
		}
		t.Kill(nil)
		return t.Wait()
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

// readFromFile will read all lines from the given filename and return them as
// a string array, if filename is empty readFromFile returns and empty string
// array. If filename is a dash (-), readFromFile will read the lines from the
// standard input.
func readLinesFromFile(filename string) ([]string, error) {
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
		line := strings.TrimSpace(scanner.Text())
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

// Check returns an error when an invalid combination of options was set.
func (opts BackupOptions) Check(gopts GlobalOptions, args []string) error {
	if opts.FilesFrom == "-" && gopts.password == "" {
		return errors.Fatal("unable to read password from stdin when data is to be read from stdin, use --password-file or $RESTIC_PASSWORD")
	}

	if opts.Stdin {
		if opts.FilesFrom != "" {
			return errors.Fatal("--stdin and --files-from cannot be used together")
		}

		if len(args) > 0 {
			return errors.Fatal("--stdin was specified and files/dirs were listed as arguments")
		}
	}

	return nil
}

// collectRejectFuncs returns a list of all functions which may reject data
// from being saved in a snapshot
func collectRejectFuncs(opts BackupOptions, repo *repository.Repository, targets []string) (fs []RejectFunc, err error) {
	// allowed devices
	if opts.ExcludeOtherFS {
		f, err := rejectByDevice(targets)
		if err != nil {
			return nil, err
		}
		fs = append(fs, f)
	}

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
		opts.Excludes = append(opts.Excludes, readExcludePatternsFromFiles(opts.ExcludeFiles)...)
	}

	if len(opts.Excludes) > 0 {
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

// readExcludePatternsFromFiles reads all exclude files and returns the list of
// exclude patterns.
func readExcludePatternsFromFiles(excludeFiles []string) []string {
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

// collectTargets returns a list of target files/dirs from several sources.
func collectTargets(opts BackupOptions, args []string) (targets []string, err error) {
	if opts.Stdin {
		return nil, nil
	}

	fromfile, err := readLinesFromFile(opts.FilesFrom)
	if err != nil {
		return nil, err
	}

	// merge files from files-from into normal args so we can reuse the normal
	// args checks and have the ability to use both files-from and args at the
	// same time
	args = append(args, fromfile...)
	if len(args) == 0 && !opts.Stdin {
		return nil, errors.Fatal("nothing to backup, please specify target files/dirs")
	}

	targets = args
	targets, err = filterExisting(targets)
	if err != nil {
		return nil, err
	}

	return targets, nil
}

// parent returns the ID of the parent snapshot. If there is none, nil is
// returned.
func findParentSnapshot(ctx context.Context, repo restic.Repository, opts BackupOptions, targets []string) (parentID *restic.ID, err error) {
	// Force using a parent
	if !opts.Force && opts.Parent != "" {
		id, err := restic.FindSnapshot(repo, opts.Parent)
		if err != nil {
			return nil, errors.Fatalf("invalid id %q: %v", opts.Parent, err)
		}

		parentID = &id
	}

	// Find last snapshot to set it as parent, if not already set
	if !opts.Force && parentID == nil {
		id, err := restic.FindLatestSnapshot(ctx, repo, targets, []restic.TagList{}, opts.Hostname)
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
		timeStamp, err = time.Parse(TimeFormat, opts.TimeStamp)
		if err != nil {
			return errors.Fatalf("error in time option: %v\n", err)
		}
	}

	var t tomb.Tomb

	p := ui.NewBackup(term, gopts.verbosity)

	// use the terminal for stdout/stderr
	prevStdout, prevStderr := gopts.stdout, gopts.stderr
	defer func() {
		gopts.stdout, gopts.stderr = prevStdout, prevStderr
	}()
	gopts.stdout, gopts.stderr = p.Stdout(), p.Stderr()

	if s, ok := os.LookupEnv("RESTIC_PROGRESS_FPS"); ok {
		fps, err := strconv.Atoi(s)
		if err == nil && fps >= 1 {
			if fps > 60 {
				fps = 60
			}
			p.MinUpdatePause = time.Second / time.Duration(fps)
		}
	}

	t.Go(func() error { return p.Run(t.Context(gopts.ctx)) })

	p.V("open repository")
	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	p.V("lock repository")
	lock, err := lockRepo(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	// rejectFuncs collect functions that can reject items from the backup
	rejectFuncs, err := collectRejectFuncs(opts, repo, targets)
	if err != nil {
		return err
	}

	p.V("load index files")
	err = repo.LoadIndex(gopts.ctx)
	if err != nil {
		return err
	}

	parentSnapshotID, err := findParentSnapshot(gopts.ctx, repo, opts, targets)
	if err != nil {
		return err
	}

	if parentSnapshotID != nil {
		p.V("using parent snapshot %v\n", parentSnapshotID.Str())
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
	if opts.Stdin {
		p.V("read data from stdin")
		targetFS = &fs.Reader{
			ModTime:    timeStamp,
			Name:       opts.StdinFilename,
			Mode:       0644,
			ReadCloser: os.Stdin,
		}
		targets = []string{opts.StdinFilename}
	}

	sc := archiver.NewScanner(targetFS)
	sc.Select = selectFilter
	sc.Error = p.ScannerError
	sc.Result = p.ReportTotal

	p.V("start scan on %v", targets)
	t.Go(func() error { return sc.Scan(t.Context(gopts.ctx), targets) })

	arch := archiver.New(repo, targetFS, archiver.Options{})
	arch.Select = selectFilter
	arch.WithAtime = opts.WithAtime
	arch.Error = p.Error
	arch.CompleteItem = p.CompleteItemFn
	arch.StartFile = p.StartFile
	arch.CompleteBlob = p.CompleteBlob

	if parentSnapshotID == nil {
		parentSnapshotID = &restic.ID{}
	}

	snapshotOpts := archiver.SnapshotOptions{
		Excludes:       opts.Excludes,
		Tags:           opts.Tags,
		Time:           timeStamp,
		Hostname:       opts.Hostname,
		ParentSnapshot: *parentSnapshotID,
	}

	uploader := archiver.IndexUploader{
		Repository: repo,
		Start: func() {
			p.VV("uploading intermediate index")
		},
		Complete: func(id restic.ID) {
			p.V("uploaded intermediate index %v", id.Str())
		},
	}

	t.Go(func() error {
		return uploader.Upload(gopts.ctx, t.Context(gopts.ctx), 30*time.Second)
	})

	p.V("start backup on %v", targets)
	_, id, err := arch.Snapshot(gopts.ctx, targets, snapshotOpts)
	if err != nil {
		return errors.Fatalf("unable to save snapshot: %v", err)
	}

	p.Finish()
	p.P("snapshot %s saved\n", id.Str())

	// cleanly shutdown all running goroutines
	t.Kill(nil)

	// let's see if one returned an error
	err = t.Wait()
	if err != nil {
		return err
	}

	return nil
}
