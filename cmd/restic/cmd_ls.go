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

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
)

var cmdLs = &cobra.Command{
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
sort specifiers '[name|size|time|atime|ctime|mtime]'.
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
		return runLs(cmd.Context(), lsOptions, globalOptions, args)
	},
}

// LsOptions collects all options for the ls command.
type LsOptions struct {
	ListLong bool
	restic.SnapshotFilter
	Recursive     bool
	HumanReadable bool
	Ncdu          bool
	Sort          string
	Reverse       bool
}

var lsOptions LsOptions

func init() {
	cmdRoot.AddCommand(cmdLs)

	flags := cmdLs.Flags()
	initSingleSnapshotFilter(flags, &lsOptions.SnapshotFilter)
	flags.BoolVarP(&lsOptions.ListLong, "long", "l", false, "use a long listing format showing size and mode")
	flags.BoolVar(&lsOptions.Recursive, "recursive", false, "include files in subfolders of the listed directories")
	flags.BoolVar(&lsOptions.HumanReadable, "human-readable", false, "print sizes in human readable format")
	flags.BoolVar(&lsOptions.Ncdu, "ncdu", false, "output NCDU export format (pipe into 'ncdu -f -')")
	flags.StringVarP(&lsOptions.Sort, "sort", "s", "name", "sort output by [name|size|time(=mtime)|atime|ctime|mtime|X(=extension)|extension]")
	flags.BoolVarP(&lsOptions.Reverse, "reverse", "R", false, "reverse sorted output")
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
		ShortID     string     `json:"short_id"`
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

// Print node in our custom JSON format, followed by a newline.
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

// lsSnapshotNcdu prints a restic snapshot in Ncdu save format.
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

func runLs(ctx context.Context, opts LsOptions, gopts GlobalOptions, args []string) error {
	if len(args) == 0 {
		return errors.Fatal("no snapshot ID specified, specify snapshot ID or use special ID 'latest'")
	}
	if opts.Ncdu && gopts.JSON {
		return errors.Fatal("only either '--json' or '--ncdu' can be specified")
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

	// for ls -l output sorting
	type ToSort struct {
		nodepath string
		node     *restic.Node
	}

	collector := []ToSort{}
	outputSort := true

	if gopts.JSON {
		printer = &jsonLsPrinter{
			enc: json.NewEncoder(globalOptions.stdout),
		}
	} else if opts.Ncdu {
		printer = &ncduLsPrinter{
			out: globalOptions.stdout,
		}
		outputSort = false
	} else {
		printer = &textLsPrinter{
			dirs:          dirs,
			ListLong:      opts.ListLong,
			HumanReadable: opts.HumanReadable,
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
			if outputSort {
				collector = append(collector, ToSort{nodepath, node})
			} else {
				if err := printer.Node(nodepath, node, false); err != nil {
					return err
				}
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
			if !printedDir && !outputSort {
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

	if outputSort {
		if opts.Sort == "size" {
			slices.SortStableFunc(collector, func(a, b ToSort) int {
				return cmp.Or(
					cmp.Compare(a.node.Size, b.node.Size),
					cmp.Compare(a.nodepath, b.nodepath),
				)
			})
		} else if opts.Sort == "time" || opts.Sort == "mtime" {
			slices.SortStableFunc(collector, func(a, b ToSort) int {
				return cmp.Or(
					a.node.ModTime.Compare(b.node.ModTime),
					cmp.Compare(a.nodepath, b.nodepath),
				)
			})
		} else if opts.Sort == "atime" {
			slices.SortStableFunc(collector, func(a, b ToSort) int {
				return cmp.Or(
					a.node.AccessTime.Compare(b.node.AccessTime),
					cmp.Compare(a.nodepath, b.nodepath),
				)
			})
		} else if opts.Sort == "ctime" {
			slices.SortStableFunc(collector, func(a, b ToSort) int {
				return cmp.Or(
					a.node.ChangeTime.Compare(b.node.ChangeTime),
					cmp.Compare(a.nodepath, b.nodepath),
				)
			})
		} else if opts.Sort == "X" || opts.Sort == "extension" {
			// prepare sort by extension
			/* linux 'ls -l' does not support multiple sort criteria,
			   last sort option specified in argument list wins.
			   ls -l has another sort option '-v' (version sorting)
			   which is not trivial to implement. In don't think it is a worthwhile
			   effort.
			*/
			type ToSortExt struct {
				nodepath  string
				extension string
				node      *restic.Node
			}
			collector_ext := make([]ToSortExt, len(collector))
			for ix, item := range collector {
				collector_ext[ix] = ToSortExt{
					nodepath:  item.nodepath,
					extension: filepath.Ext(item.nodepath),
					node:      item.node,
				}
			}

			slices.SortStableFunc(collector_ext, func(a, b ToSortExt) int {
				return cmp.Or(
					cmp.Compare(a.extension, b.extension),
					cmp.Compare(a.nodepath, b.nodepath),
				)
			})
			// rewrite collector after sort by extension
			for ix, item := range collector_ext {
				collector[ix] = ToSort{
					nodepath: item.nodepath,
					node:     item.node,
				}
			}
		} else {
			// fallback: this implies opts.Sort == "name"
			slices.SortStableFunc(collector, func(a, b ToSort) int {
				return cmp.Compare(a.nodepath, b.nodepath)
			})
		}

		if opts.Reverse {
			slices.Reverse(collector)
		}
		for _, elem := range collector {
			printer.Node(elem.nodepath, elem.node, false)
		}
	}

	return printer.Close()
}
