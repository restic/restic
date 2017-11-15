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

 +  The item was added
 -  The item was removed
 M  The metadata (access mode, timestamps, ...) for the item was changed
 C  The contents of a file has changed
 T  The type was changed, e.g. a file was made a symlink
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

// DiffStats collects the differences between two snapshots.
type DiffStats struct {
	FilesAdded, FilesRemoved, FilesChanged int
	DirsAdded, DirsRemoved                 int
	OthersAdded, OthersRemoved             int
	DataBlobsAdded, DataBlobsRemoved       int
	TreeBlobsAdded, TreeBlobsRemoved       int
	BytesAdded, BytesRemoved               int

	blobsBefore, blobsAfter restic.BlobSet
}

// NewDiffStats creates new stats for a diff run.
func NewDiffStats() *DiffStats {
	return &DiffStats{
		blobsBefore: restic.NewBlobSet(),
		blobsAfter:  restic.NewBlobSet(),
	}
}

// AddNodeBefore records all blobs of node to the stats of the first snapshot.
func (stats *DiffStats) AddNodeBefore(node *restic.Node) {
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
			stats.blobsBefore.Insert(h)
		}
	case "dir":
		h := restic.BlobHandle{
			ID:   *node.Subtree,
			Type: restic.TreeBlob,
		}
		stats.blobsBefore.Insert(h)
	}
}

// AddNodeAfter records all blobs of node to the stats of the second snapshot.
func (stats *DiffStats) AddNodeAfter(node *restic.Node) {
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
			stats.blobsAfter.Insert(h)
		}
	case "dir":
		h := restic.BlobHandle{
			ID:   *node.Subtree,
			Type: restic.TreeBlob,
		}
		stats.blobsAfter.Insert(h)
	}
}

// UpdateBlobs updates the blob counters in the stats struct.
func (stats *DiffStats) UpdateBlobs(repo restic.Repository) {
	both := stats.blobsBefore.Intersect(stats.blobsAfter)
	for h := range stats.blobsBefore.Sub(both) {
		switch h.Type {
		case restic.DataBlob:
			stats.DataBlobsRemoved++
		case restic.TreeBlob:
			stats.TreeBlobsRemoved++
		}

		size, err := repo.LookupBlobSize(h.ID, h.Type)
		if err != nil {
			Warnf("unable to find blob size for %v: %v\n", h, err)
			continue
		}

		stats.BytesRemoved += int(size)
	}

	for h := range stats.blobsAfter.Sub(both) {
		switch h.Type {
		case restic.DataBlob:
			stats.DataBlobsAdded++
		case restic.TreeBlob:
			stats.TreeBlobsAdded++
		}

		size, err := repo.LookupBlobSize(h.ID, h.Type)
		if err != nil {
			Warnf("unable to find blob size for %v: %v\n", h, err)
			continue
		}

		stats.BytesAdded += int(size)
	}
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

	uniqueNames := make(map[string]struct{})
	tree1Nodes := make(map[string]*restic.Node)
	for _, node := range tree1.Nodes {
		tree1Nodes[node.Name] = node
		uniqueNames[node.Name] = struct{}{}
	}
	tree2Nodes := make(map[string]*restic.Node)
	for _, node := range tree2.Nodes {
		tree2Nodes[node.Name] = node
		uniqueNames[node.Name] = struct{}{}
	}

	names := make([]string, 0, len(uniqueNames))
	for name := range uniqueNames {
		names = append(names, name)
	}

	sort.Sort(sort.StringSlice(names))

	for _, name := range names {
		node1, t1 := tree1Nodes[name]
		node2, t2 := tree2Nodes[name]

		stats.AddNodeBefore(node1)
		stats.AddNodeAfter(node2)

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
				mod += "C"
				stats.FilesChanged++

				if c.opts.ShowMetadata && !node1.Equals(*node2) {
					mod += "M"
				}
			} else if c.opts.ShowMetadata && !node1.Equals(*node2) {
				mod += "M"
			}

			if mod != "" {
				Printf(" % -3v %v\n", mod, name)
			}

			if node1.Type == "dir" && node2.Type == "dir" {
				err := c.diffTree(ctx, stats, name, *node1.Subtree, *node2.Subtree)
				if err != nil {
					Warnf("error: %v\n", err)
				}
			}
		case t1 && !t2:
			Printf("-    %v\n", path.Join(prefix, name))
			switch node1.Type {
			case "file":
				stats.FilesRemoved++
			case "dir":
				stats.DirsRemoved++
			default:
				stats.OthersRemoved++
			}
		case !t1 && t2:
			Printf("+    %v\n", path.Join(prefix, name))
			switch node2.Type {
			case "file":
				stats.FilesAdded++
			case "dir":
				stats.DirsAdded++
			default:
				stats.OthersAdded++
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

	stats.UpdateBlobs(repo)

	Printf("\n")
	Printf("Files:       %5d new, %5d removed, %5d changed\n", stats.FilesAdded, stats.FilesRemoved, stats.FilesChanged)
	Printf("Dirs:        %5d new, %5d removed\n", stats.DirsAdded, stats.DirsRemoved)
	Printf("Others:      %5d new, %5d removed\n", stats.OthersAdded, stats.OthersRemoved)
	Printf("Data Blobs:  %5d new, %5d removed\n", stats.DataBlobsAdded, stats.DataBlobsRemoved)
	Printf("Tree Blobs:  %5d new, %5d removed\n", stats.TreeBlobsAdded, stats.TreeBlobsRemoved)
	Printf("  Added:   %-5s\n", formatBytes(uint64(stats.BytesAdded)))
	Printf("  Removed: %-5s\n", formatBytes(uint64(stats.BytesRemoved)))

	return nil
}
