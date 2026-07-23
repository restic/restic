package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
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
	"github.com/restic/restic/internal/ui/progress"
	"github.com/restic/restic/internal/walker"
)

// errFindDone is returned from the tree walk when all requested tree IDs were found.
var errFindDone = errors.New("find: all tree IDs found")

func newFindCommand(globalOptions *global.Options) *cobra.Command {
	var opts FindOptions

	cmd := &cobra.Command{
		Use:   "find [flags] PATTERN...",
		Short: "Find a file, a directory or restic IDs",
		Long: `
The "find" command searches for files or directories in snapshots stored in the
repository. It can also be used to search for restic blobs, trees or pack
files for troubleshooting.

The default sort option for the snapshots is youngest to oldest. To sort the
output from oldest to youngest specify --reverse.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
`,
		Example: `restic find config.json
restic find --json "*.yml" "*.json"
restic find --json --blob 420f620f b46ebe8a ddd38656
restic find --show-pack-id --blob 420f620f
restic find --tree 577c2bc9 f81f2e22 a62827a9
restic find --pack 025c1d06`,
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
		s.printer.E("Marshal failed: %v", err)
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
		s.printer.E("Marshal failed: %v", err)
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

// findPatternMatch references the node returned by the tree iterator, which
// allocates a fresh Node per entry, so snapshots sharing a subtree share one
// Node allocation instead of each buffering a copy.
type findPatternMatch struct {
	path string
	node *data.Node
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

// findPatternInverted walks the union of all selected snapshots' trees once,
// loading each unique (treeID, path) exactly once. Snapshots sharing a subtree
// at a path stay grouped for the whole subtree below it; groups split only
// where directories diverge. Matches are buffered per snapshot and flushed
// into statefulOutput in snapshot order after the walk completes.
func (f *Finder) findPatternInverted(ctx context.Context, snapshots []*data.Snapshot) error {
	for _, sn := range snapshots {
		if sn.Tree == nil {
			return errors.Errorf("snapshot %v has no tree", sn.ID().Str())
		}
	}

	buffers := make([][]findPatternMatch, len(snapshots))

	// Root group: snapshots bucketed by their root treeID at "/".
	rootGroup := make(map[restic.ID][]int, len(snapshots))
	for i, sn := range snapshots {
		rootGroup[*sn.Tree] = append(rootGroup[*sn.Tree], i)
	}

	if err := f.processGroup(ctx, "/", rootGroup, buffers); err != nil {
		return err
	}

	for i, sn := range snapshots {
		buf := buffers[i]
		sort.Slice(buf, func(a, b int) bool {
			return buf[a].path < buf[b].path
		})
		f.out.newsn = sn
		for j := range buf {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			f.out.PrintPattern(buf[j].path, buf[j].node)
		}
	}
	f.out.Finish()

	return nil
}

// processGroup loads the trees referenced by treeToSnaps once per unique treeID
// at treePath, attributes matches to every snapshot sharing each tree, and
// recurses into child directories bucketed by path so snapshots converging on
// the same child path collapse to a single load.
func (f *Finder) processGroup(ctx context.Context, treePath string, treeToSnaps map[restic.ID][]int, buffers [][]findPatternMatch) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// childBuckets maps each child path to the child subtrees reached there,
	// grouped by child treeID. Snapshots converging on the same (path, treeID)
	// collapse to a single load on recursion.
	childBuckets := make(map[string]map[restic.ID][]int)

	for treeID, snaps := range treeToSnaps {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		tree, err := data.LoadTree(ctx, f.repo, treeID)
		if err != nil {
			// Soft failure: the snapshots in this group miss this subtree for
			// this run. Siblings and other groups proceed unaffected.
			debug.Log("Error loading tree %v: %v", treeID, err)
			f.printer.S("Unable to load tree %s", treeID)
			continue
		}

		for item := range tree {
			if item.Error != nil {
				return item.Error
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}

			node := item.Node
			nodePath := path.Join(treePath, node.Name)

			if node.Type == data.NodeTypeInvalid {
				return errors.Errorf("node type is empty for node %q", node.Name)
			}

			foundMatch, childMayMatch, err := f.matchFindPattern(nodePath, node)
			if err != nil {
				return err
			}

			if foundMatch && f.matchFindNodeTimeRange(node) {
				debug.Log("    found match\n")
				match := findPatternMatch{path: nodePath, node: node}
				for _, s := range snaps {
					buffers[s] = append(buffers[s], match)
				}
			}

			if node.Type != data.NodeTypeDir {
				continue
			}

			if node.Subtree == nil {
				return errors.Errorf("subtree for node %v in tree %v is nil", node.Name, nodePath)
			}

			if !childMayMatch {
				continue
			}

			bucket, ok := childBuckets[nodePath]
			if !ok {
				bucket = make(map[restic.ID][]int)
				childBuckets[nodePath] = bucket
			}
			bucket[*node.Subtree] = append(bucket[*node.Subtree], snaps...)
		}
	}

	for childPath, bucket := range childBuckets {
		if err := f.processGroup(ctx, childPath, bucket, buffers); err != nil {
			return err
		}
	}

	return nil
}

func (f *Finder) matchFindPattern(nodePath string, node *data.Node) (foundMatch bool, childMayMatch bool, err error) {
	normalizedPath := nodePath
	if f.pat.ignoreCase {
		normalizedPath = strings.ToLower(nodePath)
	}

	isDir := node.Type == data.NodeTypeDir

	for _, pat := range f.pat.pattern {
		if !foundMatch {
			foundMatch, err = filter.Match(pat, normalizedPath)
			if err != nil {
				return false, false, err
			}
		}

		if isDir && !childMayMatch {
			childMayMatch, err = filter.ChildMatch(pat, normalizedPath)
			if err != nil {
				return false, false, err
			}
		}

		if foundMatch && (!isDir || childMayMatch) {
			break
		}
	}

	return foundMatch, childMayMatch, nil
}

func (f *Finder) matchFindNodeTimeRange(node *data.Node) bool {
	if !f.pat.oldest.IsZero() && node.ModTime.Before(f.pat.oldest) {
		debug.Log("    ModTime is older than %s\n", f.pat.oldest)
		return false
	}

	if !f.pat.newest.IsZero() && node.ModTime.After(f.pat.newest) {
		debug.Log("    ModTime is newer than %s\n", f.pat.newest)
		return false
	}

	return true
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
		if f.itemsFound >= len(f.treeIDs) && len(f.blobIDs) == 0 {
			// Return an error to terminate the Walk
			return errFindDone
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

		if node.Type == "dir" && len(f.treeIDs) > 0 {
			if err := f.findTree(*node.Subtree, nodepath); err != nil {
				return err
			}
		}

		if node.Type == data.NodeTypeFile && len(f.blobIDs) > 0 {
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

func (f *Finder) addBlobHandle(h restic.BlobHandle) {
	switch h.Type {
	case restic.DataBlob:
		f.blobIDs[h.ID.String()] = struct{}{}
	case restic.TreeBlob:
		f.treeIDs[h.ID.String()] = struct{}{}
	default:
		panic(fmt.Sprintf("unknown type %v in blob list", h.Type.String()))
	}
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
	if f.treeIDs == nil {
		f.treeIDs = make(map[string]struct{})
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
			packIDs[idStr] = struct{}{}
		}
		debug.Log("Found pack %s", idStr)
		handles, err := f.repo.ListPackHandles(ctx, id, size)
		if err != nil {
			// ignore error to allow fallback to index
			return nil
		}
		for _, h := range handles {
			f.addBlobHandle(h)
		}
		// forget successfully processed pack
		delete(packIDs, idStr)
		// Stop searching when all packs have been found
		if len(packIDs) == 0 {
			return errAllPacksFound
		}
		return nil
	})
	if err != nil && !errors.Is(err, errAllPacksFound) {
		return err
	}

	if len(packIDs) > 0 {
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

	debug.Log("%d blobs %v trees found", len(f.blobIDs), len(f.treeIDs))
	return nil
}

func (f *Finder) indexPacksToBlobs(ctx context.Context, packIDs map[string]struct{}) (map[string]struct{}, error) {
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// remember which packs were found in the index
	indexPackIDs := make(map[string]struct{})
	err := f.repo.ListBlobs(wctx, func(pb restic.PackBlob) {
		packID := pb.PackID()
		idStr := packID.String()
		// keep entry in packIDs as Each() returns individual index entries
		matchingID := false
		if _, ok := packIDs[idStr]; ok {
			matchingID = true
		} else {
			if _, ok := packIDs[packID.Str()]; ok {
				// expand id
				delete(packIDs, packID.Str())
				packIDs[idStr] = struct{}{}
				matchingID = true
			}
		}
		if matchingID {
			f.addBlobHandle(pb.Handle())
			indexPackIDs[idStr] = struct{}{}
		}
	})
	if err != nil {
		return nil, err
	}

	for id := range indexPackIDs {
		delete(packIDs, id)
	}

	return packIDs, nil
}

func (f *Finder) findObjectPack(id string, t restic.BlobType) {
	rid, err := restic.ParseID(id)
	if err != nil {
		f.printer.S("Note: cannot find pack for object '%s', unable to parse ID: %v", id, err)
		return
	}

	blobs := f.repo.LookupBlob(restic.BlobHandle{Type: t, ID: rid})
	if len(blobs) == 0 {
		f.printer.S("Object %s with type %s not found in the index", t.String(), rid.Str())
		return
	}

	for _, b := range blobs {
		if b.Handle().ID.Equal(rid) {
			f.printer.S("Object belongs to pack %s", b.PackID())
			f.printer.S(" ... Pack %s: %v", b.PackID().String(), b.Handle())
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

	printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, term)

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

	if !pat.newest.IsZero() && !pat.oldest.IsZero() && pat.oldest.After(pat.newest) {
		return errors.Fatal("--oldest must specify a time before --newest")
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
	err = opts.SnapshotFilter.FindAll(ctx, snapshotLister, repo, opts.Snapshots, func(_ string, sn *data.Snapshot, err error) error {
		if err != nil {
			return err
		}
		filteredSnapshots = append(filteredSnapshots, sn)
		return nil
	})
	if err != nil {
		return err
	}

	sort.Slice(filteredSnapshots, func(i, j int) bool {
		if opts.Reverse {
			return filteredSnapshots[i].Time.Before(filteredSnapshots[j].Time)
		}
		return filteredSnapshots[i].Time.After(filteredSnapshots[j].Time)
	})

	if len(f.blobIDs) > 0 || len(f.treeIDs) > 0 {
		for _, sn := range filteredSnapshots {
			if err = f.findIDs(ctx, sn); err != nil && !errors.Is(err, errFindDone) {
				return err
			}
		}
		f.out.Finish()
	} else {
		if err = f.findPatternInverted(ctx, filteredSnapshots); err != nil {
			return err
		}
	}

	if opts.ShowPackID && (len(f.blobIDs) > 0 || len(f.treeIDs) > 0) {
		f.findObjectsPacks()
	}

	return nil
}
