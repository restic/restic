package restorer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/restic/restic/internal/repository"
	rtest "github.com/restic/restic/internal/test"
)

type symlinkRestoreProgress struct {
	noopProgressReporter
	onRestore func(string)
}

func (p symlinkRestoreProgress) AddProgress(name string, action ItemAction, _, _ uint64) {
	if action == ActionOtherRestored {
		p.onRestore(filepath.Base(name))
	}
}

func TestRestorerSymlinkSkippedDependency(t *testing.T) {
	for _, dependent := range []bool{false, true} {
		name, linkTarget := "independent", "directory"
		if dependent {
			name, linkTarget = "dependent", "a"
		}
		t.Run(name, func(t *testing.T) {
			repo := repository.TestRepository(t)
			sn, _ := saveSnapshot(t, repo, Snapshot{Nodes: map[string]Node{
				"a":         Symlink{Target: "directory"},
				"m":         Symlink{Target: linkTarget},
				"z":         Symlink{Target: "directory"},
				"directory": Dir{},
			}}, noopGetGenericAttributes)
			target := t.TempDir()
			// Preserve an existing directory link while its target is restored later.
			rtest.OK(t, os.Mkdir(filepath.Join(target, "z"), 0o700))
			rtest.OK(t, os.Symlink("z", filepath.Join(target, "a")))
			rtest.OK(t, os.Remove(filepath.Join(target, "z")))
			visited := false
			progress := symlinkRestoreProgress{onRestore: func(name string) {
				if name == "m" {
					visited = true
					_, err := os.Lstat(filepath.Join(target, "z"))
					if dependent {
						rtest.OK(t, err)
					} else {
						rtest.Assert(t, errors.Is(err, os.ErrNotExist), "skipped link resolved an unneeded dependency: %v", err)
					}
				}
			}}
			res := NewRestorer(repo, sn, Options{Overwrite: OverwriteNever, Progress: progress})
			_, err := res.RestoreTo(t.Context(), target)
			rtest.OK(t, err)
			rtest.Assert(t, visited, "missing restore progress for m")
			actual, err := os.Readlink(filepath.Join(target, "a"))
			rtest.OK(t, err)
			rtest.Equals(t, "z", actual)
			_, err = os.ReadDir(filepath.Join(target, "m"))
			rtest.OK(t, err)
		})
	}
}

func TestRestorerSymlinkCancellationDuringDependency(t *testing.T) {
	repo := repository.TestRepository(t)
	sn, _ := saveSnapshot(t, repo, Snapshot{Nodes: map[string]Node{
		"a":         Symlink{Target: "b"},
		"b":         Symlink{Target: "directory"},
		"directory": Dir{},
	}}, noopGetGenericAttributes)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	progress := symlinkRestoreProgress{onRestore: func(name string) {
		if name == "b" {
			cancel()
		}
	}}
	res := NewRestorer(repo, sn, Options{Progress: progress})
	target := t.TempDir()
	_, err := res.RestoreTo(ctx, target)
	rtest.Assert(t, errors.Is(err, context.Canceled), "expected cancellation, got %v", err)
	actual, err := os.Readlink(filepath.Join(target, "b"))
	rtest.OK(t, err)
	rtest.Equals(t, "directory", actual)
	_, err = os.Lstat(filepath.Join(target, "a"))
	rtest.Assert(t, errors.Is(err, os.ErrNotExist), "dependent link was created after cancellation: %v", err)
}

func TestRestorerSymlinkNonRelativeTargetOrder(t *testing.T) {
	for _, rooted := range []bool{false, true} {
		name := "absolute"
		if rooted {
			name = "root-relative"
		}
		t.Run(name, func(t *testing.T) {
			target := t.TempDir()
			linkTarget := filepath.Join(target, "z")
			if rooted {
				linkTarget = linkTarget[len(filepath.VolumeName(linkTarget)):]
			}
			repo := repository.TestRepository(t)
			sn, _ := saveSnapshot(t, repo, Snapshot{Nodes: map[string]Node{
				"a":         Symlink{Target: linkTarget},
				"z":         Symlink{Target: "directory"},
				"directory": Dir{},
			}}, noopGetGenericAttributes)
			visited := false
			progress := symlinkRestoreProgress{onRestore: func(name string) {
				if name == "a" {
					visited = true
					_, err := os.Lstat(filepath.Join(target, "z"))
					rtest.Assert(t, errors.Is(err, os.ErrNotExist), "non-relative target changed restore order: %v", err)
				}
			}}
			res := NewRestorer(repo, sn, Options{Progress: progress})
			_, err := res.RestoreTo(t.Context(), target)
			rtest.OK(t, err)
			rtest.Assert(t, visited, "missing restore progress for a")
			actual, err := os.Readlink(filepath.Join(target, "a"))
			rtest.OK(t, err)
			rtest.Equals(t, linkTarget, actual)
		})
	}
}

func TestRestorerSymlinkSkippedCycle(t *testing.T) {
	repo := repository.TestRepository(t)
	sn, _ := saveSnapshot(t, repo, Snapshot{Nodes: map[string]Node{
		"a": Symlink{Target: "b"},
		"b": Symlink{Target: "c"},
		"c": Symlink{Target: "b"},
	}}, noopGetGenericAttributes)
	target := t.TempDir()
	rtest.OK(t, os.Symlink("c", filepath.Join(target, "b")))
	rtest.OK(t, os.Symlink("b", filepath.Join(target, "c")))
	res := NewRestorer(repo, sn, Options{Overwrite: OverwriteNever})
	res.Error = func(_ string, _ error) error { return nil }
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err := res.RestoreTo(ctx, target)
	rtest.OK(t, err)
	rtest.OK(t, ctx.Err())
	for name, expected := range map[string]string{"a": "b", "b": "c", "c": "b"} {
		actual, err := os.Readlink(filepath.Join(target, name))
		rtest.OK(t, err)
		rtest.Equals(t, expected, actual)
	}
}
