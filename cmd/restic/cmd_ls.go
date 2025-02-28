package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
)

func newLsCommand() *cobra.Command {
	var opts LsOptions

	cmd := &cobra.Command{
		Use:   "ls [flags] snapshotID [dir...]",
		Short: "List files in a snapshot",
		Long: `
The "ls" command lists files and directories in a snapshot.

The special snapshot ID "latest" can be used to list files and
directories of the latest snapshot in the repository. The
--host flag can be used in conjunction to select the latest
snapshot originating from a certain host only.

File listings can optionally be filtered by directories. Any
positional arguments after the snapshot ID are interpreted as
absolute directory paths, and only files inside those directories
will be listed. If the --recursive flag is used, then the filter
will allow traversing into matching directories' subfolders.
Any directory paths specified must be absolute (starting with
a path separator); paths use the forward slash '/' as separator.

File listings can be sorted by specifying --sort followed by one of the
sort specifiers '(name|size|time=mtime|atime|ctime|extension)'.
The sorting can be reversed by specifying --reverse.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
`,
		DisableAutoGenTag: true,
		GroupID:           cmdGroupDefault,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLs(cmd.Context(), opts, globalOptions, args)
		},
	}
	opts.AddFlags(cmd.Flags())
	return cmd
}

// LsOptions collects all options for the ls command.
type LsOptions struct {
	ListLong bool
	restic.SnapshotFilter
	Recursive     bool
	HumanReadable bool
	Ncdu          bool
	Sort          SortMode
	Reverse       bool
}

func (opts *LsOptions) AddFlags(f *pflag.FlagSet) {
	initSingleSnapshotFilter(f, &opts.SnapshotFilter)
	f.BoolVarP(&opts.ListLong, "long", "l", false, "use a long listing format showing size and mode")
	f.BoolVar(&opts.Recursive, "recursive", false, "include files in subfolders of the listed directories")
	f.BoolVar(&opts.HumanReadable, "human-readable", false, "print sizes in human readable format")
	f.BoolVar(&opts.Ncdu, "ncdu", false, "output NCDU export format (pipe into 'ncdu -f -')")
	f.VarP(&opts.Sort, "sort", "s", "sort output by (name|size|time=mtime|atime|ctime|extension)")
	f.BoolVar(&opts.Reverse, "reverse", false, "reverse sorted output")
}

type lsPrinter interface {
	Snapshot(sn *restic.Snapshot) error
	Node(path string, node *restic.Node, isPrefixDirectory bool) error
	LeaveDir(path string) error
	Close() error
}

type jsonLsPrinter struct {
	enc *json.Encoder
}

func (p *jsonLsPrinter) Snapshot(sn *restic.Snapshot) error {
	type lsSnapshot struct {
		*restic.Snapshot
		ID          *restic.ID `json:"id"`
		ShortID     string     `json:"short_id"`     // deprecated
		MessageType string     `json:"message_type"` // "snapshot"
		StructType  string     `json:"struct_type"`  // "snapshot", deprecated
	}

	return p.enc.Encode(lsSnapshot{
		Snapshot:    sn,
		ID:          sn.ID(),
		ShortID:     sn.ID().Str(),
		MessageType: "snapshot",
		StructType:  "snapshot",
	})
}

// Node formats node in our custom JSON format, followed by a newline.
func (p *jsonLsPrinter) Node(path string, node *restic.Node, isPrefixDirectory bool) error {
	if isPrefixDirectory {
		return nil
	}
	return lsNodeJSON(p.enc, path, node)
}

func lsNodeJSON(enc *json.Encoder, path string, node *restic.Node) error {
	n := &struct {
		Name        string      `json:"name"`
		Type        string      `json:"type"`
		Path        string      `json:"path"`
		UID         uint32      `json:"uid"`
		GID         uint32      `json:"gid"`
		Size        *uint64     `json:"size,omitempty"`
		Mode        os.FileMode `json:"mode,omitempty"`
		Permissions string      `json:"permissions,omitempty"`
		ModTime     time.Time   `json:"mtime,omitempty"`
		AccessTime  time.Time   `json:"atime,omitempty"`
		ChangeTime  time.Time   `json:"ctime,omitempty"`
		Inode       uint64      `json:"inode,omitempty"`
		MessageType string      `json:"message_type"` // "node"
		StructType  string      `json:"struct_type"`  // "node", deprecated

		size uint64 // Target for Size pointer.
	}{
		Name:        node.Name,
		Type:        string(node.Type),
		Path:        path,
		UID:         node.UID,
		GID:         node.GID,
		size:        node.Size,
		Mode:        node.Mode,
		Permissions: node.Mode.String(),
		ModTime:     node.ModTime,
		AccessTime:  node.AccessTime,
		ChangeTime:  node.ChangeTime,
		Inode:       node.Inode,
		MessageType: "node",
		StructType:  "node",
	}
	// Always print size for regular files, even when empty,
	// but never for other types.
	if node.Type == restic.NodeTypeFile {
		n.Size = &n.size
	}

	return enc.Encode(n)
}

