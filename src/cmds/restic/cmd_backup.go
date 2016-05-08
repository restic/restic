package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"restic"
	"restic/backend"
	"restic/debug"
	"restic/filter"
	"restic/repository"
	"strings"
	"time"

	"golang.org/x/crypto/ssh/terminal"
)

type CmdBackup struct {
	Parent        string   `short:"p" long:"parent"                  description:"use this parent snapshot (default: last snapshot in repo that has the same target)"`
	Force         bool     `short:"f" long:"force"                   description:"Force re-reading the target. Overrides the \"parent\" flag"`
	Excludes      []string `short:"e" long:"exclude"                 description:"Exclude a pattern (can be specified multiple times)"`
	ExcludeFile   string   `long:"exclude-file"                      description:"Read exclude-patterns from file"`
	Stdin         bool     `long:"stdin"                             description:"read backup data from stdin"`
	StdinFilename string   `long:"stdin-filename"    default:"stdin" description:"file name to use when reading from stdin"`

	global *GlobalOptions
}

func init() {
	_, err := parser.AddCommand("backup",
		"save file/directory",
		"The backup command creates a snapshot of a file or directory",
		&CmdBackup{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

func formatBytes(c uint64) string {
	b := float64(c)

	switch {
	case c > 1<<40:
		return fmt.Sprintf("%.3f TiB", b/(1<<40))
	case c > 1<<30:
		return fmt.Sprintf("%.3f GiB", b/(1<<30))
	case c > 1<<20:
		return fmt.Sprintf("%.3f MiB", b/(1<<20))
	case c > 1<<10:
		return fmt.Sprintf("%.3f KiB", b/(1<<10))
	default:
		return fmt.Sprintf("%dB", c)
	}
}

func formatSeconds(sec uint64) string {
	hours := sec / 3600
	sec -= hours * 3600
	min := sec / 60
	sec -= min * 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, min, sec)
	}

	return fmt.Sprintf("%d:%02d", min, sec)
}

func formatPercent(numerator uint64, denominator uint64) string {
	if denominator == 0 {
		return ""
	}

	percent := 100.0 * float64(numerator) / float64(denominator)

	if percent > 100 {
		percent = 100
	}

	return fmt.Sprintf("%3.2f%%", percent)
}

func formatRate(bytes uint64, duration time.Duration) string {
	sec := float64(duration) / float64(time.Second)
	rate := float64(bytes) / sec / (1 << 20)
	return fmt.Sprintf("%.2fMiB/s", rate)
}

func formatDuration(d time.Duration) string {
	sec := uint64(d / time.Second)
	return formatSeconds(sec)
}

func printTree2(indent int, t *restic.Tree) {
	for _, node := range t.Nodes {
		if node.Tree() != nil {
			fmt.Printf("%s%s/\n", strings.Repeat("  ", indent), node.Name)
			printTree2(indent+1, node.Tree())
		} else {
			fmt.Printf("%s%s\n", strings.Repeat("  ", indent), node.Name)
		}
	}
}

func (cmd CmdBackup) Usage() string {
	return "DIR/FILE [DIR/FILE] [...]"
}

func (cmd CmdBackup) newScanProgress() *restic.Progress {
	if !cmd.global.ShowProgress() {
		return nil
	}

	p := restic.NewProgress(time.Second)
	p.OnUpdate = func(s restic.Stat, d time.Duration, ticker bool) {
		fmt.Printf("\x1b[2K[%s] %d directories, %d files, %s\r", formatDuration(d), s.Dirs, s.Files, formatBytes(s.Bytes))
	}
	p.OnDone = func(s restic.Stat, d time.Duration, ticker bool) {
		fmt.Printf("\x1b[2Kscanned %d directories, %d files in %s\n", s.Dirs, s.Files, formatDuration(d))
	}

	return p
}

func (cmd CmdBackup) newArchiveProgress(todo restic.Stat) *restic.Progress {
	if !cmd.global.ShowProgress() {
		return nil
	}

	archiveProgress := restic.NewProgress(time.Second)

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
			maxlen := w - len(status2)

			if maxlen < 4 {
				status1 = ""
			} else if len(status1) > maxlen {
				status1 = status1[:maxlen-4]
				status1 += "... "
			}
		}

		fmt.Printf("\x1b[2K%s%s\r", status1, status2)
	}

	archiveProgress.OnDone = func(s restic.Stat, d time.Duration, ticker bool) {
		fmt.Printf("\nduration: %s, %s\n", formatDuration(d), formatRate(todo.Bytes, d))
	}

	return archiveProgress
}

