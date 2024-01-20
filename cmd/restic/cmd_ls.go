package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
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

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
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
}

var lsOptions LsOptions

func init() {
	cmdRoot.AddCommand(cmdLs)

	flags := cmdLs.Flags()
	initSingleSnapshotFilter(flags, &lsOptions.SnapshotFilter)
	flags.BoolVarP(&lsOptions.ListLong, "long", "l", false, "use a long listing format showing size and mode")
	flags.BoolVar(&lsOptions.Recursive, "recursive", false, "include files in subfolders of the listed directories")
	flags.BoolVar(&lsOptions.HumanReadable, "human-readable", false, "print sizes in human readable format")
	flags.BoolVar(&lsOptions.Ncdu, "ncdu", false, "output NCDU save format (pipe into 'ncdu -f -')")
}

type lsSnapshot struct {
	*restic.Snapshot
	ID         *restic.ID `json:"id"`
	ShortID    string     `json:"short_id"`
	StructType string     `json:"struct_type"` // "snapshot"
}

// Print node in our custom JSON format, followed by a newline.
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
		StructType  string      `json:"struct_type"` // "node"

		size uint64 // Target for Size pointer.
	}{
		Name:        node.Name,
		Type:        node.Type,
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
		StructType:  "node",
	}
	// Always print size for regular files, even when empty,
	// but never for other types.
	if node.Type == "file" {
		n.Size = &n.size
	}

	return enc.Encode(n)
}

type ncduPrinter struct {
	out   io.Writer
	depth int
}

// lsSnapshotNcdu prints a restic snapshot in Ncdu save format.
// It opens the JSON list. Nodes are added with lsNodeNcdu and the list is closed by lsCloseNcdu.
// Format documentation: https://dev.yorhel.nl/ncdu/jsonfmt
func (p *ncduPrinter) ProcessSnapshot(sn *restic.Snapshot) {
	const NcduMajorVer = 1
	const NcduMinorVer = 2

	snapshotBytes, err := json.Marshal(sn)
	if err != nil {
		Warnf("JSON encode failed: %v\n", err)
	}
	p.depth++
	fmt.Fprintf(p.out, "[%d, %d, %s", NcduMajorVer, NcduMinorVer, string(snapshotBytes))
}

func (p *ncduPrinter) ProcessNode(path string, node *restic.Node) {
	type NcduNode struct {
		Name   string `json:"name"`
		Asize  uint64 `json:"asize"`
		Dsize  uint64 `json:"dsize"`
		Dev    uint64 `json:"dev"`
		Ino    uint64 `json:"ino"`
		NLink  uint64 `json:"nlink"`
		NotReg bool   `json:"notreg"`
		Uid    uint32 `json:"uid"`
		Gid    uint32 `json:"gid"`
		Mode   uint16 `json:"mode"`
		Mtime  int64  `json:"mtime"`
	}

	outNode := NcduNode{
		Name:   node.Name,
		Asize:  node.Size,
		Dsize:  node.Size,
		Dev:    node.DeviceID,
		Ino:    node.Inode,
		NLink:  node.Links,
		NotReg: node.Type != "dir" && node.Type != "file",
		Uid:    node.UID,
		Gid:    node.GID,
		Mode:   uint16(node.Mode),
		Mtime:  node.ModTime.Unix(),
	}

	outJson, err := json.Marshal(outNode)
	if err != nil {
		Warnf("JSON encode failed: %v\n", err)
	}

	if node.Type == "dir" {
		p.depth++
		fmt.Fprintf(p.out, ", [\n%s%s", strings.Repeat("  ", p.depth), string(outJson))
	} else {
		fmt.Fprintf(p.out, ",\n%s%s", strings.Repeat("  ", p.depth), string(outJson))
	}
}

func (p *ncduPrinter) LeaveDir(path string) {
	p.depth--
	fmt.Fprintf(p.out, "\n%s]", strings.Repeat("  ", p.depth))
}

func (p *ncduPrinter) Close() {
	fmt.Fprint(p.out, "\n]\n")
}

func runLs(ctx context.Context, opts LsOptions, gopts GlobalOptions, args []string) error {
	if len(args) == 0 {
		return errors.Fatal("no snapshot ID specified, specify snapshot ID or use special ID 'latest'")
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

	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	snapshotLister, err := restic.MemorizeList(ctx, repo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	bar := newIndexProgress(gopts.Quiet, gopts.JSON)
	if err = repo.LoadIndex(ctx, bar); err != nil {
		return err
	}

	var (
		printSnapshot  func(sn *restic.Snapshot)
		printNode      func(path string, node *restic.Node)
		printLeaveNode func(path string)
		printClose     func()
	)

	if gopts.JSON {
		enc := json.NewEncoder(globalOptions.stdout)

		printSnapshot = func(sn *restic.Snapshot) {
			err := enc.Encode(lsSnapshot{
				Snapshot:   sn,
				ID:         sn.ID(),
				ShortID:    sn.ID().Str(),
				StructType: "snapshot",
			})
			if err != nil {
				Warnf("JSON encode failed: %v\n", err)
			}
		}

		printNode = func(path string, node *restic.Node) {
			err := lsNodeJSON(enc, path, node)
			if err != nil {
				Warnf("JSON encode failed: %v\n", err)
			}
		}
	} else if opts.Ncdu {
		ncdu := &ncduPrinter{
			out: globalOptions.stdout,
		}
		printSnapshot = ncdu.ProcessSnapshot
		printNode = ncdu.ProcessNode
		printLeaveNode = ncdu.LeaveDir
		printClose = ncdu.Close
	} else {
		printSnapshot = func(sn *restic.Snapshot) {
			Verbosef("%v filtered by %v:\n", sn, dirs)
		}
		printNode = func(path string, node *restic.Node) {
			Printf("%s\n", formatNode(path, node, opts.ListLong, opts.HumanReadable))
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

	printSnapshot(sn)

	processNode := func(_ restic.ID, nodepath string, node *restic.Node, err error) error {
		if err != nil {
			return err
		}
		if node == nil {
			return nil
		}

		if withinDir(nodepath) {
			// if we're within a dir, print the node
			printNode(nodepath, node)

			// if recursive listing is requested, signal the walker that it
			// should continue walking recursively
			if opts.Recursive {
				return nil
			}
		}

		// if there's an upcoming match deeper in the tree (but we're not
		// there yet), signal the walker to descend into any subdirs
		if approachingMatchingTree(nodepath) {
			return nil
		}

		// otherwise, signal the walker to not walk recursively into any
		// subdirs
		if node.Type == "dir" {
			return walker.ErrSkipNode
		}
		return nil
	}

	err = walker.Walk(ctx, repo, *sn.Tree, walker.WalkVisitor{
		ProcessNode: processNode,
		LeaveDir: func(path string) {
			if printLeaveNode != nil && withinDir(path) && path != "/" {
				printLeaveNode(path)
			}
		},
	})

	if err != nil {
		return err
	}

	if printClose != nil {
		printClose()
	}

	return nil
}
