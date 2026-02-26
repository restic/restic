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
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
)

var cmdWebdav = &cobra.Command{
	Use:   "webdav [flags] [ip:port]",
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
	cmdFlags.StringVar(&webdavOptions.TimeTemplate, "snapshot-template", "2006-01-02_15-04-05", "set `template` to use for snapshot dirs")
}

func runWebServer(ctx context.Context, opts WebdavOptions, gopts GlobalOptions, args []string) error {
	if len(args) > 1 {
		return errors.Fatal("wrong number of parameters")
	}

	// FIXME: Proper validation, also add support for IPv6
	bindAddress := "127.0.0.1:3080"
	if len(args) == 1 {
		bindAddress = strings.ToLower(args[0])
	}

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
		blobCache: bloblru.New(64 << 20),
	}

	wd := &webdav.Handler{
		FileSystem: davFS,
		LockSystem: webdav.NewMemLS(),
	}

	for sn := range FindFilteredSnapshots(ctx, snapshotLister, repo, &webdavOptions.SnapshotFilter, nil) {
		node := &webdavFSNode{
			name:     sn.ID().Str(),
			mode:     0555 | os.ModeDir,
			modTime:  sn.Time,
			children: nil,
			snapshot: sn,
		}
		// Ignore PathTemplates for now because `fuse.snapshots_dir(struct)` is not accessible when building
		// on Windows and it would be ridiculous to duplicate the code. It should be shared, somehow!
		davFS.addNode("/ids/"+node.name, node)
		davFS.addNode("/hosts/"+sn.Hostname+"/"+node.name, node)
		davFS.addNode("/snapshots/"+sn.Time.Format(opts.TimeTemplate)+"/"+node.name, node)
		for _, tag := range sn.Tags {
			davFS.addNode("/tags/"+tag+"/"+node.name, node)
		}
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

// Implements webdav.FileSystem
type webdavFS struct {
	repo restic.Repository
	root webdavFSNode
	// snapshots *restic.Snapshot
	blobCache *bloblru.Cache
}

// Implements os.FileInfo
type webdavFSNode struct {
	name     string
	mode     os.FileMode
	modTime  time.Time
	size     int64
	children map[string]*webdavFSNode

	// Should be an interface to save on memory?
	node     *restic.Node
	snapshot *restic.Snapshot
}

func (f *webdavFSNode) Name() string       { return f.name }
func (f *webdavFSNode) Size() int64        { return f.size }
func (f *webdavFSNode) Mode() os.FileMode  { return f.mode }
func (f *webdavFSNode) ModTime() time.Time { return f.modTime }
func (f *webdavFSNode) IsDir() bool        { return f.mode.IsDir() }
func (f *webdavFSNode) Sys() interface{}   { return nil }

func (fs *webdavFS) loadSnapshot(ctx context.Context, mountPoint string, sn *restic.Snapshot) {
	Printf("Loading snapshot %s at %s\n", sn.ID().Str(), mountPoint)
	// FIXME: Need a mutex here...
	// FIXME: All this walking should be done dynamically when the client asks for a folder...
	walker.Walk(ctx, fs.repo, *sn.Tree, walker.WalkVisitor{
		ProcessNode: func(parentTreeID restic.ID, nodepath string, node *restic.Node, err error) error {
			if err != nil || node == nil {
				return err
			}
			fs.addNode(mountPoint+"/"+nodepath, &webdavFSNode{
				name:    node.Name,
				mode:    node.Mode,
				modTime: node.ModTime,
				size:    int64(node.Size),
				node:    node,
				// snapshot: sn,
			})
			return nil
		},
	})
}

func (fs *webdavFS) addNode(fullpath string, node *webdavFSNode) error {
	fullpath = strings.Trim(path.Clean("/"+fullpath), "/")
	if fullpath == "" {
		return os.ErrInvalid
	}

	parts := strings.Split(fullpath, "/")
	dir := &fs.root

	for len(parts) > 0 {
		part := parts[0]
		parts = parts[1:]
		if !dir.IsDir() {
			return os.ErrInvalid
		}
		if dir.children == nil {
			dir.children = make(map[string]*webdavFSNode)
		}
		if len(parts) == 0 {
			dir.children[part] = node
			dir.size = int64(len(dir.children))
			return nil
		}
		if dir.children[part] == nil {
			dir.children[part] = &webdavFSNode{
				name:     part,
				mode:     0555 | os.ModeDir,
				modTime:  dir.modTime,
				children: nil,
			}
		}
		dir = dir.children[part]
	}

	return os.ErrInvalid
}

func (fs *webdavFS) findNode(fullname string) (*webdavFSNode, error) {
	fullname = strings.Trim(path.Clean("/"+fullname), "/")
	if fullname == "" {
		return &fs.root, nil
	}

	parts := strings.Split(fullname, "/")
	dir := &fs.root

	for dir != nil {
		node := dir.children[parts[0]]
		parts = parts[1:]
		if len(parts) == 0 {
			return node, nil
		}
		dir = node
	}

	return nil, os.ErrNotExist
}

func (fs *webdavFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	debug.Log("OpenFile %s", name)

	// Client can only read
	if flag&(os.O_WRONLY|os.O_RDWR) != 0 {
		return nil, os.ErrPermission
	}

	node, err := fs.findNode(name)
	if err == os.ErrNotExist {
		// FIXME: Walk up the tree to make sure the snapshot (if any) is loaded
	}
	if err != nil {
		return nil, err
	}
	return &openFile{fullpath: path.Clean("/" + name), node: node, fs: fs}, nil
}

func (fs *webdavFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	node, err := fs.findNode(name)
	if err != nil {
		return nil, err
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
	fullpath string
	node     *webdavFSNode
	fs       *webdavFS
	cursor   int64
	children []os.FileInfo
	// cumsize[i] holds the cumulative size of blobs[:i].
	cumsize []uint64

	initialized bool
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
	debug.Log("Read %s %d %d", f.fullpath, f.cursor, len(p))
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
	debug.Log("Readdir %s %d %d", f.fullpath, f.cursor, count)
	if !f.node.IsDir() || f.cursor < 0 {
		return nil, os.ErrInvalid
	}

	// We wait until the first read before we do anything because WebDAV clients tend to open
	// everything and do nothing...
	if !f.initialized {
		// It's a snapshot, mount it
		if f.node.snapshot != nil && f.node.children == nil {
			f.fs.loadSnapshot(context.TODO(), f.fullpath, f.node.snapshot)
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
	debug.Log("Seek %s %d %d", f.fullpath, offset, whence)
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
