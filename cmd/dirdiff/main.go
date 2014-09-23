package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const backlog = 100

type entry struct {
	path string
	fi   os.FileInfo
}

func walk(dir string) <-chan *entry {
	ch := make(chan *entry, 100)

	go func() {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				return nil
			}

			name, err := filepath.Rel(dir, path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				return nil
			}

			ch <- &entry{
				path: name,
				fi:   info,
			}

			return nil
		})

		if err != nil {
			fmt.Fprintf(os.Stderr, "Walk() error: %v\n", err)
		}

		close(ch)
	}()

	// first element is root
	_ = <-ch

	return ch
}

func (e *entry) equals(other *entry) bool {
	if e.path != other.path {
		fmt.Printf("path does not match\n")
		return false
	}

	if e.fi.Mode() != other.fi.Mode() {
		fmt.Printf("mode does not match\n")
		return false
	}

	if e.fi.ModTime() != other.fi.ModTime() {
		fmt.Printf("ModTime does not match\n")
		return false
	}

	stat, _ := e.fi.Sys().(*syscall.Stat_t)
	stat2, _ := other.fi.Sys().(*syscall.Stat_t)

	if stat.Uid != stat2.Uid || stat2.Gid != stat2.Gid {
		return false
	}

	return true
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "USAGE: %s DIR1 DIR2\n", os.Args[0])
		os.Exit(1)
	}

	ch1 := walk(os.Args[1])
	ch2 := walk(os.Args[2])

	changes := false

	var a, b *entry
	for {
		var ok bool

		if ch1 != nil && a == nil {
			a, ok = <-ch1
			if !ok {
				ch1 = nil
			}
		}

		if ch2 != nil && b == nil {
			b, ok = <-ch2
			if !ok {
				ch2 = nil
			}
		}

		if ch1 == nil && ch2 == nil {
			break
		}

		if ch1 == nil {
			fmt.Printf("+%v\n", b.path)
			changes = true
		} else if ch2 == nil {
			fmt.Printf("-%v\n", a.path)
			changes = true
		} else if !a.equals(b) {
			if a.path < b.path {
				fmt.Printf("-%v\n", a.path)
				changes = true
				a = nil
				continue
			} else if a.path > b.path {
				fmt.Printf("+%v\n", b.path)
				changes = true
				b = nil
				continue
			} else {
				fmt.Printf("%%%v\n", a.path)
				changes = true
			}
		}

		a, b = nil, nil
	}

	if changes {
		os.Exit(1)
	}
}