func (cmd CmdBackup) newArchiveStdinProgress() *restic.Progress {
	if !cmd.global.ShowProgress() {
		return nil
	}

	archiveProgress := restic.NewProgress(time.Second)

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

		fmt.Printf("\x1b[2K%s\r", status1)
	}

	archiveProgress.OnDone = func(s restic.Stat, d time.Duration, ticker bool) {
		fmt.Printf("\nduration: %s, %s\n", formatDuration(d), formatRate(s.Bytes, d))
	}

	return archiveProgress
}

func samePaths(expected, actual []string) bool {
	if expected == nil || actual == nil {
		return true
	}

	if len(expected) != len(actual) {
		return false
	}
	for i := range expected {
		if expected[i] != actual[i] {
			return false
		}
	}

	return true
}

var errNoSnapshotFound = errors.New("no snapshot found")

func findLatestSnapshot(repo *repository.Repository, targets []string) (backend.ID, error) {
	var (
		latest   time.Time
		latestID backend.ID
		found    bool
	)

	for snapshotID := range repo.List(backend.Snapshot, make(chan struct{})) {
		snapshot, err := restic.LoadSnapshot(repo, snapshotID)
		if err != nil {
			return backend.ID{}, fmt.Errorf("Error listing snapshot: %v", err)
		}
		if snapshot.Time.After(latest) && samePaths(snapshot.Paths, targets) {
			latest = snapshot.Time
			latestID = snapshotID
			found = true
		}
	}

	if !found {
		return backend.ID{}, errNoSnapshotFound
	}

	return latestID, nil
}

// filterExisting returns a slice of all existing items, or an error if no
// items exist at all.
func filterExisting(items []string) (result []string, err error) {
	for _, item := range items {
		_, err := os.Lstat(item)
		if err != nil && os.IsNotExist(err) {
			continue
		}

		result = append(result, item)
	}

	if len(result) == 0 {
		return nil, errors.New("all target directories/files do not exist")
	}

	return
}

func (cmd CmdBackup) readFromStdin(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("when reading from stdin, no additional files can be specified")
	}

	repo, err := cmd.global.OpenRepository()
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

	_, id, err := restic.ArchiveReader(repo, cmd.newArchiveStdinProgress(), os.Stdin, cmd.StdinFilename)
	if err != nil {
		return err
	}

	fmt.Printf("archived as %v\n", id.Str())
	return nil
}

func (cmd CmdBackup) Execute(args []string) error {
	if cmd.Stdin {
		return cmd.readFromStdin(args)
	}

	if len(args) == 0 {
		return fmt.Errorf("wrong number of parameters, Usage: %s", cmd.Usage())
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

	repo, err := cmd.global.OpenRepository()
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

	var parentSnapshotID *backend.ID

	// Force using a parent
	if !cmd.Force && cmd.Parent != "" {
		id, err := restic.FindSnapshot(repo, cmd.Parent)
		if err != nil {
			return fmt.Errorf("invalid id %q: %v", cmd.Parent, err)
		}

		parentSnapshotID = &id
	}

	// Find last snapshot to set it as parent, if not already set
	if !cmd.Force && parentSnapshotID == nil {
		id, err := findLatestSnapshot(repo, target)
		if err == nil {
			parentSnapshotID = &id
		} else if err != errNoSnapshotFound {
			return err
		}
	}

	if parentSnapshotID != nil {
		cmd.global.Verbosef("using parent snapshot %v\n", parentSnapshotID.Str())
	}

	cmd.global.Verbosef("scan %v\n", target)

	// add patterns from file
	if cmd.ExcludeFile != "" {
		file, err := os.Open(cmd.ExcludeFile)
		if err != nil {
			cmd.global.Warnf("error reading exclude patterns: %v", err)
			return nil
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "#") {
				line = os.ExpandEnv(line)
				cmd.Excludes = append(cmd.Excludes, line)
			}
		}
	}

	selectFilter := func(item string, fi os.FileInfo) bool {
		matched, err := filter.List(cmd.Excludes, item)
		if err != nil {
			cmd.global.Warnf("error for exclude pattern: %v", err)
		}

		if matched {
			debug.Log("backup.Execute", "path %q excluded by a filter", item)
		}

		return !matched
	}

	stat, err := restic.Scan(target, selectFilter, cmd.newScanProgress())
	if err != nil {
		return err
	}

	arch := restic.NewArchiver(repo)
	arch.Excludes = cmd.Excludes
	arch.SelectFilter = selectFilter

	arch.Error = func(dir string, fi os.FileInfo, err error) error {
		// TODO: make ignoring errors configurable
		cmd.global.Warnf("\x1b[2K\rerror for %s: %v\n", dir, err)
		return nil
	}

	_, id, err := arch.Snapshot(cmd.newArchiveProgress(stat), target, parentSnapshotID)
	if err != nil {
		return err
	}

	cmd.global.Verbosef("snapshot %s saved\n", id.Str())

	return nil
}
