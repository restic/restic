package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fd0/khepri"
	"github.com/fd0/khepri/backend"
	"golang.org/x/crypto/ssh/terminal"
)

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
		return fmt.Sprintf("%d B", c)
	}
}

func print_tree2(indent int, t *khepri.Tree) {
	for _, node := range *t {
		if node.Tree != nil {
			fmt.Printf("%s%s/\n", strings.Repeat("  ", indent), node.Name)
			print_tree2(indent+1, node.Tree)
		} else {
			fmt.Printf("%s%s\n", strings.Repeat("  ", indent), node.Name)
		}
	}
}

func commandBackup(be backend.Server, key *khepri.Key, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: backup [dir|file]")
	}

	target := args[0]

	arch, err := khepri.NewArchiver(be, key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "err: %v\n", err)
	}
	arch.Error = func(dir string, fi os.FileInfo, err error) error {
		// TODO: make ignoring errors configurable
		fmt.Fprintf(os.Stderr, "\nerror for %s: %v\n%v\n", dir, err, fi)
		return nil
	}

	fmt.Printf("scanning %s\n", target)

	if terminal.IsTerminal(int(os.Stdout.Fd())) {
		arch.ScannerUpdate = func(stats khepri.Stats) {
			fmt.Printf("\r%6d directories, %6d files, %14s", stats.Directories, stats.Files, format_bytes(stats.Bytes))
		}
	}

	// TODO: add filter
	// arch.Filter = func(dir string, fi os.FileInfo) bool {
	// 	return true
	// }

	t, err := arch.LoadTree(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return err
	}

	fmt.Printf("\r%6d directories, %6d files, %14s\n", arch.Stats.Directories, arch.Stats.Files, format_bytes(arch.Stats.Bytes))

	stats := khepri.Stats{}
	if terminal.IsTerminal(int(os.Stdout.Fd())) {
		arch.SaveUpdate = func(s khepri.Stats) {
			stats.Files += s.Files
			stats.Directories += s.Directories
			stats.Other += s.Other
			stats.Bytes += s.Bytes

			fmt.Printf("\r%3.2f%% %d/%d directories, %d/%d files, %s/%s",
				float64(stats.Bytes)/float64(arch.Stats.Bytes)*100,
				stats.Directories, arch.Stats.Directories,
				stats.Files, arch.Stats.Files,
				format_bytes(stats.Bytes), format_bytes(arch.Stats.Bytes))
		}
	}

	start := time.Now()
	sn, id, err := arch.Snapshot(target, t)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}

	fmt.Printf("\nsnapshot %s saved: %v\n", id, sn)
	duration := time.Now().Sub(start)
	fmt.Printf("duration: %s, %.2fMiB/s\n", duration, float64(arch.Stats.Bytes)/float64(duration/time.Second)/(1<<20))

	return nil
}
