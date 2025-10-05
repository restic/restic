package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/walker"
)

func newFindCommand(globalOptions *global.Options) *cobra.Command {
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
			finalizeSnapshotFilter(&opts.SnapshotFilter)
			return runFind(cmd.Context(), opts, *globalOptions, args, globalOptions.Term)
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
	data.SnapshotFilter
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

type statefulOutput struct {
	ListLong      bool
	HumanReadable bool
	JSON          bool
	inuse         bool
	newsn         *data.Snapshot
	oldsn         *data.Snapshot
	hits          int
	printer       interface {
		S(string, ...interface{})
		P(string, ...interface{})
		E(string, ...interface{})
	}
	stdout io.Writer
}

func (s *statefulOutput) PrintPatternJSON(path string, node *data.Node) {
	type findNode data.Node
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
		s.printer.E("Marshall failed: %v", err)
		return
	}
	if !s.inuse {
		_, _ = s.stdout.Write([]byte("["))
		s.inuse = true
	}
	if s.newsn != s.oldsn {
		if s.oldsn != nil {
			_, _ = fmt.Fprintf(s.stdout, "],\"hits\":%d,\"snapshot\":%q},", s.hits, s.oldsn.ID())
		}
		_, _ = s.stdout.Write([]byte(`{"matches":[`))
		s.oldsn = s.newsn
		s.hits = 0
	}
	if s.hits > 0 {
		_, _ = s.stdout.Write([]byte(","))
	}
	_, _ = s.stdout.Write(b)
	s.hits++
}

func (s *statefulOutput) PrintPatternNormal(path string, node *data.Node) {
	if s.newsn != s.oldsn {
		if s.oldsn != nil {
			s.printer.P("")
		}
		s.oldsn = s.newsn
		s.printer.P("Found matching entries in snapshot %s from %s", s.oldsn.ID().Str(), s.oldsn.Time.Local().Format(global.TimeFormat))
	}
	s.printer.S(formatNode(path, node, s.ListLong, s.HumanReadable))
}

func (s *statefulOutput) PrintPattern(path string, node *data.Node) {
	if s.JSON {
		s.PrintPatternJSON(path, node)
	} else {
		s.PrintPatternNormal(path, node)
	}
}

func (s *statefulOutput) PrintObjectJSON(kind, id, nodepath, treeID string, sn *data.Snapshot) {
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
		s.printer.E("Marshall failed: %v", err)
		return
	}
	if !s.inuse {
		_, _ = s.stdout.Write([]byte("["))
		s.inuse = true
	}
	if s.hits > 0 {
		_, _ = s.stdout.Write([]byte(","))
	}
	_, _ = s.stdout.Write(b)
	s.hits++
}

func (s *statefulOutput) PrintObjectNormal(kind, id, nodepath, treeID string, sn *data.Snapshot) {
	s.printer.S("Found %s %s", kind, id)
	if kind == "blob" {
		s.printer.S(" ... in file %s", nodepath)
		s.printer.S("     (tree %s)", treeID)
	} else {
		s.printer.S(" ... path %s", nodepath)
	}
	s.printer.S(" ... in snapshot %s (%s)", sn.ID().Str(), sn.Time.Local().Format(global.TimeFormat))
}

func (s *statefulOutput) PrintObject(kind, id, nodepath, treeID string, sn *data.Snapshot) {
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
			_, _ = fmt.Fprintf(s.stdout, "],\"hits\":%d,\"snapshot\":%q}", s.hits, s.oldsn.ID())
		}
		if s.inuse {
			_, _ = s.stdout.Write([]byte("]\n"))
		} else {
			_, _ = s.stdout.Write([]byte("[]\n"))
		}
		return
	}
}

// Finder bundles information needed to find a file or directory.
type Finder struct {
	repo       restic.Repository
	pat        findPattern
	out        statefulOutput
	blobIDs    map[string]struct{}
	treeIDs    map[string]struct{}
	itemsFound int
	printer    interface {
		S(string, ...interface{})
		P(string, ...interface{})
		E(string, ...interface{})
	}
}

