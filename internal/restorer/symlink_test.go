package restorer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/repository"
	rtest "github.com/restic/restic/internal/test"
)

func TestRestorerSymlinkChains(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		name := "forward"
		first, second, third := "a", "b", "c"
		if reverse {
			name = "reverse"
			first, second, third = "z", "y", "x"
		}
		t.Run(name, func(t *testing.T) {
			repo := repository.TestRepository(t)
			sn, _ := saveSnapshot(t, repo, Snapshot{Nodes: map[string]Node{
				first:  Symlink{Target: second},
				second: Symlink{Target: third},
				third:  Symlink{Target: "directory"},
				"directory": Dir{Nodes: map[string]Node{
					"file": File{Data: "restored content"},
				}},
				"file-link":      Symlink{Target: "file-link-next"},
				"file-link-next": Symlink{Target: "file-target"},
				"file-target":    File{Data: "file content"},
			}}, noopGetGenericAttributes)
			target := t.TempDir()
			res := NewRestorer(repo, sn, Options{})
			_, err := res.RestoreTo(context.Background(), target)
			rtest.OK(t, err)

			for _, link := range []struct{ name, target string }{
				{first, second}, {second, third}, {third, "directory"},
			} {
				linkPath := filepath.Join(target, link.name)
				actual, err := os.Readlink(linkPath)
				rtest.OK(t, err)
				rtest.Equals(t, link.target, actual)
				entries, err := os.ReadDir(linkPath)
				rtest.OK(t, err)
				rtest.Equals(t, 1, len(entries))
				content, err := os.ReadFile(filepath.Join(linkPath, "file"))
				rtest.OK(t, err)
				rtest.Equals(t, "restored content", string(content))
			}
			for name, expected := range map[string]string{"file-link": "file-link-next", "file-link-next": "file-target"} {
				linkPath := filepath.Join(target, name)
				actual, err := os.Readlink(linkPath)
				rtest.OK(t, err)
				rtest.Equals(t, expected, actual)
				content, err := os.ReadFile(linkPath)
				rtest.OK(t, err)
				rtest.Equals(t, "file content", string(content))
			}
		})
	}
}

func TestRestorerSymlinkPathComponents(t *testing.T) {
	for _, tc := range []struct {
		name   string
		nodes  map[string]Node
		link   string
		target string
	}{
		{
			name: "intermediate symlink",
			nodes: map[string]Node{
				"a":     Symlink{Target: filepath.FromSlash("alias/child-link")},
				"alias": Symlink{Target: "actual-directory"},
				"actual-directory": Dir{Nodes: map[string]Node{
					"child-link": Symlink{Target: filepath.FromSlash("../targetdir")},
				}},
				"targetdir": Dir{Nodes: map[string]Node{"file": File{Data: "content"}}},
			},
			link: "a", target: filepath.FromSlash("alias/child-link"),
		},
		{
			name: "nested relative target",
			nodes: map[string]Node{
				"links": Dir{Nodes: map[string]Node{
					"a": Symlink{Target: filepath.FromSlash("../z")},
				}},
				"z":         Symlink{Target: "targetdir"},
				"targetdir": Dir{Nodes: map[string]Node{"file": File{Data: "content"}}},
			},
			link: filepath.FromSlash("links/a"), target: filepath.FromSlash("../z"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := repository.TestRepository(t)
			sn, _ := saveSnapshot(t, repo, Snapshot{Nodes: tc.nodes}, noopGetGenericAttributes)
			res := NewRestorer(repo, sn, Options{})
			target := t.TempDir()
			_, err := res.RestoreTo(t.Context(), target)
			rtest.OK(t, err)
			linkPath := filepath.Join(target, tc.link)
			actual, err := os.Readlink(linkPath)
			rtest.OK(t, err)
			rtest.Equals(t, tc.target, actual)
			entries, err := os.ReadDir(linkPath)
			rtest.OK(t, err)
			rtest.Equals(t, 1, len(entries))
			content, err := os.ReadFile(filepath.Join(linkPath, "file"))
			rtest.OK(t, err)
			rtest.Equals(t, "content", string(content))
		})
	}
}

