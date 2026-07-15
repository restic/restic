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

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/restic/restic/internal/walker"
)

func newLsCommand(globalOptions *global.Options) *cobra.Command {
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
			finalizeSnapshotFilter(&opts.SnapshotFilter)
			return runLs(cmd.Context(), opts, *globalOptions, args, globalOptions.Term)
		},
	}
	opts.AddFlags(cmd.Flags())
	return cmd
}

// LsOptions collects all options for the ls command.
type LsOptions struct {
	ListLong bool
	data.SnapshotFilter
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
	Snapshot(sn *data.Snapshot) error
	NodeOutput(n lsNodeOutput, isPrefixDirectory bool) error
	LeaveDir(path string) error
	Close() error
}

type jsonLsPrinter struct {
	enc *json.Encoder
}

func (p *jsonLsPrinter) Snapshot(sn *data.Snapshot) error {
	type lsSnapshot struct {
		*data.Snapshot
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

type lsNodeOutput struct {
	Name        string        `json:"name"`
	Type        data.NodeType `json:"type"`
	Path        string        `json:"path"`
	UID         uint32        `json:"uid"`
	GID         uint32        `json:"gid"`
	Size        *uint64       `json:"size,omitempty"`
	Mode        os.FileMode   `json:"mode,omitempty"`
	Permissions string        `json:"permissions,omitempty"`
	ModTime     time.Time     `json:"mtime,omitempty"`
	AccessTime  time.Time     `json:"atime,omitempty"`
	ChangeTime  time.Time     `json:"ctime,omitempty"`
	Inode       uint64        `json:"inode,omitempty"`
	MessageType string        `json:"message_type"`
	StructType  string        `json:"struct_type"`

	// Used for the ncdu output only
	LinkTarget string `json:"-"`
	DeviceID   uint64 `json:"-"`
	Links      uint64 `json:"-"`
}

func (n lsNodeOutput) fileSize() uint64 {
	if n.Size == nil {
		return 0
	}
	return *n.Size
}

func lsNodeOutputFrom(path string, node *data.Node) lsNodeOutput {
	n := lsNodeOutput{
		Name:        node.Name,
		Type:        node.Type,
		Path:        path,
		UID:         node.UID,
		GID:         node.GID,
		Mode:        node.Mode,
		Permissions: node.Mode.String(),
		ModTime:     node.ModTime,
		AccessTime:  node.AccessTime,
		ChangeTime:  node.ChangeTime,
		Inode:       node.Inode,
		LinkTarget:  node.LinkTarget,
		DeviceID:    node.DeviceID,
		Links:       node.Links,
		MessageType: "node",
		StructType:  "node",
	}
	// Always print size for regular files, even when empty,
	// but never for other types.
	if node.Type == data.NodeTypeFile {
		size := node.Size
		n.Size = &size
	}
	return n
}

func (p *jsonLsPrinter) NodeOutput(n lsNodeOutput, isPrefixDirectory bool) error {
	if isPrefixDirectory {
		return nil
	}
	return p.enc.Encode(n)
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
func (p *ncduLsPrinter) Snapshot(sn *data.Snapshot) error {
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

func lsNcduOutput(node lsNodeOutput) ([]byte, error) {
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

	size := node.fileSize()
	outNode := NcduNode{
		Name:  node.Name,
		Asize: size,
		// round up to nearest full blocksize
		Dsize:  (size + blockSize - 1) / blockSize * blockSize,
		Dev:    node.DeviceID,
		Ino:    node.Inode,
		NLink:  node.Links,
		NotReg: node.Type != data.NodeTypeDir && node.Type != data.NodeTypeFile,
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

func (p *ncduLsPrinter) NodeOutput(node lsNodeOutput, _ bool) error {
	out, err := lsNcduOutput(node)
	if err != nil {
		return err
	}

	if node.Type == data.NodeTypeDir {
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
	termPrinter   interface {
		P(msg string, args ...interface{})
		S(msg string, args ...interface{})
	}
}

func (p *textLsPrinter) Snapshot(sn *data.Snapshot) error {
	p.termPrinter.P("%v filtered by %v:", sn, p.dirs)
	return nil
}
func (p *textLsPrinter) NodeOutput(node lsNodeOutput, isPrefixDirectory bool) error {
	if !isPrefixDirectory {
		p.termPrinter.S("%s", formatNodeOutput(node, p.ListLong, p.HumanReadable))
	}
	return nil
}

func (p *textLsPrinter) LeaveDir(_ string) error {
	return nil
}
func (p *textLsPrinter) Close() error {
	return nil
}

func runLs(ctx context.Context, opts LsOptions, gopts global.Options, args []string, term ui.Terminal) error {
	termPrinter := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, term)

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

	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock, termPrinter)
	if err != nil {
		return err
	}
	defer unlock()

	snapshotLister, err := restic.MemorizeList(ctx, repo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	if err = repo.LoadIndex(ctx, termPrinter); err != nil {
		return err
	}

	var printer lsPrinter

	if gopts.JSON {
		printer = &jsonLsPrinter{
			enc: json.NewEncoder(gopts.Term.OutputWriter()),
		}
	} else if opts.Ncdu {
		printer = &ncduLsPrinter{
			out: gopts.Term.OutputWriter(),
		}
	} else {
		printer = &textLsPrinter{
			dirs:          dirs,
			ListLong:      opts.ListLong,
			HumanReadable: opts.HumanReadable,
			termPrinter:   termPrinter,
		}
	}
	if opts.Sort != SortModeName || opts.Reverse {
		printer = &sortedPrinter{
			printer:  printer,
			sortMode: opts.Sort,
			reverse:  opts.Reverse,
		}
	}

	sn, subfolder, err := opts.SnapshotFilter.FindLatest(ctx, snapshotLister, repo, args[0])
	if err != nil {
		return err
	}

	sn.Tree, err = data.FindTreeDirectory(ctx, repo, sn.Tree, subfolder)
	if err != nil {
		return err
	}

	if err := printer.Snapshot(sn); err != nil {
		return err
	}

	processNode := func(_ restic.ID, nodepath string, node *data.Node, err error) error {
		if err != nil {
			return err
		}
		if node == nil {
			return nil
		}

		printedDir := false
		if withinDir(nodepath) {
			// if we're within a target path, print the node
			if err := printer.NodeOutput(lsNodeOutputFrom(nodepath, node), false); err != nil {
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
				return printer.NodeOutput(lsNodeOutputFrom(nodepath, node), true)
			}
			return nil
		}

		// otherwise, signal the walker to not walk recursively into any
		// subdirs
		if node.Type == data.NodeTypeDir {
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
	collector []lsNodeOutput
	sortMode  SortMode
	reverse   bool
}

func (p *sortedPrinter) Snapshot(sn *data.Snapshot) error {
	return p.printer.Snapshot(sn)
}
func (p *sortedPrinter) NodeOutput(node lsNodeOutput, isPrefixDirectory bool) error {
	if !isPrefixDirectory {
		p.collector = append(p.collector, node)
	}
	return nil
}

func (p *sortedPrinter) LeaveDir(_ string) error {
	return nil
}
func (p *sortedPrinter) Close() error {
	var comparator func(a, b lsNodeOutput) int
	switch p.sortMode {
	case SortModeName:
	case SortModeSize:
		comparator = func(a, b lsNodeOutput) int {
			return cmp.Or(
				cmp.Compare(a.fileSize(), b.fileSize()),
				cmp.Compare(a.Path, b.Path),
			)
		}
	case SortModeMtime:
		comparator = func(a, b lsNodeOutput) int {
			return cmp.Or(
				a.ModTime.Compare(b.ModTime),
				cmp.Compare(a.Path, b.Path),
			)
		}
	case SortModeAtime:
		comparator = func(a, b lsNodeOutput) int {
			return cmp.Or(
				a.AccessTime.Compare(b.AccessTime),
				cmp.Compare(a.Path, b.Path),
			)
		}
	case SortModeCtime:
		comparator = func(a, b lsNodeOutput) int {
			return cmp.Or(
				a.ChangeTime.Compare(b.ChangeTime),
				cmp.Compare(a.Path, b.Path),
			)
		}
	case SortModeExt:
		// map name to extension
		mapExt := make(map[string]string, len(p.collector))
		for _, item := range p.collector {
			ext := filepath.Ext(item.Path)
			mapExt[item.Path] = ext
		}

		comparator = func(a, b lsNodeOutput) int {
			return cmp.Or(
				cmp.Compare(mapExt[a.Path], mapExt[b.Path]),
				cmp.Compare(a.Path, b.Path),
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
		if err := p.printer.NodeOutput(elem, false); err != nil {
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
