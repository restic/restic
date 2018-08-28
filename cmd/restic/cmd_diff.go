package main

import (
	"context"
	"path"
	"reflect"
	"sort"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/spf13/cobra"
)

var cmdDiff = &cobra.Command{
	Use:   "diff snapshot-ID snapshot-ID",
	Short: "Show differences between two snapshots",
	Long: `
The "diff" command shows differences from the first to the second snapshot. The
first characters in each line display what has happened to a particular file or
directory:

* +  The item was added
* -  The item was removed
* U  The metadata (access mode, timestamps, ...) for the item was updated
* M  The file's content was modified
* T  The type was changed, e.g. a file was made a symlink
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDiff(diffOptions, globalOptions, args)
	},
}

// DiffOptions collects all options for the diff command.
type DiffOptions struct {
	ShowMetadata bool
}

var diffOptions DiffOptions

func init() {
	cmdRoot.AddCommand(cmdDiff)

	f := cmdDiff.Flags()
	f.BoolVar(&diffOptions.ShowMetadata, "metadata", false, "print changes in metadata")
}

func loadSnapshot(ctx context.Context, repo *repository.Repository, desc string) (*restic.Snapshot, error) {
	id, err := restic.FindSnapshot(repo, desc)
	if err != nil {
		return nil, err
	}

	return restic.LoadSnapshot(ctx, repo, id)
}

// Comparer collects all things needed to compare two snapshots.
type Comparer struct {
	repo restic.Repository
	opts DiffOptions
}

// DiffStat collects stats for all types of items.
type DiffStat struct {
	Files, Dirs, Others  int
	DataBlobs, TreeBlobs int
	Bytes                int
}

// Add adds stats information for node to s.
func (s *DiffStat) Add(node *restic.Node) {
	if node == nil {
		return
	}

	switch node.Type {
	case "file":
		s.Files++
	case "dir":
		s.Dirs++
	default:
		s.Others++
	}
}

// addBlobs adds the blobs of node to s.
func addBlobs(bs restic.BlobSet, node *restic.Node) {
	if node == nil {
		return
	}

	switch node.Type {
	case "file":
		for _, blob := range node.Content {
			h := restic.BlobHandle{
				ID:   blob,
				Type: restic.DataBlob,
			}
			bs.Insert(h)
		}
	case "dir":
		h := restic.BlobHandle{
			ID:   *node.Subtree,
			Type: restic.TreeBlob,
		}
		bs.Insert(h)
	}
}

// DiffStats collects the differences between two snapshots.
type DiffStats struct {
	ChangedFiles            int
	Added                   DiffStat
	Removed                 DiffStat
	BlobsBefore, BlobsAfter restic.BlobSet
}

// NewDiffStats creates new stats for a diff run.
func NewDiffStats() *DiffStats {
	return &DiffStats{
		BlobsBefore: restic.NewBlobSet(),
		BlobsAfter:  restic.NewBlobSet(),
	}
}

// updateBlobs updates the blob counters in the stats struct.
func updateBlobs(repo restic.Repository, blobs restic.BlobSet, stats *DiffStat) {
	for h := range blobs {
		switch h.Type {
		case restic.DataBlob:
			stats.DataBlobs++
		case restic.TreeBlob:
			stats.TreeBlobs++
		}

		size, found := repo.LookupBlobSize(h.ID, h.Type)
		if !found {
			Warnf("unable to find blob size for %v\n", h)
			continue
		}

		stats.Bytes += int(size)
	}
}

func (c *Comparer) printDir(ctx context.Context, mode string, stats *DiffStat, blobs restic.BlobSet, prefix string, id restic.ID) error {
	debug.Log("print %v tree %v", mode, id)
	tree, err := c.repo.LoadTree(ctx, id)
	if err != nil {
		return err
	}

	for _, node := range tree.Nodes {
		name := path.Join(prefix, node.Name)
		if node.Type == "dir" {
			name += "/"
		}
		Printf("%-5s%v\n", mode, name)
		stats.Add(node)
		addBlobs(blobs, node)

		if node.Type == "dir" {
			err := c.printDir(ctx, mode, stats, blobs, name, *node.Subtree)
			if err != nil {
				Warnf("error: %v\n", err)
			}
		}
	}

	return nil
}

func uniqueNodeNames(tree1, tree2 *restic.Tree) (tree1Nodes, tree2Nodes map[string]*restic.Node, uniqueNames []string) {
	names := make(map[string]struct{})
	tree1Nodes = make(map[string]*restic.Node)
	for _, node := range tree1.Nodes {
		tree1Nodes[node.Name] = node
		names[node.Name] = struct{}{}
	}

	tree2Nodes = make(map[string]*restic.Node)
	for _, node := range tree2.Nodes {
		tree2Nodes[node.Name] = node
		names[node.Name] = struct{}{}
	}

	uniqueNames = make([]string, 0, len(names))
	for name := range names {
		uniqueNames = append(uniqueNames, name)
	}

	sort.Sort(sort.StringSlice(uniqueNames))
	return tree1Nodes, tree2Nodes, uniqueNames
}

func (c *Comparer) diffTree(ctx context.Context, stats *DiffStats, prefix string, id1, id2 restic.ID) error {
	debug.Log("diffing %v to %v", id1, id2)
	tree1, err := c.repo.LoadTree(ctx, id1)
	if err != nil {
		return err
	}

	tree2, err := c.repo.LoadTree(ctx, id2)
	if err != nil {
		return err
	}

	tree1Nodes, tree2Nodes, names := uniqueNodeNames(tree1, tree2)

	for _, name := range names {
		node1, t1 := tree1Nodes[name]
		node2, t2 := tree2Nodes[name]

		addBlobs(stats.BlobsBefore, node1)
		addBlobs(stats.BlobsAfter, node2)

		switch {
		case t1 && t2:
			name := path.Join(prefix, name)
			mod := ""

			if node1.Type != node2.Type {
				mod += "T"
			}

			if node2.Type == "dir" {
				name += "/"
			}

			if node1.Type == "file" &&
				node2.Type == "file" &&
				!reflect.DeepEqual(node1.Content, node2.Content) {
				mod += "M"
				stats.ChangedFiles++
			} else if c.opts.ShowMetadata && !node1.Equals(*node2) {
				mod += "U"
			}

			if mod != "" {
				Printf("%-5s%v\n", mod, name)
			}

			if node1.Type == "dir" && node2.Type == "dir" {
				err := c.diffTree(ctx, stats, name, *node1.Subtree, *node2.Subtree)
				if err != nil {
					Warnf("error: %v\n", err)
				}
			}
		case t1 && !t2:
			prefix := path.Join(prefix, name)
			if node1.Type == "dir" {
				prefix += "/"
			}
			Printf("%-5s%v\n", "-", prefix)
			stats.Removed.Add(node1)

			if node1.Type == "dir" {
				err := c.printDir(ctx, "-", &stats.Removed, stats.BlobsBefore, prefix, *node1.Subtree)
				if err != nil {
					Warnf("error: %v\n", err)
				}
			}
		case !t1 && t2:
			prefix := path.Join(prefix, name)
			if node2.Type == "dir" {
				prefix += "/"
			}
			Printf("%-5s%v\n", "+", prefix)
			stats.Added.Add(node2)

			if node2.Type == "dir" {
				err := c.printDir(ctx, "+", &stats.Added, stats.BlobsAfter, prefix, *node2.Subtree)
				if err != nil {
					Warnf("error: %v\n", err)
				}
			}
		}
	}

	return nil
}

func runDiff(opts DiffOptions, gopts GlobalOptions, args []string) error {
	if len(args) != 2 {
		return errors.Fatalf("specify two snapshot IDs")
	}

	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if err = repo.LoadIndex(ctx); err != nil {
		return err
	}

	if !gopts.NoLock {
		lock, err := lockRepo(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	sn1, err := loadSnapshot(ctx, repo, args[0])
	if err != nil {
		return err
	}

	sn2, err := loadSnapshot(ctx, repo, args[1])
	if err != nil {
		return err
	}

	Verbosef("comparing snapshot %v to %v:\n\n", sn1.ID().Str(), sn2.ID().Str())

	if sn1.Tree == nil {
		return errors.Errorf("snapshot %v has nil tree", sn1.ID().Str())
	}

	if sn2.Tree == nil {
		return errors.Errorf("snapshot %v has nil tree", sn2.ID().Str())
	}

	c := &Comparer{
		repo: repo,
		opts: diffOptions,
	}

	stats := NewDiffStats()

	err = c.diffTree(ctx, stats, "/", *sn1.Tree, *sn2.Tree)
	if err != nil {
		return err
	}

	both := stats.BlobsBefore.Intersect(stats.BlobsAfter)
	updateBlobs(repo, stats.BlobsBefore.Sub(both), &stats.Removed)
	updateBlobs(repo, stats.BlobsAfter.Sub(both), &stats.Added)

	Printf("\n")
	Printf("Files:       %5d new, %5d removed, %5d changed\n", stats.Added.Files, stats.Removed.Files, stats.ChangedFiles)
	Printf("Dirs:        %5d new, %5d removed\n", stats.Added.Dirs, stats.Removed.Dirs)
	Printf("Others:      %5d new, %5d removed\n", stats.Added.Others, stats.Removed.Others)
	Printf("Data Blobs:  %5d new, %5d removed\n", stats.Added.DataBlobs, stats.Removed.DataBlobs)
	Printf("Tree Blobs:  %5d new, %5d removed\n", stats.Added.TreeBlobs, stats.Removed.TreeBlobs)
	Printf("  Added:   %-5s\n", formatBytes(uint64(stats.Added.Bytes)))
	Printf("  Removed: %-5s\n", formatBytes(uint64(stats.Removed.Bytes)))

	return nil
}