func (f *Finder) findInSnapshot(ctx context.Context, sn *data.Snapshot) error {
	debug.Log("searching in snapshot %s\n  for entries within [%s %s]", sn.ID(), f.pat.oldest, f.pat.newest)

	if sn.Tree == nil {
		return errors.Errorf("snapshot %v has no tree", sn.ID().Str())
	}

	f.out.newsn = sn
	return walker.Walk(ctx, f.repo, *sn.Tree, walker.WalkVisitor{ProcessNode: func(parentTreeID restic.ID, nodepath string, node *data.Node, err error) error {
		if err != nil {
			debug.Log("Error loading tree %v: %v", parentTreeID, err)

			f.printer.S("Unable to load tree %s", parentTreeID)
			f.printer.S(" ... which belongs to snapshot %s", sn.ID())

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
		if node.Type == data.NodeTypeDir {
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

func (f *Finder) findTree(treeID restic.ID, nodepath string) error {
	found := false
	if _, ok := f.treeIDs[treeID.String()]; ok {
		found = true
	} else if _, ok := f.treeIDs[treeID.Str()]; ok {
		found = true
	}
	if found {
		f.out.PrintObject("tree", treeID.String(), nodepath, "", f.out.newsn)
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

func (f *Finder) findIDs(ctx context.Context, sn *data.Snapshot) error {
	debug.Log("searching IDs in snapshot %s", sn.ID())

	if sn.Tree == nil {
		return errors.Errorf("snapshot %v has no tree", sn.ID().Str())
	}

	f.out.newsn = sn
	return walker.Walk(ctx, f.repo, *sn.Tree, walker.WalkVisitor{ProcessNode: func(parentTreeID restic.ID, nodepath string, node *data.Node, err error) error {
		if err != nil {
			debug.Log("Error loading tree %v: %v", parentTreeID, err)

			f.printer.S("Unable to load tree %s", parentTreeID)
			f.printer.S(" ... which belongs to snapshot %s", sn.ID())

			return walker.ErrSkipNode
		}

		if node == nil {
			if nodepath == "/" {
				if err := f.findTree(parentTreeID, "/"); err != nil {
					return err
				}
			}
			return nil
		}

		if node.Type == "dir" && f.treeIDs != nil {
			if err := f.findTree(*node.Subtree, nodepath); err != nil {
				return err
			}
		}

		if node.Type == data.NodeTypeFile && f.blobIDs != nil {
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
				f.out.PrintObject("blob", idStr, nodepath, parentTreeID.String(), sn)
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
		f.printer.E("some pack files are missing from the repository, getting their blobs from the repository index: %v\n\n", list)
	}
	return packIDs, nil
}

func (f *Finder) findObjectPack(id string, t restic.BlobType) {
	rid, err := restic.ParseID(id)
	if err != nil {
		f.printer.S("Note: cannot find pack for object '%s', unable to parse ID: %v", id, err)
		return
	}

	blobs := f.repo.LookupBlob(t, rid)
	if len(blobs) == 0 {
		f.printer.S("Object %s not found in the index", rid.Str())
		return
	}

	for _, b := range blobs {
		if b.ID.Equal(rid) {
			f.printer.S("Object belongs to pack %s", b.PackID)
			f.printer.S(" ... Pack %s: %s", b.PackID.Str(), b.String())
			break
		}
	}
}

func (f *Finder) findObjectsPacks() {
	for i := range f.blobIDs {
		f.findObjectPack(i, restic.DataBlob)
	}

	for i := range f.treeIDs {
		f.findObjectPack(i, restic.TreeBlob)
	}
}

func runFind(ctx context.Context, opts FindOptions, gopts global.Options, args []string, term ui.Terminal) error {
	if len(args) == 0 {
		return errors.Fatal("wrong number of arguments")
	}

	printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, term)

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

	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock, printer)
	if err != nil {
		return err
	}
	defer unlock()

	snapshotLister, err := restic.MemorizeList(ctx, repo, restic.SnapshotFile)
	if err != nil {
		return err
	}
	if err = repo.LoadIndex(ctx, printer); err != nil {
		return err
	}

	f := &Finder{
		repo:    repo,
		pat:     pat,
		out:     statefulOutput{ListLong: opts.ListLong, HumanReadable: opts.HumanReadable, JSON: gopts.JSON, printer: printer, stdout: term.OutputRaw()},
		printer: printer,
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
		err := f.packsToBlobs(ctx, f.pat.pattern)
		if err != nil {
			return err
		}
	}

	var filteredSnapshots []*data.Snapshot
	for sn := range FindFilteredSnapshots(ctx, snapshotLister, repo, &opts.SnapshotFilter, opts.Snapshots, printer) {
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

	if opts.ShowPackID && (f.blobIDs != nil || f.treeIDs != nil) {
		f.findObjectsPacks()
	}

	return nil
}