func (p *jsonLsPrinter) LeaveDir(_ string) error { return nil }
func (p *jsonLsPrinter) Close() error            { return nil }

type ncduLsPrinter struct {
	out   io.Writer
	depth int
}

// Snapshot prints a restic snapshot in Ncdu save format.
// It opens the JSON list. Nodes are added with lsNodeNcdu and the list is closed by lsCloseNcdu.
// Format documentation: https://dev.yorhel.nl/ncdu/jsonfmt
func (p *ncduLsPrinter) Snapshot(sn *restic.Snapshot) error {
	const NcduMajorVer = 1
	const NcduMinorVer = 2

	snapshotBytes, err := json.Marshal(sn)
	if err != nil {
		return err
	}
	p.depth++
	_, err = fmt.Fprintf(p.out, "[%d, %d, %s, [{\"name\":\"/\"}", NcduMajorVer, NcduMinorVer, string(snapshotBytes))
	return err
}

func lsNcduNode(_ string, node *restic.Node) ([]byte, error) {
	type NcduNode struct {
		Name   string `json:"name"`
		Asize  uint64 `json:"asize"`
		Dsize  uint64 `json:"dsize"`
		Dev    uint64 `json:"dev"`
		Ino    uint64 `json:"ino"`
		NLink  uint64 `json:"nlink"`
		NotReg bool   `json:"notreg"`
		UID    uint32 `json:"uid"`
		GID    uint32 `json:"gid"`
		Mode   uint16 `json:"mode"`
		Mtime  int64  `json:"mtime"`
	}

	const blockSize = 512

	outNode := NcduNode{
		Name:  node.Name,
		Asize: node.Size,
		// round up to nearest full blocksize
		Dsize:  (node.Size + blockSize - 1) / blockSize * blockSize,
		Dev:    node.DeviceID,
		Ino:    node.Inode,
		NLink:  node.Links,
		NotReg: node.Type != restic.NodeTypeDir && node.Type != restic.NodeTypeFile,
		UID:    node.UID,
		GID:    node.GID,
		Mode:   uint16(node.Mode & os.ModePerm),
		Mtime:  node.ModTime.Unix(),
	}
	//  bits according to inode(7) manpage
	if node.Mode&os.ModeSetuid != 0 {
		outNode.Mode |= 0o4000
	}
	if node.Mode&os.ModeSetgid != 0 {
		outNode.Mode |= 0o2000
	}
	if node.Mode&os.ModeSticky != 0 {
		outNode.Mode |= 0o1000
	}
	if outNode.Mtime < 0 {
		// ncdu does not allow negative times
		outNode.Mtime = 0
	}

	return json.Marshal(outNode)
}

func (p *ncduLsPrinter) Node(path string, node *restic.Node, _ bool) error {
	out, err := lsNcduNode(path, node)
	if err != nil {
		return err
	}

	if node.Type == restic.NodeTypeDir {
		_, err = fmt.Fprintf(p.out, ",\n%s[\n%s%s", strings.Repeat("  ", p.depth), strings.Repeat("  ", p.depth+1), string(out))
		p.depth++
	} else {
		_, err = fmt.Fprintf(p.out, ",\n%s%s", strings.Repeat("  ", p.depth), string(out))
	}
	return err
}

func (p *ncduLsPrinter) LeaveDir(_ string) error {
	p.depth--
	_, err := fmt.Fprintf(p.out, "\n%s]", strings.Repeat("  ", p.depth))
	return err
}

func (p *ncduLsPrinter) Close() error {
	_, err := fmt.Fprint(p.out, "\n]\n]\n")
	return err
}

type textLsPrinter struct {
	dirs          []string
	ListLong      bool
	HumanReadable bool
}

