package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	. "github.com/restic/restic/test"
)

type dirEntry struct {
	path string
	fi   os.FileInfo
}

func walkDir(dir string) <-chan *dirEntry {
	ch := make(chan *dirEntry, 100)

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

			ch <- &dirEntry{
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

func (e *dirEntry) equals(other *dirEntry) bool {
	if e.path != other.path {
		fmt.Fprintf(os.Stderr, "%v: path does not match\n", e.path)
		return false
	}

	if e.fi.Mode() != other.fi.Mode() {
		fmt.Fprintf(os.Stderr, "%v: mode does not match\n", e.path)
		return false
	}

	if e.fi.ModTime() != other.fi.ModTime() {
		fmt.Fprintf(os.Stderr, "%v: ModTime does not match\n", e.path)
		return false
	}

	stat, _ := e.fi.Sys().(*syscall.Stat_t)
	stat2, _ := other.fi.Sys().(*syscall.Stat_t)

	if stat.Uid != stat2.Uid || stat2.Gid != stat2.Gid {
		fmt.Fprintf(os.Stderr, "%v: UID or GID do not match\n", e.path)
		return false
	}

	return true
}

// directoriesEqualContents checks if both directories contain exactly the same
// contents.
func directoriesEqualContents(dir1, dir2 string) bool {
	ch1 := walkDir(dir1)
	ch2 := walkDir(dir2)

	changes := false

	var a, b *dirEntry
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
		return false
	}

	return true
}

type dirStat struct {
	files, dirs, other uint
	size               uint64
}

func isFile(fi os.FileInfo) bool {
	return fi.Mode()&(os.ModeType|os.ModeCharDevice) == 0
}

// dirStats walks dir and collects stats.
func dirStats(dir string) (stat dirStat) {
	for entry := range walkDir(dir) {
		if isFile(entry.fi) {
			stat.files++
			stat.size += uint64(entry.fi.Size())
			continue
		}

		if entry.fi.IsDir() {
			stat.dirs++
			continue
		}

		stat.other++
	}

	return stat
}

type testEnvironment struct {
	base, cache, repo, testdata string
}

func configureRestic(t testing.TB, cache, repo string) {
	mainOpts.CacheDir = cache
	mainOpts.Repo = repo
	mainOpts.Quiet = true

	mainOpts.password = TestPassword
}

func cleanupTempdir(t testing.TB, tempdir string) {
	if !TestCleanup {
		t.Logf("leaving temporary directory %v used for test", tempdir)
		return
	}

	OK(t, os.RemoveAll(tempdir))
}

// withTestEnvironment creates a test environment and calls f with it. After f has
// returned, the temporary directory is removed.
func withTestEnvironment(t testing.TB, f func(*testEnvironment)) {
	if !RunIntegrationTest {
		t.Skip("integration tests disabled")
	}

	tempdir, err := ioutil.TempDir(TestTempDir, "restic-test-")
	OK(t, err)

	env := testEnvironment{
		base:     tempdir,
		cache:    filepath.Join(tempdir, "cache"),
		repo:     filepath.Join(tempdir, "repo"),
		testdata: filepath.Join(tempdir, "testdata"),
	}

	configureRestic(t, env.cache, env.repo)
	OK(t, os.MkdirAll(env.testdata, 0700))

	f(&env)

	if !TestCleanup {
		t.Logf("leaving temporary directory %v used for test", tempdir)
		return
	}

	OK(t, os.RemoveAll(tempdir))
}
