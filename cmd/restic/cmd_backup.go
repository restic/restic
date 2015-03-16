package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"golang.org/x/crypto/ssh/terminal"
)

type CmdBackup struct {
	Parent string `short:"p" long:"parent"    description:"use this parent snapshot (default: not set)"`
}

func init() {
	_, err := parser.AddCommand("backup",
		"save file/directory",
		"The backup command creates a snapshot of a file or directory",
		&CmdBackup{})
	if err != nil {
		panic(err)
	}
}

func format_bytes(c uint64) string {
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

func format_seconds(sec uint64) string {
	hours := sec / 3600
	sec -= hours * 3600
	min := sec / 60
	sec -= min * 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, min, sec)
	}

	return fmt.Sprintf("%d:%02d", min, sec)
}

func format_duration(d time.Duration) string {
	sec := uint64(d / time.Second)
	return format_seconds(sec)
}

func print_tree2(indent int, t *restic.Tree) {
	for _, node := range t.Nodes {
		if node.Tree() != nil {
			fmt.Printf("%s%s/\n", strings.Repeat("  ", indent), node.Name)
			print_tree2(indent+1, node.Tree())
		} else {
			fmt.Printf("%s%s\n", strings.Repeat("  ", indent), node.Name)
		}
	}
}

func (cmd CmdBackup) Usage() string {
	return "DIR/FILE [snapshot-ID]"
}

func newScanProgress() *restic.Progress {
	if !terminal.IsTerminal(int(os.Stdout.Fd())) {
		return nil
	}

	p := restic.NewProgress(time.Second)
	p.OnUpdate = func(s restic.Stat, d time.Duration, ticker bool) {
		fmt.Printf("\x1b[2K\r[%s] %d directories, %d files, %s", format_duration(d), s.Dirs, s.Files, format_bytes(s.Bytes))
	}
	p.OnDone = func(s restic.Stat, d time.Duration, ticker bool) {
		fmt.Printf("\nDone in %s\n", format_duration(d))
	}

	return p
}

func newArchiveProgress(todo restic.Stat) *restic.Progress {
	if !terminal.IsTerminal(int(os.Stdout.Fd())) {
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
		percent := float64(s.Bytes) / float64(todo.Bytes) * 100
		if percent > 100 {
			percent = 100
		}

		status1 := fmt.Sprintf("[%s] %3.2f%%  %s/s  %s / %s  %d / %d items  ",
			format_duration(d),
			percent,
			format_bytes(bps),
			format_bytes(s.Bytes), format_bytes(todo.Bytes),
			itemsDone, itemsTodo)
		status2 := fmt.Sprintf("ETA %s ", format_seconds(eta))

		w, _, err := terminal.GetSize(int(os.Stdout.Fd()))
		if err == nil {
			if len(status1)+len(status2) > w {
				max := w - len(status2) - 4
				status1 = status1[:max] + "... "
			}
		}

		fmt.Printf("\x1b[2K\r%s%s", status1, status2)
	}

	archiveProgress.OnDone = func(s restic.Stat, d time.Duration, ticker bool) {
		sec := uint64(d / time.Second)
		fmt.Printf("\nduration: %s, %.2fMiB/s\n",
			format_duration(d),
			float64(todo.Bytes)/float64(sec)/(1<<20))
	}

	return archiveProgress
}

func (cmd CmdBackup) Execute(args []string) error {
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

	s, err := OpenRepo()
	if err != nil {
		return err
	}

	var parentSnapshotID backend.ID

	if cmd.Parent != "" {
		parentSnapshotID, err = s.FindSnapshot(cmd.Parent)
		if err != nil {
			return fmt.Errorf("invalid id %q: %v", cmd.Parent, err)
		}

		fmt.Printf("found parent snapshot %v\n", parentSnapshotID)
	}

	fmt.Printf("scan %v\n", target)

	stat, err := restic.Scan(target, newScanProgress())

	// TODO: add filter
	// arch.Filter = func(dir string, fi os.FileInfo) bool {
	// 	return true
	// }

	arch, err := restic.NewArchiver(s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "err: %v\n", err)
	}

	arch.Error = func(dir string, fi os.FileInfo, err error) error {
		// TODO: make ignoring errors configurable
		fmt.Fprintf(os.Stderr, "\x1b[2K\rerror for %s: %v\n", dir, err)
		return nil
	}

	fmt.Printf("loading blobs\n")
	err = arch.Preload()
	if err != nil {
		return err
	}

	_, id, err := arch.Snapshot(newArchiveProgress(stat), target, parentSnapshotID)
	if err != nil {
		return err
	}

	plen, err := s.PrefixLength(backend.Snapshot)
	if err != nil {
		return err
	}

	fmt.Printf("snapshot %s saved\n", id[:plen])

	return nil
}
