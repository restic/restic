package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
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
)

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
	PreserveEmptyDir   bool
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
	f.BoolVar(&opts.PreserveEmptyDir, "preserve-empty-directory", false, "retain the internal information about empty directories")

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
		S(string, ...any)
		P(string, ...any)
		E(string, ...any)
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

func (s *statefulOutput) PrintObjectJSON(kind, id, nodepath, treeID string, sn *data.Snapshot, pb restic.PackBlob) {
	var packID string
	if pb != nil && !pb.PackID().IsNull() {
		packID = pb.PackID().String()
	}
	b, err := json.Marshal(struct {
		// Add these attributes
		ObjectType string    `json:"object_type"`
		ID         string    `json:"id"`
		Path       string    `json:"path"`
		ParentTree string    `json:"parent_tree,omitempty"`
		SnapshotID string    `json:"snapshot"`
		Time       time.Time `json:"time"`
		Packfile   string    `json:"packfile,omitempty"`
	}{
		ObjectType: kind,
		ID:         id,
		Path:       nodepath,
		SnapshotID: sn.ID().String(),
		ParentTree: treeID,
		Time:       sn.Time,
		Packfile:   packID,
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

func (s *statefulOutput) PrintObjectNormal(kind, id, nodepath, treeID string, sn *data.Snapshot, pb restic.PackBlob) {
	s.printer.S("Found %s %s", kind, id)
	if kind == "blob" {
		s.printer.S(" ... in file %s", nodepath)
		s.printer.S("     (tree %s)", treeID)
	} else {
		s.printer.S(" ... path %s", nodepath)
	}
	s.printer.S(" ... in snapshot %s (%s)", sn.ID().Str(), sn.Time.Local().Format(global.TimeFormat))
	if pb != nil && !pb.PackID().IsNull() {
		s.printer.S(" ... packfile %v: %v", pb.PackID(), pb.Handle())
	}
}

func (s *statefulOutput) PrintObject(kind, id, nodepath, treeID string, sn *data.Snapshot, pb restic.PackBlob) {
	if s.JSON {
		s.PrintObjectJSON(kind, id, nodepath, treeID, sn, pb)
	} else {
		s.PrintObjectNormal(kind, id, nodepath, treeID, sn, pb)
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
	repo    restic.Repository
	pat     findPattern
	out     statefulOutput
	blobIDs map[string]struct{}
	treeIDs map[string]struct{}
	printer interface {
		S(string, ...any)
		P(string, ...any)
		E(string, ...any)
	}
}

type DirectoryInfo struct {
	pathname string
	sn       *data.Snapshot
}

type subDirectory struct {
	ID   restic.ID
	name string
}

type OutputSorter struct {
	sn       *data.Snapshot
	pathname string
	node     *data.Node
}

type ToSortBlobs struct {
	sn       *data.Snapshot
	pathname string
	blobID   restic.ID
	parent   restic.ID
	pb       restic.PackBlob
}

var EmptyDirectory restic.ID

// walkDirectoryTree recursively builds the directory name for one snapshot and one tree level
func walkDirectoryTree(
	directoryNames map[restic.ID][]DirectoryInfo,
	parentToChild map[restic.ID][]subDirectory,
	parent restic.ID,
	parentPath string,
	sn *data.Snapshot,
	preserveEmptyDirectory bool,
) {
	for _, child := range parentToChild[parent] {
		if !preserveEmptyDirectory && child.ID.Equal(EmptyDirectory) {
			continue
		}
		pathname := path.Join(parentPath, child.name)
		directoryNames[child.ID] = append(directoryNames[child.ID], DirectoryInfo{pathname, sn})
		walkDirectoryTree(directoryNames, parentToChild, child.ID, pathname, sn, preserveEmptyDirectory)
	}
}

// buildDirectoryTree: construct directory tree from the parent->child relationship
// result: directoryNames
func buildDirectoryTree(snapshots data.Snapshots, parentToChild map[restic.ID][]subDirectory, reverse bool, preserveEmpty bool,
) (directoryNames map[restic.ID][]DirectoryInfo) {
	// tree ac08ce34ba4f8123618661bef2425f7028ffb9ac740578a3ee88684d2523fee8
	// is under normal circumstances pretty useless unless an explicit search pattern is used.
	// It is normally suppressed, but can be activated by using option --preserve-empty-directory
	EmptyDirectory = restic.Hash([]byte(`{"nodes":[]}` + "\n"))
	directoryNames = make(map[restic.ID][]DirectoryInfo)
	for _, sn := range snapshots {
		directoryNames[*sn.Tree] = []DirectoryInfo{{"/", sn}}
		walkDirectoryTree(directoryNames, parentToChild, *sn.Tree, "/", sn, preserveEmpty)
	}

	// sort the itemList for each tree in descending time order (by default)
	for tree, itemList := range directoryNames {
		if len(itemList) <= 1 {
			continue
		}
		slices.SortFunc(itemList, func(a, b DirectoryInfo) int {
			if !reverse {
				return cmp.Or(
					b.sn.Time.Compare(a.sn.Time),
					bytes.Compare(a.sn.ID()[:], b.sn.ID()[:]),
					cmp.Compare(a.pathname, b.pathname),
				)
			}
			return cmp.Or(
				a.sn.Time.Compare(b.sn.Time),
				bytes.Compare(a.sn.ID()[:], b.sn.ID()[:]),
				cmp.Compare(a.pathname, b.pathname),
			)
		})
		directoryNames[tree] = itemList
	}

	return directoryNames
}

// streamTrees collects all parent -> child relationships
// input: treeRoots - a slice of selected snapshot roots
// result: map parent -> children relationship
func streamTrees(ctx context.Context, repo restic.Repository, treeRoots []restic.ID,
) (parentToChild map[restic.ID][]subDirectory, err error) {

	var lock sync.Mutex
	seenParent := restic.NewIDSet()
	parentToChild = make(map[restic.ID][]subDirectory)
	err = data.StreamTrees(ctx, repo, treeRoots, restic.NoopCounter, func(parent restic.ID) bool {
		visited := seenParent.Has(parent)
		seenParent.Insert(parent)
		return visited
	}, func(parent restic.ID, err error, nodes data.TreeNodeIterator) error {
		if err != nil {
			return fmt.Errorf("LoadTree(%v) returned error %v", parent.Str(), err)
		}

		children := []subDirectory{}
		for tree := range nodes {
			if tree.Error != nil {
				return fmt.Errorf("LoadTree returned error %v", tree.Error)
			}

			node := tree.Node
			if node.Type == data.NodeTypeDir {
				children = append(children, subDirectory{*node.Subtree, node.Name})
			}
		}

		lock.Lock()
		parentToChild[parent] = children
		lock.Unlock()
		return nil
	})

	return parentToChild, err
}

// lookupPackfileIDs generates a map of restic.PackBlob from the input blob list, for the given type of blobs
func lookupPackfileIDs(repo restic.Repository, list restic.IDs, t restic.BlobType) (map[restic.ID]restic.PackBlob, error) {
	pb := make(map[restic.ID]restic.PackBlob, len(list))
	for _, id := range list {
		res := repo.LookupBlob(restic.BlobHandle{Type: t, ID: id})
		if len(res) == 0 {
			return nil, errors.Fatalf("Could not find blob %v in any packfile", id)
		}
		// in cases of duplicates pick the first, otherwise the only one PackBlob
		pb[id] = res[0]
	}
	return pb, nil
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
// belong to those packs and places them in f.treeIDs and f.blobIDs
func (f *Finder) packsToBlobs(ctx context.Context, packs []string) error {
	packIDs := make(map[string]struct{})
	for _, p := range packs {
		packIDs[p] = struct{}{}
	}

	debug.Log("Looking for packs...")
	err := f.repo.List(ctx, restic.PackFile, func(id restic.ID, size int64) error {
		idStr := id.String()
		if !has(packIDs, idStr) {
			// Look for short ID form
			if !has(packIDs, id.Str()) {
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

// indexPacksToBlobs tries to find blobs for packsfiles in 'packIDs' via master index
func (f *Finder) indexPacksToBlobs(ctx context.Context, packIDs map[string]struct{}) (map[string]struct{}, error) {
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// remember which packs were found in the index
	indexPackIDs := make(map[string]struct{})
	err := f.repo.ListBlobs(wctx, func(pb restic.PackBlob) {
		packID := pb.PackID()
		idStr := packID.String()
		// keep entry in packIDs as ListBlobs() returns individual index entries
		matchingID := false
		if has(packIDs, idStr) {
			matchingID = true
		} else {
			if has(packIDs, packID.Str()) {
				// expand id - short packfile ID
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

// streamBlobs finds the data blobs for selected blobs (--blob))
// run through the trees again and check contents for the given blob set
func (f *Finder) streamBlobs(ctx context.Context, treeRoots restic.IDs, directoryNames map[restic.ID][]DirectoryInfo,
) (sorter []ToSortBlobs, err error) {

	blobIDs := restic.NewIDSet()
	for blobStr := range f.blobIDs {
		id, err := restic.ParseID(blobStr)
		if err == nil {
			blobIDs.Insert(id)
		}
	}

	var lock sync.Mutex
	sorter = []ToSortBlobs{}
	seenParent := restic.NewIDSet()
	err = data.StreamTrees(ctx, f.repo, treeRoots, restic.NoopCounter, func(parent restic.ID) bool {
		visited := seenParent.Has(parent)
		seenParent.Insert(parent)
		return visited
	}, func(parent restic.ID, err error, nodes data.TreeNodeIterator) error {
		if err != nil {
			return fmt.Errorf("LoadTree(%v) returned error %v", parent.Str(), err)
		}

		for tree := range nodes {
			if tree.Error != nil {
				return fmt.Errorf("LoadTree returned error %v", tree.Error)
			}

			node := tree.Node
			if node.Type == data.NodeTypeFile {
				for _, dirItem := range directoryNames[parent] {
					pathname := path.Join(dirItem.pathname, node.Name)
					for _, cont := range node.Content {

						if blobIDs.Has(cont) || has(f.blobIDs, cont.Str()) {
							lock.Lock()
							sorter = append(sorter, ToSortBlobs{
								blobID:   cont,
								sn:       dirItem.sn,
								pathname: pathname,
								parent:   parent,
							})
							lock.Unlock()
						}
					}
				}
			}
		}
		return nil
	})

	return sorter, err
}

// printSelectedBlobs call streamBlobs(), sorts the selection and
// get them formatted onto the selected output unit
func (f *Finder) printSelectedBlobs(ctx context.Context, treeRoots restic.IDs, directoryNames map[restic.ID][]DirectoryInfo, opts FindOptions,
) error {
	sorter, err := f.streamBlobs(ctx, treeRoots, directoryNames)
	if err != nil {
		f.printer.E("StreamTrees ended with error %v", err)
		return err
	}

	if opts.ShowPackID {
		blobList := make([]restic.ID, 0, len(sorter))
		for _, item := range sorter {
			blobList = append(blobList, item.blobID)
		}

		packIDs, err := lookupPackfileIDs(f.repo, blobList, restic.DataBlob)
		if err != nil {
			return err
		}

		// add packfile ID to the list
		for i, item := range sorter {
			item.pb = packIDs[item.blobID]
			sorter[i] = item
		}
	}

	slices.SortFunc(sorter, func(a, b ToSortBlobs) int {
		if !opts.Reverse {
			return cmp.Or(
				b.sn.Time.Compare(a.sn.Time),
				bytes.Compare(a.sn.ID()[:], b.sn.ID()[:]),
				cmp.Compare(a.pathname, b.pathname),
			)
		}
		return cmp.Or(
			a.sn.Time.Compare(b.sn.Time),
			bytes.Compare(a.sn.ID()[:], b.sn.ID()[:]),
			cmp.Compare(a.pathname, b.pathname),
		)
	})

	for _, item := range sorter {
		f.out.PrintObject("blob", item.blobID.String(), item.pathname, item.parent.String(), item.sn, item.pb)
	}
	return nil
}

// printSelectedTrees prints records for selected tree blobs
// no data.StreamTrees needed here, just the selection via 'f.treeIDs'
func (f *Finder) printSelectedTrees(directoryNames map[restic.ID][]DirectoryInfo, opts FindOptions,
) error {
	treeList := restic.IDs{}
	// find the requested trees in the 'directoryNames' map
	for tree := range directoryNames {
		if has(f.treeIDs, tree.String()) || has(f.treeIDs, tree.Str()) {
			treeList = append(treeList, tree)
		}
	}

	var err error
	packIDs := make(map[restic.ID]restic.PackBlob, len(f.treeIDs))
	if opts.ShowPackID {
		packIDs, err = lookupPackfileIDs(f.repo, treeList, restic.TreeBlob)
		if err != nil {
			return err
		}
	}

	// sort this set of data by ascending pathname
	type Sorter struct {
		pathname string
		tree     restic.ID
		sn       *data.Snapshot
		pb       restic.PackBlob
	}

	sorter := make([]Sorter, 0, len(treeList))
	for _, tree := range treeList {
		for _, item := range directoryNames[tree] {
			sorter = append(sorter, Sorter{
				pathname: item.pathname,
				tree:     tree,
				sn:       item.sn,
				pb:       packIDs[tree]})
		}
	}
	slices.SortFunc(sorter, func(a, b Sorter) int {
		return cmp.Or(
			cmp.Compare(a.pathname, b.pathname),
			b.sn.Time.Compare(a.sn.Time),
			bytes.Compare(a.sn.ID()[:], b.sn.ID()[:]),
		)
	})

	for _, item := range sorter {
		f.out.PrintObject("tree", item.tree.String(), item.pathname, "", item.sn, item.pb)
	}

	return nil
}

// streamPatterns checks for patterns in pathnames
func (f *Finder) streamPatterns(
	ctx context.Context,
	treeRoots restic.IDs,
	directoryNames map[restic.ID][]DirectoryInfo,
	opts FindOptions,
) (sorter []OutputSorter, err error) {

	var lock sync.Mutex
	sorter = []OutputSorter{}
	seenParent := restic.NewIDSet()
	err = data.StreamTrees(ctx, f.repo, treeRoots, restic.NoopCounter, func(parent restic.ID) bool {
		visited := seenParent.Has(parent)
		seenParent.Insert(parent)
		return visited
	}, func(parent restic.ID, err error, nodes data.TreeNodeIterator) error {
		if err != nil {
			return fmt.Errorf("LoadTree(%v) returned error %v", parent.Str(), err)
		}

		for tree := range nodes {
			if tree.Error != nil {
				return fmt.Errorf("LoadTree returned error %v", tree.Error)
			}

			node := tree.Node
			if !f.pat.oldest.IsZero() && node.ModTime.Before(f.pat.oldest) {
				debug.Log("    ModTime is older than %s\n", f.pat.oldest)
				continue
			}
			if !f.pat.newest.IsZero() && node.ModTime.After(f.pat.newest) {
				debug.Log("    ModTime is newer than %s\n", f.pat.newest)
				continue
			}

			for _, pat := range f.pat.pattern {
				for _, dirItem := range directoryNames[parent] {
					pathname := path.Join(dirItem.pathname, node.Name)
					if opts.CaseInsensitive {
						pathname = strings.ToLower(pathname)
					}
					found, err := filter.Match(pat, pathname)
					if err != nil {
						return err
					}
					if !found {
						continue
					}

					debug.Log("    found match%v\n", pathname)
					lock.Lock()
					sorter = append(sorter, OutputSorter{
						sn:       dirItem.sn,
						pathname: pathname,
						node:     node,
					})
					lock.Unlock()
				}
			}
		}
		return nil
	})

	return sorter, err
}

// printPatterns prints the entries found by streamPatterns()
func (f *Finder) printPatterns(
	ctx context.Context,
	treeRoots restic.IDs,
	directoryNames map[restic.ID][]DirectoryInfo,
	opts FindOptions,
) error {
	if len(f.pat.pattern) == 0 {
		return nil
	}

	sorter, err := f.streamPatterns(ctx, treeRoots, directoryNames, opts)
	if err != nil {
		return err
	}

	slices.SortFunc(sorter, func(a, b OutputSorter) int {
		if !opts.Reverse {
			return cmp.Or(
				b.sn.Time.Compare(a.sn.Time),
				bytes.Compare(a.sn.ID()[:], b.sn.ID()[:]),
				cmp.Compare(a.pathname, b.pathname),
			)
		}
		return cmp.Or(
			a.sn.Time.Compare(b.sn.Time),
			bytes.Compare(a.sn.ID()[:], b.sn.ID()[:]),
			cmp.Compare(a.pathname, b.pathname),
		)
	})

	// need to set f.out.newsn for each new snapshot in 'sorter'
	var oldSn *data.Snapshot
	for _, item := range sorter {
		if oldSn == nil || oldSn != item.sn {
			f.out.newsn = item.sn
			oldSn = item.sn
		}
		f.out.PrintPattern(item.pathname, item.node)
	}

	return nil
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

	// Check at most only one kind of IDs is provided: we can't mix types
	if (opts.BlobID && opts.TreeID) ||
		(opts.BlobID && opts.PackID) ||
		(opts.TreeID && opts.PackID) {
		return errors.Fatal("cannot have several ID types")
	}

	hexString := regexp.MustCompile(`^[0-9a-fA-F]+$`)
	searchForIDs := false
	if opts.BlobID || opts.TreeID || opts.PackID {
		searchForIDs = true
		// quickly weed out any wrong IDs before we open the repository
		for _, id := range args {
			_, err := restic.ParseID(id)
			if err != nil && (len(id) != 8 || hexString.FindString(id) != id) {
				return errors.Fatalf("unable to parse ID %q", id)
			}
		}
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
		blobIDs: make(map[string]struct{}),
		treeIDs: make(map[string]struct{}),
	}

	if searchForIDs {
		for _, pat := range args {
			_, _ = restic.ParseID(pat)
			switch {
			case opts.BlobID:
				f.blobIDs[pat] = struct{}{}
			case opts.TreeID:
				f.treeIDs[pat] = struct{}{}
			case opts.PackID:
				// packsToBlobs() deposits the restic.ID(s) for these packfiles in
				// f.treeIDs and/or f.blobIDs
				err := f.packsToBlobs(ctx, f.pat.pattern)
				if err != nil {
					return err
				}
			}
		}
	}

	var filteredSnapshots []*data.Snapshot
	var treeRoots restic.IDs
	err = opts.SnapshotFilter.FindAll(ctx, snapshotLister, repo, opts.Snapshots, func(_ string, sn *data.Snapshot, err error) error {
		if err != nil {
			return err
		}
		filteredSnapshots = append(filteredSnapshots, sn)
		treeRoots = append(treeRoots, *sn.Tree)
		return nil
	})
	if err != nil {
		return err
	}

	// stream all selected snapshots and build directory names from it
	parentToChild, err := streamTrees(ctx, repo, treeRoots)
	if err != nil {
		return err
	}
	directoryNames := buildDirectoryTree(filteredSnapshots, parentToChild, opts.Reverse, opts.PreserveEmptyDir)

	// action 1 - check for --tree
	if len(f.treeIDs) > 0 {
		err := f.printSelectedTrees(directoryNames, opts)
		if err != nil {
			return err
		}
	}

	// action 2 - check for --blob
	if len(f.blobIDs) > 0 {
		err := f.printSelectedBlobs(ctx, treeRoots, directoryNames, opts)
		if err != nil {
			return err
		}
	}

	// action 3 - always check for patterns
	err = f.printPatterns(ctx, treeRoots, directoryNames, opts)
	if err != nil {
		return err
	}

	f.out.Finish()
	return nil
}

func has(set map[string]struct{}, key string) bool {
	_, ok := set[key]
	return ok
}