func TestRestorerSymlinkExistingDependency(t *testing.T) {
	for _, overwrite := range []OverwriteBehavior{OverwriteAlways, OverwriteNever} {
		t.Run(overwrite.String(), func(t *testing.T) {
			repo := repository.TestRepository(t)
			sn, _ := saveSnapshot(t, repo, Snapshot{Nodes: map[string]Node{
				"a": Symlink{Target: "b"},
				"b": Symlink{Target: "directory"},
				"directory": Dir{Nodes: map[string]Node{
					"file": File{Data: "restored content"},
				}},
			}}, noopGetGenericAttributes)
			target := t.TempDir()
			rtest.OK(t, os.WriteFile(filepath.Join(target, "existing-file"), []byte("existing content"), 0o600))
			rtest.OK(t, os.Symlink("existing-file", filepath.Join(target, "b")))
			res := NewRestorer(repo, sn, Options{Overwrite: overwrite})
			_, err := res.RestoreTo(t.Context(), target)
			rtest.OK(t, err)
			actual, err := os.Readlink(filepath.Join(target, "b"))
			rtest.OK(t, err)
			if overwrite == OverwriteNever {
				rtest.Equals(t, "existing-file", actual)
				content, err := os.ReadFile(filepath.Join(target, "a"))
				rtest.OK(t, err)
				rtest.Equals(t, "existing content", string(content))
			} else {
				rtest.Equals(t, "directory", actual)
				entries, err := os.ReadDir(filepath.Join(target, "a"))
				rtest.OK(t, err)
				rtest.Equals(t, 1, len(entries))
				content, err := os.ReadFile(filepath.Join(target, "a", "file"))
				rtest.OK(t, err)
				rtest.Equals(t, "restored content", string(content))
			}
		})
	}
}

func TestRestorerSymlinkRepeatedExistingComponent(t *testing.T) {
	repo := repository.TestRepository(t)
	sn, _ := saveSnapshot(t, repo, Snapshot{Nodes: map[string]Node{
		"a":         Symlink{Target: filepath.FromSlash("bridge/bridge/z")},
		"z":         Symlink{Target: "directory"},
		"directory": Dir{Nodes: map[string]Node{"file": File{Data: "content"}}},
	}}, noopGetGenericAttributes)
	target := t.TempDir()
	rtest.OK(t, os.Symlink(".", filepath.Join(target, "bridge")))
	res := NewRestorer(repo, sn, Options{})
	_, err := res.RestoreTo(t.Context(), target)
	rtest.OK(t, err)
	entries, err := os.ReadDir(filepath.Join(target, "a"))
	rtest.OK(t, err)
	rtest.Equals(t, 1, len(entries))
}

func TestRestorerSymlinkLongPath(t *testing.T) {
	repo := repository.TestRepository(t)
	sn, _ := saveSnapshot(t, repo, Snapshot{Nodes: map[string]Node{
		"links":     Dir{Nodes: map[string]Node{"a": Symlink{Target: filepath.FromSlash("../z")}}},
		"z":         Symlink{Target: "directory"},
		"directory": Dir{Nodes: map[string]Node{"file": File{Data: "content"}}},
	}}, noopGetGenericAttributes)
	target := filepath.Join(t.TempDir(), strings.Repeat("long-path", 16), strings.Repeat("long-path", 16))
	res := NewRestorer(repo, sn, Options{})
	_, err := res.RestoreTo(t.Context(), target)
	rtest.OK(t, err)
	linkPath := filepath.Join(target, "links", "a")
	rtest.Assert(t, len(linkPath) > 260, "test path must exceed MAX_PATH")
	actual, err := os.Readlink(linkPath)
	rtest.OK(t, err)
	rtest.Equals(t, filepath.FromSlash("../z"), actual)
	entries, err := os.ReadDir(linkPath)
	rtest.OK(t, err)
	rtest.Equals(t, 1, len(entries))
}

func TestRestorerSymlinkSelection(t *testing.T) {
	repo := repository.TestRepository(t)
	sn, _ := saveSnapshot(t, repo, Snapshot{Nodes: map[string]Node{
		"a":         Symlink{Target: "b"},
		"b":         Symlink{Target: "directory"},
		"directory": Dir{},
	}}, noopGetGenericAttributes)
	res := NewRestorer(repo, sn, Options{})
	res.SelectFilter = func(item string, _ bool) (bool, bool) {
		return filepath.Base(item) == "a", false
	}
	target := t.TempDir()
	_, err := res.RestoreTo(t.Context(), target)
	rtest.OK(t, err)
	actual, err := os.Readlink(filepath.Join(target, "a"))
	rtest.OK(t, err)
	rtest.Equals(t, "b", actual)
	for _, name := range []string{"b", "directory"} {
		_, err := os.Lstat(filepath.Join(target, name))
		rtest.Assert(t, errors.Is(err, os.ErrNotExist), "excluded path %q was created: %v", name, err)
	}
}

