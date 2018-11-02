package main

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
)

var cmdFind = &cobra.Command{
	Use:   "find [flags] PATTERN...",
	Short: "Find a file, a directory or restic IDs",
	Long: `
The "find" command searches for files or directories in snapshots stored in the
repo.
It can also be used to search for restic blobs or trees for troubleshooting.`,
	Example: `restic find config.json
restic find --json "*.yml" "*.json"
restic find --json --blob 420f620f b46ebe8a ddd38656
restic find --show-pack-id --blob 420f620f
restic find --tree 577c2bc9 f81f2e22 a62827a9
restic find --pack 025c1d06`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runFind(findOptions, globalOptions, args)
	},
}

const shortStr = 8 // Length of short IDs: 4 bytes as hex strings

// FindOptions bundles all options for the find command.
type FindOptions struct {
	Oldest             string
	Newest             string
	Snapshots          []string
	BlobID, TreeID     bool
	PackID, ShowPackID bool
	CaseInsensitive    bool
	ListLong           bool
	Host               string
	Paths              []string
	Tags               restic.TagLists
}

var findOptions FindOptions

func init() {
	cmdRoot.AddCommand(cmdFind)

	f := cmdFind.Flags()
	f.StringVarP(&findOptions.Oldest, "oldest", "O", "", "oldest modification date/time")
	f.StringVarP(&findOptions.Newest, "newest", "N", "", "newest modification date/time")
	f.StringArrayVarP(&findOptions.Snapshots, "snapshot", "s", nil, "snapshot `id` to search in (can be given multiple times)")
	f.BoolVar(&findOptions.BlobID, "blob", false, "pattern is a blob-ID")
	f.BoolVar(&findOptions.TreeID, "tree", false, "pattern is a tree-ID")
	f.BoolVar(&findOptions.PackID, "pack", false, "pattern is a pack-ID")
	f.BoolVar(&findOptions.ShowPackID, "show-pack-id", false, "display the pack-ID the blobs belong to (with --blob)")
	f.BoolVarP(&findOptions.CaseInsensitive, "ignore-case", "i", false, "ignore case for pattern")
	f.BoolVarP(&findOptions.ListLong, "long", "l", false, "use a long listing format showing size and mode")

	f.StringVarP(&findOptions.Host, "host", "H", "", "only consider snapshots for this `host`, when no snapshot ID is given")
	f.Var(&findOptions.Tags, "tag", "only consider snapshots which include this `taglist`, when no snapshot-ID is given")
	f.StringArrayVar(&findOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path`, when no snapshot-ID is given")
}

type findPattern struct {
	oldest, newest time.Time
	pattern        []string
	ignoreCase     bool
}

var timeFormats = []string{
	"2006-01-02",
	"2006-01-02 15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05 MST",
	"02.01.2006",
	"02.01.2006 15:04",
	"02.01.2006 15:04:05",
	"02.01.2006 15:04:05 -0700",
	"02.01.2006 15:04:05 MST",
	"Mon Jan 2 15:04:05 -0700 MST 2006",
}

func parseTime(str string) (time.Time, error) {
	for _, fmt := range timeFormats {
		if t, err := time.ParseInLocation(fmt, str, time.Local); err == nil {
			return t, nil
		}
	}

	return time.Time{}, errors.Fatalf("unable to parse time: %q", str)
}

type statefulOutput struct {
	ListLong bool
	JSON     bool
	inuse    bool
	newsn    *restic.Snapshot
	oldsn    *restic.Snapshot
	hits     int
}

func (s *statefulOutput) PrintPatternJSON(path string, node *restic.Node) {
	type findNode restic.Node
	b, err := json.Marshal(struct {
		// Add these attributes
		Path        string `json:"path,omitempty"`
		Permissions string `json:"permissions,omitempty"`

		*findNode

		// Make the following attributes disappear
		Name               byte `json:"name,omitempty"`
		Inode              byte `json:"inode,omitempty"`
		ExtendedAttributes byte `json:"extended_attributes,omitempty"`
		Device             byte `json:"device,omitempty"`
		Content            byte `json:"content,omitempty"`
		Subtree            byte `json:"subtree,omitempty"`
	}{
		Path:        path,
		Permissions: node.Mode.String(),
		findNode:    (*findNode)(node),
	})
	if err != nil {
		Warnf("Marshall failed: %v\n", err)
		return
	}
	if !s.inuse {
		Printf("[")
		s.inuse = true
	}
	if s.newsn != s.oldsn {
		if s.oldsn != nil {
			Printf("],\"hits\":%d,\"snapshot\":%q},", s.hits, s.oldsn.ID())
		}
		Printf(`{"matches":[`)
		s.oldsn = s.newsn
		s.hits = 0
	}
	if s.hits > 0 {
		Printf(",")
	}
	Printf(string(b))
	s.hits++
}

func (s *statefulOutput) PrintPatternNormal(path string, node *restic.Node) {
	if s.newsn != s.oldsn {
		if s.oldsn != nil {
			Verbosef("\n")
		}
		s.oldsn = s.newsn
		Verbosef("Found matching entries in snapshot %s\n", s.oldsn.ID().Str())
	}
	Printf(formatNode(path, node, s.ListLong) + "\n")
}

func (s *statefulOutput) PrintPattern(path string, node *restic.Node) {
	if s.JSON {
		s.PrintPatternJSON(path, node)
	} else {
		s.PrintPatternNormal(path, node)
	}
}

func (s *statefulOutput) PrintObjectJSON(kind, id, nodepath, treeID string, sn *restic.Snapshot) {
	b, err := json.Marshal(struct {
		// Add these attributes
		ObjectType string    `json:"object_type"`
		ID         string    `json:"id"`
		Path       string    `json:"path"`
		ParentTree string    `json:"parent_tree,omitempty"`
		SnapshotID string    `json:"snapshot"`
		Time       time.Time `json:"time,omitempty"`
	}{
		ObjectType: kind,
		ID:         id,
		Path:       nodepath,
		SnapshotID: sn.ID().String(),
		ParentTree: treeID,
		Time:       sn.Time,
	})
	if err != nil {
		Warnf("Marshall failed: %v\n", err)
		return
	}
	if !s.inuse {
		Printf("[")
		s.inuse = true
	}
	if s.hits > 0 {
		Printf(",")
	}
	Printf(string(b))
	s.hits++
}

func (s *statefulOutput) PrintObjectNormal(kind, id, nodepath, treeID string, sn *restic.Snapshot) {
	Printf("Found %s %s\n", kind, id)
	if kind == "blob" {
		Printf(" ... in file %s\n", nodepath)
		Printf("     (tree %s)\n", treeID)
	} else {
		Printf(" ... path %s\n", nodepath)
	}
	Printf(" ... in snapshot %s (%s)\n", sn.ID().Str(), sn.Time.Local().Format(TimeFormat))
}

func (s *statefulOutput) PrintObject(kind, id, nodepath, treeID string, sn *restic.Snapshot) {
	if s.JSON {
		s.PrintObjectJSON(kind, id, nodepath, treeID, sn)
	} else {
		s.PrintObjectNormal(kind, id, nodepath, treeID, sn)
	}
}

func (s *statefulOutput) Finish() {
	if s.JSON {
		// do some finishing up
		if s.oldsn != nil {
			Printf("],\"hits\":%d,\"snapshot\":%q}", s.hits, s.oldsn.ID())
		}
		if s.inuse {
			Printf("]\n")
		} else {
			Printf("[]\n")
		}
		return
	}
}

// Finder bundles information needed to find a file or directory.
type Finder struct {
	repo        restic.Repository
	pat         findPattern
	out         statefulOutput
	ignoreTrees restic.IDSet
	blobIDs     map[string]struct{}
	treeIDs     map[string]struct{}
	itemsFound  int
}

func (f *Finder) findInSnapshot(ctx context.Context, sn *restic.Snapshot) error {
	debug.Log("searching in snapshot %s\n  for entries within [%s %s]", sn.ID(), f.pat.oldest, f.pat.newest)

	if sn.Tree == nil {
		return errors.Errorf("snapshot %v has no tree", sn.ID().Str())
	}

	f.out.newsn = sn
	return walker.Walk(ctx, f.repo, *sn.Tree, f.ignoreTrees, func(_ restic.ID, nodepath string, node *restic.Node, err error) (bool, error) {
		if err != nil {
			return false, err
		}

		if node == nil {
			return false, nil
		}

		normalizedNodepath := nodepath
		if f.pat.ignoreCase {
			normalizedNodepath = strings.ToLower(nodepath)
		}

		var foundMatch bool

		for _, pat := range f.pat.pattern {
			found, err := filter.Match(pat, normalizedNodepath)
			if err != nil {
				return false, err
			}
			if found {
				foundMatch = true
				break
			}
		}

		var (
			ignoreIfNoMatch = true
			errIfNoMatch    error
		)
		if node.Type == "dir" {
			var childMayMatch bool
			for _, pat := range f.pat.pattern {
				mayMatch, err := filter.ChildMatch(pat, normalizedNodepath)
				if err != nil {
					return false, err
				}
				if mayMatch {
					childMayMatch = true
					break
				}
			}

			if !childMayMatch {
				ignoreIfNoMatch = true
				errIfNoMatch = walker.SkipNode
			} else {
				ignoreIfNoMatch = false
			}
		}

		if !foundMatch {
			return ignoreIfNoMatch, errIfNoMatch
		}

		if !f.pat.oldest.IsZero() && node.ModTime.Before(f.pat.oldest) {
			debug.Log("    ModTime is older than %s\n", f.pat.oldest)
			return ignoreIfNoMatch, errIfNoMatch
		}

		if !f.pat.newest.IsZero() && node.ModTime.After(f.pat.newest) {
			debug.Log("    ModTime is newer than %s\n", f.pat.newest)
			return ignoreIfNoMatch, errIfNoMatch
		}

		debug.Log("    found match\n")
		f.out.PrintPattern(nodepath, node)
		return false, nil
	})
}

func (f *Finder) findIDs(ctx context.Context, sn *restic.Snapshot) error {
	debug.Log("searching IDs in snapshot %s", sn.ID())

	if sn.Tree == nil {
		return errors.Errorf("snapshot %v has no tree", sn.ID().Str())
	}

	f.out.newsn = sn
	return walker.Walk(ctx, f.repo, *sn.Tree, f.ignoreTrees, func(parentTreeID restic.ID, nodepath string, node *restic.Node, err error) (bool, error) {
		if err != nil {
			return false, err
		}

		if node == nil {
			return false, nil
		}

		if node.Type == "dir" && f.treeIDs != nil {
			treeID := node.Subtree
			found := false
			if _, ok := f.treeIDs[treeID.Str()]; ok {
				found = true
			} else if _, ok := f.treeIDs[treeID.String()]; ok {
				found = true
			}
			if found {
				f.out.PrintObject("tree", treeID.String(), nodepath, "", sn)
				f.itemsFound++
				// Terminate if we have found all trees (and we are not
				// looking for blobs)
				if f.itemsFound >= len(f.treeIDs) && f.blobIDs == nil {
					// Return an error to terminate the Walk
					return true, errors.New("OK")
				}
			}
		}

		if node.Type == "file" && f.blobIDs != nil {
			for _, id := range node.Content {
				idStr := id.String()
				if _, ok := f.blobIDs[idStr]; !ok {
					// Look for short ID form
					if _, ok := f.blobIDs[idStr[:shortStr]]; !ok {
						continue
					}
					// Replace the short ID with the long one
					f.blobIDs[idStr] = struct{}{}
					delete(f.blobIDs, idStr[:shortStr])
				}
				f.out.PrintObject("blob", idStr, nodepath, parentTreeID.String(), sn)
				break
			}
		}

		return false, nil
	})
}

// packsToBlobs converts the list of pack IDs to a list of blob IDs that
// belong to those packs.
func (f *Finder) packsToBlobs(ctx context.Context, packs []string) error {
	packIDs := make(map[string]struct{})
	for _, p := range packs {
		packIDs[p] = struct{}{}
	}
	if f.blobIDs == nil {
		f.blobIDs = make(map[string]struct{})
	}

	allPacksFound := false
	packsFound := 0

	debug.Log("Looking for packs...")
	err := f.repo.List(ctx, restic.DataFile, func(id restic.ID, size int64) error {
		if allPacksFound {
			return nil
		}
		idStr := id.String()
		if _, ok := packIDs[idStr]; !ok {
			// Look for short ID form
			if _, ok := packIDs[idStr[:shortStr]]; !ok {
				return nil
			}
		}
		debug.Log("Found pack %s", idStr)
		blobs, _, err := f.repo.ListPack(ctx, id, size)
		if err != nil {
			return err
		}
		for _, b := range blobs {
			f.blobIDs[b.ID.String()] = struct{}{}
		}
		// Stop searching when all packs have been found
		packsFound++
		if packsFound >= len(packIDs) {
			allPacksFound = true
		}
		return nil
	})

	if err != nil {
		return err
	}

	if !allPacksFound {
		return errors.Fatal("unable to find all specified pack(s)")
	}

	debug.Log("%d blobs found", len(f.blobIDs))
	return nil
}

func (f *Finder) findBlobsPacks(ctx context.Context) {
	idx := f.repo.Index()
	for i := range f.blobIDs {
		rid, err := restic.ParseID(i)
		if err != nil {
			Printf("Note: cannot find pack for blob '%s', unable to parse ID: %v\n", i, err)
			continue
		}

		blobs, found := idx.Lookup(rid, restic.DataBlob)
		if !found {
			Printf("Blob %s not found in the index\n", rid.Str())
			continue
		}

		for _, b := range blobs {
			if b.ID.Equal(rid) {
				Printf("Blob belongs to pack %s\n ... Pack %s: %s\n", b.PackID, b.PackID.Str(), b.String())
				break
			}
		}
	}
}

func runFind(opts FindOptions, gopts GlobalOptions, args []string) error {
	if len(args) == 0 {
		return errors.Fatal("wrong number of arguments")
	}

	var err error
	pat := findPattern{pattern: args}
	if opts.CaseInsensitive {
		for i := range pat.pattern {
			pat.pattern[i] = strings.ToLower(pat.pattern[i])
		}
		pat.ignoreCase = true
	}

	if opts.Oldest != "" {
		if pat.oldest, err = parseTime(opts.Oldest); err != nil {
			return err
		}
	}

	if opts.Newest != "" {
		if pat.newest, err = parseTime(opts.Newest); err != nil {
			return err
		}
	}

	// Check at most only one kind of IDs is provided: currently we
	// can't mix types
	if (opts.BlobID && opts.TreeID) ||
		(opts.BlobID && opts.PackID) ||
		(opts.TreeID && opts.PackID) {
		return errors.Fatal("cannot have several ID types")
	}

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		lock, err := lockRepo(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	if err = repo.LoadIndex(gopts.ctx); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()

	f := &Finder{
		repo:        repo,
		pat:         pat,
		out:         statefulOutput{ListLong: opts.ListLong, JSON: globalOptions.JSON},
		ignoreTrees: restic.NewIDSet(),
	}

	if opts.BlobID {
		f.blobIDs = make(map[string]struct{})
		for _, pat := range f.pat.pattern {
			f.blobIDs[pat] = struct{}{}
		}
	}
	if opts.TreeID {
		f.treeIDs = make(map[string]struct{})
		for _, pat := range f.pat.pattern {
			f.treeIDs[pat] = struct{}{}
		}
	}

	if opts.PackID {
		f.packsToBlobs(ctx, []string{f.pat.pattern[0]}) // TODO: support multiple packs
	}

	for sn := range FindFilteredSnapshots(ctx, repo, opts.Host, opts.Tags, opts.Paths, opts.Snapshots) {
		if f.blobIDs != nil || f.treeIDs != nil {
			if err = f.findIDs(ctx, sn); err != nil && err.Error() != "OK" {
				return err
			}
			continue
		}
		if err = f.findInSnapshot(ctx, sn); err != nil {
			return err
		}
	}
	f.out.Finish()

	if opts.ShowPackID && f.blobIDs != nil {
		f.findBlobsPacks(ctx)
	}

	return nil
}
