package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/net/webdav"

	"github.com/restic/restic/internal/bloblru"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
)

var cmdWebdav = &cobra.Command{
	Use:   "webdav [flags] address:port",
	Short: "Serve the repository via WebDAV",
	Long: `
The "webdav" command serves the repository via WebDAV. This is a
read-only mount.

Snapshot Directories
====================

If you need a different template for directories that contain snapshots,
you can pass a time template via --time-template and path templates via
--path-template.

Example time template without colons:

    --time-template "2006-01-02_15-04-05"

You need to specify a sample format for exactly the following timestamp:

    Mon Jan 2 15:04:05 -0700 MST 2006

For details please see the documentation for time.Format() at:
  https://godoc.org/time#Time.Format

For path templates, you can use the following patterns which will be replaced:
    %i by short snapshot ID
    %I by long snapshot ID
    %u by username
    %h by hostname
    %t by tags
    %T by timestamp as specified by --time-template

The default path templates are:
    "ids/%i"
    "snapshots/%T"
    "hosts/%h/%T"
    "tags/%t/%T"

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWebServer(cmd.Context(), webdavOptions, globalOptions, args)
	},
}

type WebdavOptions struct {
	restic.SnapshotFilter
	TimeTemplate  string
	PathTemplates []string
}

var webdavOptions WebdavOptions

func init() {
	cmdRoot.AddCommand(cmdWebdav)
	cmdFlags := cmdWebdav.Flags()
	initMultiSnapshotFilter(cmdFlags, &webdavOptions.SnapshotFilter, true)
	cmdFlags.StringArrayVar(&webdavOptions.PathTemplates, "path-template", nil, "set `template` for path names (can be specified multiple times)")
	cmdFlags.StringVar(&webdavOptions.TimeTemplate, "snapshot-template", time.RFC3339, "set `template` to use for snapshot dirs")
}

func runWebServer(ctx context.Context, opts WebdavOptions, gopts GlobalOptions, args []string) error {

	// PathTemplates and TimeTemplate are ignored for now because `fuse.snapshots_dir(struct)`
	// is not accessible when building on Windows and it would be ridiculous to duplicate the
	// code. It should be shared, somehow.

	if len(args) == 0 {
		return errors.Fatal("wrong number of parameters")
	}

	bindAddress := args[0]

	// FIXME: Proper validation, also check for IPv6
	if strings.Index(bindAddress, "http://") == 0 {
		bindAddress = bindAddress[7:]
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
	err = repo.LoadIndex(ctx, bar)
	if err != nil {
		return err
	}

	davFS := &webdavFS{
		repo: repo,
		root: webdavFSNode{
			name:     "",
			mode:     0555 | os.ModeDir,
			modTime:  time.Now(),
			children: make(map[string]*webdavFSNode),
		},
		snapshots: make(map[string]*restic.Snapshot),
		blobCache: bloblru.New(64 << 20),
	}

	for sn := range FindFilteredSnapshots(ctx, snapshotLister, repo, &webdavOptions.SnapshotFilter, args[1:]) {
		snID := sn.ID().Str()
		davFS.root.children[snID] = &webdavFSNode{
			name:     snID,
			mode:     0555 | os.ModeDir,
			modTime:  sn.Time,
			children: make(map[string]*webdavFSNode),
		}
		davFS.snapshots[snID] = sn
	}

	wd := &webdav.Handler{
		FileSystem: davFS,
		LockSystem: webdav.NewMemLS(),
	}

	Printf("Now serving the repository at http://%s\n", bindAddress)
	Printf("Tree contains %d snapshots\n", len(davFS.root.children))
	Printf("When finished, quit with Ctrl-c here.\n")

	// FIXME: Remove before PR, this is handy for testing but likely undesirable :)
	if runtime.GOOS == "windows" {
		browseURL := "\\\\" + strings.Replace(bindAddress, ":", "@", 1) + "\\DavWWWRoot"
		exec.Command("explorer", browseURL).Start()
	}

	return http.ListenAndServe(bindAddress, wd)
}

type webdavFS struct {
	repo      restic.Repository
	root      webdavFSNode
	snapshots map[string]*restic.Snapshot
	blobCache *bloblru.Cache
}

// Implements os.FileInfo
type webdavFSNode struct {
	name    string
	mode    os.FileMode
	modTime time.Time
	size    int64

	node     *restic.Node
	children map[string]*webdavFSNode
}

func (f *webdavFSNode) Name() string       { return f.name }
func (f *webdavFSNode) Size() int64        { return f.size }
func (f *webdavFSNode) Mode() os.FileMode  { return f.mode }
func (f *webdavFSNode) ModTime() time.Time { return f.modTime }
func (f *webdavFSNode) IsDir() bool        { return f.mode.IsDir() }
func (f *webdavFSNode) Sys() interface{}   { return nil }

func mountSnapshot(ctx context.Context, fs *webdavFS, mountPoint string, sn *restic.Snapshot) {
	Printf("Mounting snapshot %s at %s\n", sn.ID().Str(), mountPoint)
	// FIXME: Need a mutex here...
	// FIXME: All this walking should be done dynamically when the client asks for a folder...
	walker.Walk(ctx, fs.repo, *sn.Tree, walker.WalkVisitor{
		ProcessNode: func(parentTreeID restic.ID, nodepath string, node *restic.Node, err error) error {
			if err != nil || node == nil {
				return err
			}
			dir, wdNode, err := fs.find(mountPoint + "/" + nodepath)
			if err != nil || dir == nil {
				Printf("Found a leaf before the branch was created (parent not found %s)!\n", nodepath)
				return walker.ErrSkipNode
			}
			if wdNode != nil {
				Printf("Walked through a file that already exists in the tree!!! (%s: %s)\n", node.Type, nodepath)
				return walker.ErrSkipNode
			}
			if dir.children == nil {
				dir.children = make(map[string]*webdavFSNode)
			}
			dir.children[node.Name] = &webdavFSNode{
				name:    node.Name,
				mode:    node.Mode,
				modTime: node.ModTime,
				size:    int64(node.Size),
				node:    node,
			}
			dir.size = int64(len(dir.children))
			return nil
		},
	})
}

func (fs *webdavFS) find(fullname string) (*webdavFSNode, *webdavFSNode, error) {
	fullname = strings.Trim(path.Clean("/"+fullname), "/")
	if fullname == "" {
		return nil, &fs.root, nil
	}

	parts := strings.Split(fullname, "/")
	dir := &fs.root

	for dir != nil {
		part := parts[0]
		parts = parts[1:]
		if len(parts) == 0 {
			return dir, dir.children[part], nil
		}
		dir = dir.children[part]
	}

	return nil, nil, os.ErrNotExist
}

func (fs *webdavFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	Printf("OpenFile %s\n", name)

	// Client can only read
	if flag&(os.O_WRONLY|os.O_RDWR) != 0 {
		return nil, os.ErrPermission
	}

	_, node, err := fs.find(name)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, os.ErrNotExist
	}
	return &openFile{node: node, fs: fs}, nil
}

func (fs *webdavFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	_, node, err := fs.find(name)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, os.ErrNotExist
	}
	return node, nil
}

func (fs *webdavFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	return os.ErrPermission
}

func (fs *webdavFS) RemoveAll(ctx context.Context, name string) error {
	return os.ErrPermission
}

func (fs *webdavFS) Rename(ctx context.Context, oldName, newName string) error {
	return os.ErrPermission
}

type openFile struct {
	node   *webdavFSNode
	fs     *webdavFS
	cursor int64

	initialized bool
	// cumsize[i] holds the cumulative size of blobs[:i].
	children []os.FileInfo
	cumsize  []uint64
}

func (f *openFile) getBlobAt(ctx context.Context, i int) (blob []byte, err error) {
	blob, ok := f.fs.blobCache.Get(f.node.node.Content[i])
	if ok {
		return blob, nil
	}

	blob, err = f.fs.repo.LoadBlob(ctx, restic.DataBlob, f.node.node.Content[i], nil)
	if err != nil {
		return nil, err
	}

	f.fs.blobCache.Add(f.node.node.Content[i], blob)

	return blob, nil
}

func (f *openFile) Read(p []byte) (int, error) {
	Printf("Read %s %d %d\n", f.node.name, f.cursor, len(p))
	if f.node.IsDir() || f.cursor < 0 {
		return 0, os.ErrInvalid
	}
	if f.cursor >= f.node.Size() {
		return 0, io.EOF
	}

	// We wait until the first read before we do anything because WebDAV clients tend to open
	// everything and do nothing...
	if !f.initialized {
		var bytes uint64
		cumsize := make([]uint64, 1+len(f.node.node.Content))
		for i, id := range f.node.node.Content {
			size, found := f.fs.repo.LookupBlobSize(id, restic.DataBlob)
			if !found {
				return 0, errors.Errorf("id %v not found in repository", id)
			}
			bytes += uint64(size)
			cumsize[i+1] = bytes
		}
		if bytes != f.node.node.Size {
			Printf("sizes do not match: node.Size %d != size %d", bytes, f.node.Size())
		}
		f.cumsize = cumsize
		f.initialized = true
	}

	offset := uint64(f.cursor)
	remainingBytes := uint64(len(p))
	readBytes := 0

	if offset+remainingBytes > uint64(f.node.Size()) {
		remainingBytes = uint64(f.node.Size()) - remainingBytes
	}

	// Skip blobs before the offset
	startContent := -1 + sort.Search(len(f.cumsize), func(i int) bool {
		return f.cumsize[i] > offset
	})
	offset -= f.cumsize[startContent]

	for i := startContent; remainingBytes > 0 && i < len(f.cumsize)-1; i++ {
		blob, err := f.getBlobAt(context.TODO(), i)
		if err != nil {
			return 0, err
		}

		if offset > 0 {
			blob = blob[offset:]
			offset = 0
		}

		copied := copy(p, blob)
		remainingBytes -= uint64(copied)
		readBytes += copied

		p = p[copied:]
	}

	f.cursor += int64(readBytes)
	return readBytes, nil
}

func (f *openFile) Readdir(count int) ([]os.FileInfo, error) {
	Printf("Readdir %s %d %d\n", f.node.name, f.cursor, count)
	if !f.node.IsDir() || f.cursor < 0 {
		return nil, os.ErrInvalid
	}

	// We wait until the first read before we do anything because WebDAV clients tend to open
	// everything and do nothing...
	if !f.initialized {
		// It's a snapshot, mount it
		if f.node != &f.fs.root && f.node.node == nil && len(f.children) == 0 {
			mountSnapshot(context.TODO(), f.fs, "/"+f.node.name, f.fs.snapshots[f.node.name])
		}
		children := make([]os.FileInfo, 0, len(f.node.children))
		for _, c := range f.node.children {
			children = append(children, c)
		}
		f.children = children
		f.initialized = true
	}

	if count <= 0 {
		return f.children, nil
	}
	if f.cursor >= f.node.Size() {
		return nil, io.EOF
	}
	start := f.cursor
	f.cursor += int64(count)
	if f.cursor > f.node.Size() {
		f.cursor = f.node.Size()
	}
	return f.children[start:f.cursor], nil
}

func (f *openFile) Seek(offset int64, whence int) (int64, error) {
	Printf("Seek %s %d %d\n", f.node.name, offset, whence)
	switch whence {
	case io.SeekStart:
		f.cursor = offset
	case io.SeekCurrent:
		f.cursor += offset
	case io.SeekEnd:
		f.cursor = f.node.Size() - offset
	default:
		return 0, os.ErrInvalid
	}
	return f.cursor, nil
}

func (f *openFile) Stat() (os.FileInfo, error) {
	return f.node, nil
}

func (f *openFile) Write(p []byte) (int, error) {
	return 0, os.ErrPermission
}

func (f *openFile) Close() error {
	return nil
}