func TestRestorerSymlinkDependencyError(t *testing.T) {
	repo := repository.TestRepository(t)
	sn, _ := saveSnapshot(t, repo, Snapshot{Nodes: map[string]Node{
		"a":         Symlink{Target: "b"},
		"b":         Symlink{Target: "directory"},
		"directory": Dir{},
	}}, noopGetGenericAttributes)
	target := t.TempDir()
	rtest.OK(t, os.Mkdir(filepath.Join(target, "b"), 0o700))
	rtest.OK(t, os.WriteFile(filepath.Join(target, "b", "existing"), []byte("keep"), 0o600))
	res := NewRestorer(repo, sn, Options{})
	var failedLocations []string
	res.Error = func(location string, _ error) error {
		failedLocations = append(failedLocations, location)
		return nil
	}
	_, err := res.RestoreTo(t.Context(), target)
	rtest.OK(t, err)
	rtest.Equals(t, []string{string(filepath.Separator) + "b"}, failedLocations)
	actual, err := os.Readlink(filepath.Join(target, "a"))
	rtest.OK(t, err)
	rtest.Equals(t, "b", actual)
	content, err := os.ReadFile(filepath.Join(target, "b", "existing"))
	rtest.OK(t, err)
	rtest.Equals(t, "keep", string(content))
}

func TestRestorerSymlinkDryRun(t *testing.T) {
	repo := repository.TestRepository(t)
	sn, _ := saveSnapshot(t, repo, Snapshot{Nodes: map[string]Node{
		"a":         Symlink{Target: "b"},
		"b":         Symlink{Target: "directory"},
		"directory": Dir{},
	}}, noopGetGenericAttributes)
	target := t.TempDir()
	rtest.OK(t, os.WriteFile(filepath.Join(target, "existing-file"), []byte("existing content"), 0o600))
	rtest.OK(t, os.Symlink("existing-file", filepath.Join(target, "b")))
	res := NewRestorer(repo, sn, Options{DryRun: true})
	_, err := res.RestoreTo(t.Context(), target)
	rtest.OK(t, err)
	actual, err := os.Readlink(filepath.Join(target, "b"))
	rtest.OK(t, err)
	rtest.Equals(t, "existing-file", actual)
	content, err := os.ReadFile(filepath.Join(target, "b"))
	rtest.OK(t, err)
	rtest.Equals(t, "existing content", string(content))
	entries, err := os.ReadDir(target)
	rtest.OK(t, err)
	rtest.Equals(t, 2, len(entries))
}

func TestRestorerSymlinkDirectoryMetadata(t *testing.T) {
	mtime := time.Unix(1700000000, 0)
	repo := repository.TestRepository(t)
	sn, _ := saveSnapshot(t, repo, Snapshot{Nodes: map[string]Node{
		"links": Dir{ModTime: mtime, Nodes: map[string]Node{
			"a": Symlink{Target: filepath.FromSlash("../z")},
		}},
		"z":         Symlink{Target: "directory"},
		"directory": Dir{},
	}}, noopGetGenericAttributes)
	target := t.TempDir()
	res := NewRestorer(repo, sn, Options{})
	_, err := res.RestoreTo(t.Context(), target)
	rtest.OK(t, err)
	_, err = os.ReadDir(filepath.Join(target, "links", "a"))
	rtest.OK(t, err)
	fi, err := os.Stat(filepath.Join(target, "links"))
	rtest.OK(t, err)
	rtest.Assert(t, mtime.Equal(fi.ModTime()), "directory mtime changed: want %v, got %v", mtime, fi.ModTime())
}

func TestRestorerSymlinkDanglingAndCycle(t *testing.T) {
	repo := repository.TestRepository(t)
	links := map[string]string{"a": "b", "b": "a", "dangling": "missing"}
	nodes := make(map[string]Node)
	for name, target := range links {
		nodes[name] = Symlink{Target: target}
	}
	sn, _ := saveSnapshot(t, repo, Snapshot{Nodes: nodes}, noopGetGenericAttributes)
	res := NewRestorer(repo, sn, Options{})
	// Some platforms cannot restore metadata on a cyclic symlink. Keep the
	// existing error behavior, but ensure a cycle does not prevent termination.
	res.Error = func(location string, err error) error {
		t.Logf("restore error for %q: %v", location, err)
		return nil
	}
	target := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err := res.RestoreTo(ctx, target)
	rtest.OK(t, err)
	rtest.OK(t, ctx.Err())
	for name, expected := range links {
		actual, err := os.Readlink(filepath.Join(target, name))
		if name != "dangling" && errors.Is(err, os.ErrNotExist) {
			continue
		}
		rtest.OK(t, err)
		rtest.Equals(t, expected, actual)
	}
}
