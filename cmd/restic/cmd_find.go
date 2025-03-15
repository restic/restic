package main

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
)

func newFindCommand() *cobra.Command {
	var opts FindOptions

	cmd := &cobra.Command{
		Use:   "find [flags] PATTERN...",
		Short: "Find a file, a directory or restic IDs",
		Long: `
The "find" command searches for files or directories in snapshots stored in the
repo.
It can also be used to search for restic blobs or trees for troubleshooting.
The default sort option for the snapshots is youngest to oldest. To sort the
output from oldest to youngest specify --reverse.`,
		Example: `restic find config.json
restic find --json "*.yml" "*.json"
restic find --json --blob 420f620f b46ebe8a ddd38656
restic find --show-pack-id --blob 420f620f
restic find --tree 577c2bc9 f81f2e22 a62827a9
restic find --pack 025c1d06

restic find --oldest '2024-01-01 00:00:00' --newest '2024-12-31 23:59:59' testfile
find the the files with modifications times in the year 2024 which have a path component
including the string 'testfile'. Find searches in all filtered snapshots.
Use 'name*' for more generic searches.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
`,
		GroupID:           cmdGroupDefault,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFind(cmd.Context(), opts, globalOptions, args)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// FindOptions bundles all options for the find command.
type FindOptions struct {
	Oldest             string
	Newest             string
	Snapshots          []string
	BlobID, TreeID     bool
	PackID, ShowPackID bool
	CaseInsensitive    bool
	ListLong           bool
	HumanReadable      bool
	Reverse            bool
	JSON               bool
	restic.SnapshotFilter
}

func (opts *FindOptions) AddFlags(f *pflag.FlagSet) {
	f.StringVarP(&opts.Oldest, "oldest", "O", "", "oldest modification date/time")
	f.StringVarP(&opts.Newest, "newest", "N", "", "newest modification date/time")
	f.StringArrayVarP(&opts.Snapshots, "snapshot", "s", nil, "snapshot `id` to search in (can be given multiple times)")
	f.BoolVar(&opts.BlobID, "blob", false, "pattern is a blob-ID")
	f.BoolVar(&opts.TreeID, "tree", false, "pattern is a tree-ID")
	f.BoolVar(&opts.PackID, "pack", false, "pattern is a pack-ID")
	f.BoolVar(&opts.ShowPackID, "show-pack-id", false, "display the pack-ID the blobs belong to (with --blob or --tree)")
	f.BoolVarP(&opts.CaseInsensitive, "ignore-case", "i", false, "ignore case for pattern")
	f.BoolVarP(&opts.Reverse, "reverse", "R", false, "reverse sort order oldest to newest")
	f.BoolVarP(&opts.ListLong, "long", "l", false, "use a long listing format showing size and mode")
	f.BoolVar(&opts.HumanReadable, "human-readable", false, "print sizes in human readable format")

	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)
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

type printBuffer struct {
	ObjectType string    `json:"object_type"`
	ID         restic.ID `json:"id"`
	Path       string    `json:"path"`
	ParentTree restic.ID `json:"parent_tree,omitempty"`
	SnapshotID string    `json:"snapshot"`
	Time       time.Time `json:"time,omitempty"`
	PackID     restic.ID `json:"packid,omitempty"`
	position   int
	// pack information from restic.PackedBlob
	Length             uint `json:"length,omitempty"`
	Offset             uint `json:"offset,omitempty"`
	UncompressedLength uint `json:"uncompressed_length,omitempty"`
}

type statefulOutput struct {
	ListLong      bool
	HumanReadable bool
	JSON          bool
	inuse         bool
	newsn         *restic.Snapshot
	oldsn         *restic.Snapshot
	hits          int
	// for catching packfile ID
	entryCounter int
	buffer       map[restic.ID]printBuffer
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
		ExtendedAttributes byte `json:"extended_attributes,omitempty"`
		GenericAttributes  byte `json:"generic_attributes,omitempty"`
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
	Print(string(b))
	s.hits++
}

func (s *statefulOutput) PrintPatternNormal(path string, node *restic.Node) {
	if s.newsn != s.oldsn {
		if s.oldsn != nil {
			Verbosef("\n")
		}
		s.oldsn = s.newsn
		Verbosef("Found matching entries in snapshot %s from %s\n", s.oldsn.ID().Str(), s.oldsn.Time.Local().Format(TimeFormat))
	}
	Println(formatNode(path, node, s.ListLong, s.HumanReadable))
}

func (s *statefulOutput) PrintPattern(path string, node *restic.Node) {
	if s.JSON {
		s.PrintPatternJSON(path, node)
	} else {
		s.PrintPatternNormal(path, node)
	}
}

// PrintObject just collects all its reocrds in a map: 'position' keep track of
// the original order in which the entries have been inserted
func (s *statefulOutput) PrintObject(kind string, id restic.ID, nodepath string, treeID restic.ID, sn *restic.Snapshot, bp restic.PackedBlob) {
	s.buffer[id] = printBuffer{
		ObjectType:         kind,
		Path:               nodepath,
		ParentTree:         treeID,
		SnapshotID:         sn.ID().String(),
		Time:               sn.Time,
		position:           s.entryCounter,
		Length:             bp.Length,
		PackID:             bp.PackID,
		UncompressedLength: bp.UncompressedLength,
		Offset:             bp.Offset,
	}
	s.entryCounter++
}

func (s *statefulOutput) Finish(opts FindOptions, f *Finder) {
	// finish up the rest from find <pattern>
	if s.JSON && !(opts.BlobID) || (opts.PackID) || (opts.TreeID) {
		// do some finishing up
		if s.oldsn != nil {
			Printf("],\"hits\":%d,\"snapshot\":%q}", s.hits, s.oldsn.ID())
		}
		if s.inuse {
			Printf("]\n")
			return
		}
	}

	// convert map into slice and filter out unwanted entries at the same time
	output := []printBuffer{}
	for id, data := range s.buffer {
		_, okBlob := f.blobIDs[id.String()]
		_, okTree := f.treeIDs[id.String()]
		_, okPack := f.packIDs[data.PackID.String()]
		data.ID = id
		if opts.BlobID && okBlob || opts.TreeID && okTree || opts.PackID && okPack {
			output = append(output, data)
		}
	}
	slices.SortStableFunc(output, func(a, b printBuffer) int {
		return cmp.Compare(a.position, b.position)
	})

	if opts.JSON {
		b, err := json.Marshal(output)
		if err != nil {
			Warnf("Marshall failed: %v\n", err)
			return
		}
		Println(string(b))

	} else {
		for _, d := range output {
			debug.Log("Found %s %s", d.ObjectType, d.ID)
			if d.ObjectType == "blob" {
				Printf(" ... in file %s\n", d.Path)
				Printf("     (tree %s)\n", d.ParentTree)
				if opts.ShowPackID {
					Printf(" ... packID %s length %d offset %d ULength %d\n",
						d.PackID, d.Length, d.Offset, d.UncompressedLength)
				}
			} else {
				Printf(" ... path %s\n", d.Path)
				if opts.ShowPackID {
					Printf(" ... packID %s length %d offset %d ULength %d\n",
						d.PackID, d.Length, d.Offset, d.UncompressedLength)
				}
			}
			Printf(" ... in snapshot %s (%s)\n", d.SnapshotID, d.Time.Local().Format(TimeFormat))
		}
	}
}

// Finder bundles information needed to find a file, a directory or a pack.
type Finder struct {
	repo       restic.Repository
	pat        findPattern
	out        statefulOutput
	blobIDs    map[string]struct{}
	treeIDs    map[string]struct{}
	packIDs    map[string]struct{}
	itemsFound int
}

func (f *Finder) findInSnapshot(ctx context.Context, sn *restic.Snapshot) error {
	debug.Log("searching in snapshot %s\n  for entries within [%s %s]", sn.ID(), f.pat.oldest, f.pat.newest)

	if sn.Tree == nil {
		return errors.Errorf("snapshot %v has no tree", sn.ID().Str())
	}

	f.out.newsn = sn
	return walker.Walk(ctx, f.repo, *sn.Tree, walker.WalkVisitor{ProcessNode: func(parentTreeID restic.ID, nodepath string, node *restic.Node, err error) error {
		if err != nil {
			debug.Log("Error loading tree %v: %v", parentTreeID, err)

			Printf("Unable to load tree %s\n ... which belongs to snapshot %s\n", parentTreeID, sn.ID())

			return walker.ErrSkipNode
		}

		if node == nil {
			return nil
		}

		normalizedNodepath := nodepath
		if f.pat.ignoreCase {
			normalizedNodepath = strings.ToLower(nodepath)
		}

		var foundMatch bool

		for _, pat := range f.pat.pattern {
			found, err := filter.Match(pat, normalizedNodepath)
			if err != nil {
				return err
			}
			if found {
				foundMatch = true
				break
			}
		}

		var errIfNoMatch error
		if node.Type == restic.NodeTypeDir {
			var childMayMatch bool
			for _, pat := range f.pat.pattern {
				mayMatch, err := filter.ChildMatch(pat, normalizedNodepath)
				if err != nil {
					return err
				}
				if mayMatch {
					childMayMatch = true
					break
				}
			}

			if !childMayMatch {
				errIfNoMatch = walker.ErrSkipNode
			}
		}

		if !foundMatch {
			return errIfNoMatch
		}

		if !f.pat.oldest.IsZero() && node.ModTime.Before(f.pat.oldest) {
			debug.Log("    ModTime is older than %s\n", f.pat.oldest)
			return errIfNoMatch
		}

		if !f.pat.newest.IsZero() && node.ModTime.After(f.pat.newest) {
			debug.Log("    ModTime is newer than %s\n", f.pat.newest)
			return errIfNoMatch
		}

		debug.Log("    found match\n")
		f.out.PrintPattern(nodepath, node)
		return nil
	}})
}

func (f *Finder) findTree(treeID restic.ID, nodepath string, parentID restic.ID, opts FindOptions) error {
	pb, err := extractBlobData(f.repo, restic.TreeBlob, treeID)
	if err != nil {
		return err
	}
	found := false
	if _, ok := f.treeIDs[treeID.String()]; ok {
		found = true
	} else if _, ok := f.treeIDs[treeID.Str()]; ok {
		found = true
	}
	if found || opts.PackID {
		f.out.PrintObject("tree", treeID, nodepath, parentID, f.out.newsn, pb)
		f.itemsFound++
		// Terminate if we have found all trees (and we are not
		// looking for blobs)
		if f.itemsFound >= len(f.treeIDs) && f.blobIDs == nil {
			// Return an error to terminate the Walk
			return errors.New("OK")
		}
	}
	return nil
}

func (f *Finder) findIDs(ctx context.Context, sn *restic.Snapshot, opts FindOptions) error {
	debug.Log("searching IDs in snapshot %s", sn.ID())

	if sn.Tree == nil {
		return errors.Errorf("snapshot %v has no tree", sn.ID().Str())
	}

	f.out.newsn = sn
	return walker.Walk(ctx, f.repo, *sn.Tree, walker.WalkVisitor{ProcessNode: func(parentTreeID restic.ID, nodepath string, node *restic.Node, err error) error {
		if err != nil {
			debug.Log("Error loading tree %v: %v", parentTreeID, err)

			Printf("Unable to load tree %s\n ... which belongs to snapshot %s\n", parentTreeID, sn.ID())

			return walker.ErrSkipNode
		}

		if node == nil {
			if nodepath == "/" {
				if err := f.findTree(parentTreeID, "/", parentTreeID, opts); err != nil {
					return err
				}
			}
			return nil
		}

		if node.Type == "dir" {
			if err := f.findTree(*node.Subtree, nodepath, parentTreeID, opts); err != nil {
				return err
			}
		}

		if node.Type == restic.NodeTypeFile {
			for _, id := range node.Content {
				if ctx.Err() != nil {
					return ctx.Err()
				}

				idStr := id.String()
				if _, ok := f.blobIDs[idStr]; !ok {
					// Look for short ID form
					if _, ok := f.blobIDs[id.Str()]; !ok {
						continue
					}
					// Replace the short ID with the long one
					f.blobIDs[idStr] = struct{}{}
					delete(f.blobIDs, id.Str())
				}

				pb, err := extractBlobData(f.repo, restic.DataBlob, id)
				if err != nil {
					return err
				}
				f.out.PrintObject("blob", id, nodepath, parentTreeID, sn, pb)
			}
		}

		return nil
	}})
}

var errAllPacksFound = errors.New("all packs found")

// packsToBlobs converts the list of pack IDs to a list of blob IDs that
// belong to those packs.
func (f *Finder) packsToBlobs(ctx context.Context, packs []string) error {
	packIDs := make(map[string]struct{})
	for _, p := range packs {
		packIDs[p] = struct{}{}
		f.packIDs[p] = struct{}{}
	}
	if f.blobIDs == nil {
		f.blobIDs = make(map[string]struct{})
	}

	debug.Log("Looking for packs...")
	err := f.repo.List(ctx, restic.PackFile, func(id restic.ID, size int64) error {
		idStr := id.String()
		if _, ok := packIDs[idStr]; !ok {
			// Look for short ID form
			if _, ok := packIDs[id.Str()]; !ok {
				return nil
			}
			delete(packIDs, id.Str())
		} else {
			// forget found id
			delete(packIDs, idStr)
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
		if len(packIDs) == 0 {
			return errAllPacksFound
		}
		return nil
	})

	if err != nil && err != errAllPacksFound {
		return err
	}

	if err != errAllPacksFound {
		// try to resolve unknown pack ids from the index
		packIDs, err = f.indexPacksToBlobs(ctx, packIDs)
		if err != nil {
			return err
		}
	}

	if len(packIDs) > 0 {
		list := make([]string, 0, len(packIDs))
		for h := range packIDs {
			list = append(list, h)
		}

		sort.Strings(list)
		return errors.Fatalf("unable to find pack(s): %v", list)
	}

	debug.Log("%d blobs found", len(f.blobIDs))
	return nil
}

func (f *Finder) indexPacksToBlobs(ctx context.Context, packIDs map[string]struct{}) (map[string]struct{}, error) {
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// remember which packs were found in the index
	indexPackIDs := make(map[string]struct{})
	err := f.repo.ListBlobs(wctx, func(pb restic.PackedBlob) {
		idStr := pb.PackID.String()
		// keep entry in packIDs as Each() returns individual index entries
		matchingID := false
		if _, ok := packIDs[idStr]; ok {
			matchingID = true
		} else {
			if _, ok := packIDs[pb.PackID.Str()]; ok {
				// expand id
				delete(packIDs, pb.PackID.Str())
				packIDs[idStr] = struct{}{}
				matchingID = true
			}
		}
		if matchingID {
			f.blobIDs[pb.ID.String()] = struct{}{}
			indexPackIDs[idStr] = struct{}{}
		}
	})
	if err != nil {
		return nil, err
	}

	for id := range indexPackIDs {
		delete(packIDs, id)
	}

	if len(indexPackIDs) > 0 {
		list := make([]string, 0, len(indexPackIDs))
		for h := range indexPackIDs {
			list = append(list, h)
		}
		Warnf("some pack files are missing from the repository, getting their blobs from the repository index: %v\n\n", list)
	}
	return packIDs, nil
}

func runFind(ctx context.Context, opts FindOptions, gopts GlobalOptions, args []string) error {
	if len(args) == 0 {
		return errors.Fatal("wrong number of arguments")
	}

	var err error
	opts.JSON = gopts.JSON
	pat := findPattern{pattern: args}
	if opts.CaseInsensitive {
		for i := range pat.pattern {
			pat.pattern[i] = strings.ToLower(pat.pattern[i])
		}
		pat.ignoreCase = true
	}

	if (opts.Oldest != "" || opts.Newest != "") && (opts.BlobID || opts.TreeID || opts.PackID) {
		return errors.Fatal("You cannot mix modification time matching with ID matching")
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

	if pat.oldest.After(pat.newest) {
		return errors.Fatal("option conflict: `--oldest` >= `--newest`")
	}

	// Check at most only one kind of IDs is provided: currently we
	// can't mix types
	if opts.BlobID && opts.TreeID || opts.BlobID && opts.PackID || opts.TreeID && opts.PackID {
		return errors.Fatal("cannot have several ID types")
	}

	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock)
	if err != nil {
		return err
	}
	defer unlock()

	snapshotLister, err := restic.MemorizeList(ctx, repo, restic.SnapshotFile)
	if err != nil {
		return err
	}
	bar := newIndexProgress(gopts.Quiet, gopts.JSON)
	if err = repo.LoadIndex(ctx, bar); err != nil {
		return err
	}

	f := &Finder{
		repo: repo,
		pat:  pat,
		out: statefulOutput{
			ListLong:      opts.ListLong,
			HumanReadable: opts.HumanReadable,
			JSON:          gopts.JSON,
			buffer:        map[restic.ID]printBuffer{},
		},
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
		f.packIDs = make(map[string]struct{})
		err := f.packsToBlobs(ctx, f.pat.pattern)
		if err != nil {
			return err
		}
	}

	var filteredSnapshots []*restic.Snapshot
	for sn := range FindFilteredSnapshots(ctx, snapshotLister, repo, &opts.SnapshotFilter, opts.Snapshots) {
		filteredSnapshots = append(filteredSnapshots, sn)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	sort.Slice(filteredSnapshots, func(i, j int) bool {
		if opts.Reverse {
			return filteredSnapshots[i].Time.Before(filteredSnapshots[j].Time)
		}
		return filteredSnapshots[i].Time.After(filteredSnapshots[j].Time)
	})

	for _, sn := range filteredSnapshots {
		if f.blobIDs != nil || f.treeIDs != nil {
			if err = f.findIDs(ctx, sn, opts); err != nil && err.Error() != "OK" {
				return err
			}
			continue
		}
		if err = f.findInSnapshot(ctx, sn); err != nil {
			return err
		}
	}

	f.out.Finish(opts, f)
	return nil
}

func extractBlobData(repo restic.Repository, Type restic.BlobType, id restic.ID) (restic.PackedBlob, error) {
	results := repo.LookupBlob(Type, id)
	if len(results) == 0 {
		return restic.PackedBlob{}, errors.Errorf("blob %s cannot be located", id.Str())
	}

	for _, result := range results {
		return result, nil
	}
	return restic.PackedBlob{}, nil
}
