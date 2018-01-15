package serve

import (
	"context"
	"os"
	"path"
	"sync"
	"time"

	"github.com/restic/restic/internal/errors"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/net/webdav"
)

// Config holds settings for the file system served.
type Config struct {
	Host  string
	Tags  []restic.TagList
	Paths []string
}

const snapshotFormat = "2006-01-02_150405"

// RepoFileSystem implements a read-only file system on top of a repositoy.
type RepoFileSystem struct {
	repo      restic.Repository
	lastCheck time.Time

	entries map[string]webdav.File
	m       sync.Mutex
}

// NewRepoFileSystem returns a new file system for the repo.
func NewRepoFileSystem(ctx context.Context, repo restic.Repository, cfg Config) (*RepoFileSystem, error) {
	snapshots := restic.FindFilteredSnapshots(ctx, repo, cfg.Host, cfg.Tags, cfg.Paths)

	lastcheck := time.Now()

	nodes := make([]*restic.Node, 0, len(snapshots))
	entries := make(map[string]webdav.File)

	for _, sn := range snapshots {
		name := sn.Time.Format(snapshotFormat)
		snFileInfo := virtFileInfo{
			name:    name,
			size:    0,
			mode:    0755 | os.ModeDir,
			modtime: sn.Time,
			isdir:   true,
		}

		if sn.Tree == nil {
			return nil, errors.Errorf("snapshot %v has nil tree", sn.ID().Str())
		}

		tree, err := repo.LoadTree(ctx, *sn.Tree)
		if err != nil {
			return nil, err
		}

		p := path.Join("/", name)
		entries[p] = &RepoDir{
			fi:    snFileInfo,
			nodes: tree.Nodes,
		}

		nodes = append(nodes, &restic.Node{
			Name: name,
			Type: "dir",
		})
	}

	entries["/"] = &RepoDir{
		nodes: nodes,
		fi: virtFileInfo{
			name:    "root",
			size:    0,
			mode:    0755 | os.ModeDir,
			modtime: lastcheck,
			isdir:   true,
		},
	}

	fs := &RepoFileSystem{
		repo:      repo,
		lastCheck: lastcheck,
		entries:   entries,
	}

	return fs, nil
}

// statically ensure that RepoFileSystem implements webdav.FileSystem
var _ webdav.FileSystem = &RepoFileSystem{}

// Mkdir creates a new directory, it is not available for RepoFileSystem.
func (fs *RepoFileSystem) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	return webdav.ErrForbidden
}

func (fs *RepoFileSystem) loadPath(ctx context.Context, name string) error {
	debug.Log("%v", name)

	fs.m.Lock()
	_, ok := fs.entries[name]
	fs.m.Unlock()
	if ok {
		return nil
	}

	dirname := path.Dir(name)
	if dirname == "/" {
		return nil
	}

	err := fs.loadPath(ctx, dirname)
	if err != nil {
		return err
	}

	entry, ok := fs.entries[dirname]
	if !ok {
		// loadPath did not succeed
		return nil
	}

	repodir, ok := entry.(*RepoDir)
	if !ok {
		return nil
	}

	filename := path.Base(name)
	for _, node := range repodir.nodes {
		if node.Name != filename {
			continue
		}

		debug.Log("found item %v :%v", filename, node)

		switch node.Type {
		case "dir":
			if node.Subtree == nil {
				return errors.Errorf("tree %v has nil tree", dirname)
			}

			tree, err := fs.repo.LoadTree(ctx, *node.Subtree)
			if err != nil {
				return err
			}

			newEntry := &RepoDir{
				fi:    fileInfoFromNode(node),
				nodes: tree.Nodes,
			}

			fs.m.Lock()
			fs.entries[name] = newEntry
			fs.m.Unlock()
		case "file":
			newEntry := &RepoFile{
				fi:   fileInfoFromNode(node),
				node: node,
			}
			fs.m.Lock()
			fs.entries[name] = newEntry
			fs.m.Unlock()
		}

		return nil
	}

	return nil
}

// OpenFile opens a file for reading.
func (fs *RepoFileSystem) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	name = path.Clean(name)
	debug.Log("%v", name)
	if flag != os.O_RDONLY {
		return nil, webdav.ErrForbidden
	}

	err := fs.loadPath(ctx, name)
	if err != nil {
		return nil, err
	}

	fs.m.Lock()
	entry, ok := fs.entries[name]
	fs.m.Unlock()
	if !ok {
		return nil, os.ErrNotExist
	}

	return entry, nil
}

// RemoveAll recursively removes files and directories, it is not available for RepoFileSystem.
func (fs *RepoFileSystem) RemoveAll(ctx context.Context, name string) error {
	debug.Log("%v", name)
	return webdav.ErrForbidden
}

// Rename renames files or directories, it is not available for RepoFileSystem.
func (fs *RepoFileSystem) Rename(ctx context.Context, oldName, newName string) error {
	debug.Log("%v -> %v", oldName, newName)
	return webdav.ErrForbidden
}

// Stat returns information on a file or directory.
func (fs *RepoFileSystem) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	name = path.Clean(name)
	err := fs.loadPath(ctx, name)
	if err != nil {
		return nil, err
	}

	fs.m.Lock()
	entry, ok := fs.entries[name]
	fs.m.Unlock()
	if !ok {
		debug.Log("%v not found", name)
		return nil, os.ErrNotExist
	}

	fi, err := entry.Stat()
	debug.Log("%v %v", name, fi)
	return fi, err
}