func (p *textLsPrinter) Snapshot(sn *restic.Snapshot) error {
	Verbosef("%v filtered by %v:\n", sn, p.dirs)
	return nil
}
func (p *textLsPrinter) Node(path string, node *restic.Node, isPrefixDirectory bool) error {
	if !isPrefixDirectory {
		Printf("%s\n", formatNode(path, node, p.ListLong, p.HumanReadable))
	}
	return nil
}

func (p *textLsPrinter) LeaveDir(_ string) error {
	return nil
}
func (p *textLsPrinter) Close() error {
	return nil
}

// for ls -l output sorting
type toSortOutput struct {
	nodepath string
	node     *restic.Node
}

func runLs(ctx context.Context, opts LsOptions, gopts GlobalOptions, args []string) error {
	if len(args) == 0 {
		return errors.Fatal("no snapshot ID specified, specify snapshot ID or use special ID 'latest'")
	}
	if opts.Ncdu && gopts.JSON {
		return errors.Fatal("only either '--json' or '--ncdu' can be specified")
	}
	if opts.Sort != SortModeName && opts.Ncdu {
		return errors.Fatal("--sort and --ncdu are mutually exclusive")
	}
	if opts.Reverse && opts.Ncdu {
		return errors.Fatal("--reverse and --ncdu are mutually exclusive")
	}

	// extract any specific directories to walk
	var dirs []string
	if len(args) > 1 {
		dirs = args[1:]
		for _, dir := range dirs {
			if !strings.HasPrefix(dir, "/") {
				return errors.Fatal("All path filters must be absolute, starting with a forward slash '/'")
			}
		}
	}

	withinDir := func(nodepath string) bool {
		if len(dirs) == 0 {
			return true
		}

		for _, dir := range dirs {
			// we're within one of the selected dirs, example:
			//   nodepath: "/test/foo"
			//   dir:      "/test"
			if fs.HasPathPrefix(dir, nodepath) {
				return true
			}
		}
		return false
	}

	approachingMatchingTree := func(nodepath string) bool {
		if len(dirs) == 0 {
			return true
		}

		for _, dir := range dirs {
			// the current node path is a prefix for one of the
			// directories, so we're interested in something deeper in the
			// tree. Example:
			//   nodepath: "/test"
			//   dir:      "/test/foo"
			if fs.HasPathPrefix(nodepath, dir) {
				return true
			}
		}
		return false
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

	var printer lsPrinter

	if gopts.JSON {
		printer = &jsonLsPrinter{
			enc: json.NewEncoder(globalOptions.stdout),
		}
	} else if opts.Ncdu {
		printer = &ncduLsPrinter{
			out: globalOptions.stdout,
		}
	} else {
		printer = &textLsPrinter{
			dirs:          dirs,
			ListLong:      opts.ListLong,
			HumanReadable: opts.HumanReadable,
		}
	}
	if opts.Sort != SortModeName || opts.Reverse {
		printer = &sortedPrinter{
			printer:  printer,
			sortMode: opts.Sort,
			reverse:  opts.Reverse,
		}
	}

	sn, subfolder, err := (&restic.SnapshotFilter{
		Hosts: opts.Hosts,
		Paths: opts.Paths,
		Tags:  opts.Tags,
	}).FindLatest(ctx, snapshotLister, repo, args[0])
	if err != nil {
		return err
	}

	sn.Tree, err = restic.FindTreeDirectory(ctx, repo, sn.Tree, subfolder)
	if err != nil {
		return err
	}

	if err := printer.Snapshot(sn); err != nil {
		return err
	}

	processNode := func(_ restic.ID, nodepath string, node *restic.Node, err error) error {
		if err != nil {
			return err
		}
		if node == nil {
			return nil
		}

		printedDir := false
		if withinDir(nodepath) {
			// if we're within a target path, print the node
			if err := printer.Node(nodepath, node, false); err != nil {
				return err
			}
			printedDir = true

			// if recursive listing is requested, signal the walker that it
			// should continue walking recursively
			if opts.Recursive {
				return nil
			}
		}

		// if there's an upcoming match deeper in the tree (but we're not
		// there yet), signal the walker to descend into any subdirs
		if approachingMatchingTree(nodepath) {
			// print node leading up to the target paths
			if !printedDir {
				return printer.Node(nodepath, node, true)
			}
			return nil
		}

		// otherwise, signal the walker to not walk recursively into any
		// subdirs
		if node.Type == restic.NodeTypeDir {
			// immediately generate leaveDir if the directory is skipped
			if printedDir {
				if err := printer.LeaveDir(nodepath); err != nil {
					return err
				}
			}
			return walker.ErrSkipNode
		}
		return nil
	}

	err = walker.Walk(ctx, repo, *sn.Tree, walker.WalkVisitor{
		ProcessNode: processNode,
		LeaveDir: func(path string) error {
			// the root path `/` has no corresponding node and is thus also skipped by processNode
			if path != "/" {
				return printer.LeaveDir(path)
			}
			return nil
		},
	})

	if err != nil {
		return err
	}

	return printer.Close()
}

type sortedPrinter struct {
	printer   lsPrinter
	collector []toSortOutput
	sortMode  SortMode
	reverse   bool
}

func (p *sortedPrinter) Snapshot(sn *restic.Snapshot) error {
	return p.printer.Snapshot(sn)
}
func (p *sortedPrinter) Node(path string, node *restic.Node, isPrefixDirectory bool) error {
	if !isPrefixDirectory {
		p.collector = append(p.collector, toSortOutput{path, node})
	}
	return nil
}

func (p *sortedPrinter) LeaveDir(_ string) error {
	return nil
}
func (p *sortedPrinter) Close() error {
	var comparator func(a, b toSortOutput) int
	switch p.sortMode {
	case SortModeName:
	case SortModeSize:
		comparator = func(a, b toSortOutput) int {
			return cmp.Or(
				cmp.Compare(a.node.Size, b.node.Size),
				cmp.Compare(a.nodepath, b.nodepath),
			)
		}
	case SortModeMtime:
		comparator = func(a, b toSortOutput) int {
			return cmp.Or(
				a.node.ModTime.Compare(b.node.ModTime),
				cmp.Compare(a.nodepath, b.nodepath),
			)
		}
	case SortModeAtime:
		comparator = func(a, b toSortOutput) int {
			return cmp.Or(
				a.node.AccessTime.Compare(b.node.AccessTime),
				cmp.Compare(a.nodepath, b.nodepath),
			)
		}
	case SortModeCtime:
		comparator = func(a, b toSortOutput) int {
			return cmp.Or(
				a.node.ChangeTime.Compare(b.node.ChangeTime),
				cmp.Compare(a.nodepath, b.nodepath),
			)
		}
	case SortModeExt:
		// map name to extension
		mapExt := make(map[string]string, len(p.collector))
		for _, item := range p.collector {
			ext := filepath.Ext(item.nodepath)
			mapExt[item.nodepath] = ext
		}

		comparator = func(a, b toSortOutput) int {
			return cmp.Or(
				cmp.Compare(mapExt[a.nodepath], mapExt[b.nodepath]),
				cmp.Compare(a.nodepath, b.nodepath),
			)
		}
	}

	if comparator != nil {
		slices.SortStableFunc(p.collector, comparator)
	}
	if p.reverse {
		slices.Reverse(p.collector)
	}
	for _, elem := range p.collector {
		if err := p.printer.Node(elem.nodepath, elem.node, false); err != nil {
			return err
		}
	}
	return nil
}

// SortMode defines the allowed sorting modes
type SortMode uint

// Allowed sort modes
const (
	SortModeName SortMode = iota
	SortModeSize
	SortModeAtime
	SortModeCtime
	SortModeMtime
	SortModeExt
	SortModeInvalid
)

// Set implements the method needed for pflag command flag parsing.
func (c *SortMode) Set(s string) error {
	switch s {
	case "name":
		*c = SortModeName
	case "size":
		*c = SortModeSize
	case "atime":
		*c = SortModeAtime
	case "ctime":
		*c = SortModeCtime
	case "mtime", "time":
		*c = SortModeMtime
	case "extension":
		*c = SortModeExt
	default:
		*c = SortModeInvalid
		return fmt.Errorf("invalid sort mode %q, must be one of (name|size|time=mtime|atime|ctime|extension)", s)
	}

	return nil
}

func (c *SortMode) String() string {
	switch *c {
	case SortModeName:
		return "name"
	case SortModeSize:
		return "size"
	case SortModeAtime:
		return "atime"
	case SortModeCtime:
		return "ctime"
	case SortModeMtime:
		return "mtime"
	case SortModeExt:
		return "extension"
	default:
		return "invalid"
	}
}

func (c *SortMode) Type() string {
	return "mode"
}
