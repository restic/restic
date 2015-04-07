// FUSE service loop, for servers that wish to use it.

package fs

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"io"
	"reflect"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"
)

import (
	"bazil.org/fuse"
	"bazil.org/fuse/fuseutil"
)

const (
	attrValidTime  = 1 * time.Minute
	entryValidTime = 1 * time.Minute
)

// TODO: FINISH DOCS

// An FS is the interface required of a file system.
//
// Other FUSE requests can be handled by implementing methods from the
// FS* interfaces, for example FSIniter.
type FS interface {
	// Root is called to obtain the Node for the file system root.
	Root() (Node, error)
}

type FSIniter interface {
	// Init is called to initialize the FUSE connection.
	// It can inspect the request and adjust the response as desired.
	// Init must return promptly.
	Init(ctx context.Context, req *fuse.InitRequest, resp *fuse.InitResponse) error
}

type FSStatfser interface {
	// Statfs is called to obtain file system metadata.
	// It should write that data to resp.
	Statfs(ctx context.Context, req *fuse.StatfsRequest, resp *fuse.StatfsResponse) error
}

type FSDestroyer interface {
	// Destroy is called when the file system is shutting down.
	//
	// Linux only sends this request for block device backed (fuseblk)
	// filesystems, to allow them to flush writes to disk before the
	// unmount completes.
	Destroy()
}

type FSInodeGenerator interface {
	// GenerateInode is called to pick a dynamic inode number when it
	// would otherwise be 0.
	//
	// Not all filesystems bother tracking inodes, but FUSE requires
	// the inode to be set, and fewer duplicates in general makes UNIX
	// tools work better.
	//
	// Operations where the nodes may return 0 inodes include Getattr,
	// Setattr and ReadDir.
	//
	// If FS does not implement FSInodeGenerator, GenerateDynamicInode
	// is used.
	//
	// Implementing this is useful to e.g. constrain the range of
	// inode values used for dynamic inodes.
	GenerateInode(parentInode uint64, name string) uint64
}

// A Node is the interface required of a file or directory.
// See the documentation for type FS for general information
// pertaining to all methods.
//
// Other FUSE requests can be handled by implementing methods from the
// Node* interfaces, for example NodeOpener.
type Node interface {
	Attr(*fuse.Attr)
}

type NodeGetattrer interface {
	// Getattr obtains the standard metadata for the receiver.
	// It should store that metadata in resp.
	//
	// If this method is not implemented, the attributes will be
	// generated based on Attr(), with zero values filled in.
	Getattr(ctx context.Context, req *fuse.GetattrRequest, resp *fuse.GetattrResponse) error
}

type NodeSetattrer interface {
	// Setattr sets the standard metadata for the receiver.
	//
	// Note, this is also used to communicate changes in the size of
	// the file. Not implementing Setattr causes writes to be unable
	// to grow the file (except with OpenDirectIO, which bypasses that
	// mechanism).
	//
	// req.Valid is a bitmask of what fields are actually being set.
	// For example, the method should not change the mode of the file
	// unless req.Valid.Mode() is true.
	Setattr(ctx context.Context, req *fuse.SetattrRequest, resp *fuse.SetattrResponse) error
}

type NodeSymlinker interface {
	// Symlink creates a new symbolic link in the receiver, which must be a directory.
	//
	// TODO is the above true about directories?
	Symlink(ctx context.Context, req *fuse.SymlinkRequest) (Node, error)
}

// This optional request will be called only for symbolic link nodes.
type NodeReadlinker interface {
	// Readlink reads a symbolic link.
	Readlink(ctx context.Context, req *fuse.ReadlinkRequest) (string, error)
}

type NodeLinker interface {
	// Link creates a new directory entry in the receiver based on an
	// existing Node. Receiver must be a directory.
	Link(ctx context.Context, req *fuse.LinkRequest, old Node) (Node, error)
}

type NodeRemover interface {
	// Remove removes the entry with the given name from
	// the receiver, which must be a directory.  The entry to be removed
	// may correspond to a file (unlink) or to a directory (rmdir).
	Remove(ctx context.Context, req *fuse.RemoveRequest) error
}

type NodeAccesser interface {
	// Access checks whether the calling context has permission for
	// the given operations on the receiver. If so, Access should
	// return nil. If not, Access should return EPERM.
	//
	// Note that this call affects the result of the access(2) system
	// call but not the open(2) system call. If Access is not
	// implemented, the Node behaves as if it always returns nil
	// (permission granted), relying on checks in Open instead.
	Access(ctx context.Context, req *fuse.AccessRequest) error
}

type NodeStringLookuper interface {
	// Lookup looks up a specific entry in the receiver,
	// which must be a directory.  Lookup should return a Node
	// corresponding to the entry.  If the name does not exist in
	// the directory, Lookup should return nil, err.
	//
	// Lookup need not to handle the names "." and "..".
	Lookup(ctx context.Context, name string) (Node, error)
}

type NodeRequestLookuper interface {
	// Lookup looks up a specific entry in the receiver.
	// See NodeStringLookuper for more.
	Lookup(ctx context.Context, req *fuse.LookupRequest, resp *fuse.LookupResponse) (Node, error)
}

type NodeMkdirer interface {
	Mkdir(ctx context.Context, req *fuse.MkdirRequest) (Node, error)
}

type NodeOpener interface {
	// Open opens the receiver. After a successful open, a client
	// process has a file descriptor referring to this Handle.
	//
	// Open can also be also called on non-files. For example,
	// directories are Opened for ReadDir or fchdir(2).
	//
	// If this method is not implemented, the open will always
	// succeed, and the Node itself will be used as the Handle.
	//
	// XXX note about access.  XXX OpenFlags.
	Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (Handle, error)
}

type NodeCreater interface {
	// Create creates a new directory entry in the receiver, which
	// must be a directory.
	Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (Node, Handle, error)
}

type NodeForgetter interface {
	// Forget about this node. This node will not receive further
	// method calls.
	//
	// Forget is not necessarily seen on unmount, as all nodes are
	// implicitly forgotten as part part of the unmount.
	Forget()
}

type NodeRenamer interface {
	Rename(ctx context.Context, req *fuse.RenameRequest, newDir Node) error
}

type NodeMknoder interface {
	Mknod(ctx context.Context, req *fuse.MknodRequest) (Node, error)
}

// TODO this should be on Handle not Node
type NodeFsyncer interface {
	Fsync(ctx context.Context, req *fuse.FsyncRequest) error
}

type NodeGetxattrer interface {
	// Getxattr gets an extended attribute by the given name from the
	// node.
	//
	// If there is no xattr by that name, returns fuse.ErrNoXattr.
	Getxattr(ctx context.Context, req *fuse.GetxattrRequest, resp *fuse.GetxattrResponse) error
}

type NodeListxattrer interface {
	// Listxattr lists the extended attributes recorded for the node.
	Listxattr(ctx context.Context, req *fuse.ListxattrRequest, resp *fuse.ListxattrResponse) error
}

type NodeSetxattrer interface {
	// Setxattr sets an extended attribute with the given name and
	// value for the node.
	Setxattr(ctx context.Context, req *fuse.SetxattrRequest) error
}

type NodeRemovexattrer interface {
	// Removexattr removes an extended attribute for the name.
	//
	// If there is no xattr by that name, returns fuse.ErrNoXattr.
	Removexattr(ctx context.Context, req *fuse.RemovexattrRequest) error
}

var startTime = time.Now()

func nodeAttr(n Node) (attr fuse.Attr) {
	attr.Nlink = 1
	attr.Atime = startTime
	attr.Mtime = startTime
	attr.Ctime = startTime
	attr.Crtime = startTime
	n.Attr(&attr)
	return
}

// A Handle is the interface required of an opened file or directory.
// See the documentation for type FS for general information
// pertaining to all methods.
//
// Other FUSE requests can be handled by implementing methods from the
// Handle* interfaces. The most common to implement are HandleReader,
// HandleReadDirer, and HandleWriter.
//
// TODO implement methods: Getlk, Setlk, Setlkw
type Handle interface {
}

type HandleFlusher interface {
	// Flush is called each time the file or directory is closed.
	// Because there can be multiple file descriptors referring to a
	// single opened file, Flush can be called multiple times.
	Flush(ctx context.Context, req *fuse.FlushRequest) error
}

type HandleReadAller interface {
	ReadAll(ctx context.Context) ([]byte, error)
}

type HandleReadDirAller interface {
	ReadDirAll(ctx context.Context) ([]fuse.Dirent, error)
}

type HandleReader interface {
	// Read requests to read data from the handle.
	//
	// There is a page cache in the kernel that normally submits only
	// page-aligned reads spanning one or more pages. However, you
	// should not rely on this. To see individual requests as
	// submitted by the file system clients, set OpenDirectIO.
	//
	// Note that reads beyond the size of the file as reported by Attr
	// are not even attempted (except in OpenDirectIO mode).
	Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error
}

type HandleWriter interface {
	// Write requests to write data into the handle.
	//
	// There is a writeback page cache in the kernel that normally submits
	// only page-aligned writes spanning one or more pages. However,
	// you should not rely on this. To see individual requests as
	// submitted by the file system clients, set OpenDirectIO.
	//
	// Note that file size changes are communicated through Setattr.
	// Writes beyond the size of the file as reported by Attr are not
	// even attempted (except in OpenDirectIO mode).
	Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error
}

type HandleReleaser interface {
	Release(ctx context.Context, req *fuse.ReleaseRequest) error
}

type Server struct {
	FS FS

	// Function to send debug log messages to. If nil, use fuse.Debug.
	// Note that changing this or fuse.Debug may not affect existing
	// calls to Serve.
	//
	// See fuse.Debug for the rules that log functions must follow.
	Debug func(msg interface{})
}

// Serve serves the FUSE connection by making calls to the methods
// of fs and the Nodes and Handles it makes available.  It returns only
// when the connection has been closed or an unexpected error occurs.
func (s *Server) Serve(c *fuse.Conn) error {
	sc := serveConn{
		fs:           s.FS,
		debug:        s.Debug,
		req:          map[fuse.RequestID]*serveRequest{},
		dynamicInode: GenerateDynamicInode,
	}
	if sc.debug == nil {
		sc.debug = fuse.Debug
	}
	if dyn, ok := sc.fs.(FSInodeGenerator); ok {
		sc.dynamicInode = dyn.GenerateInode
	}

	root, err := sc.fs.Root()
	if err != nil {
		return fmt.Errorf("cannot obtain root node: %v", err)
	}
	sc.node = append(sc.node, nil, &serveNode{inode: 1, node: root, refs: 1})
	sc.handle = append(sc.handle, nil)

	for {
		req, err := c.ReadRequest()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		go sc.serve(req)
	}
	return nil
}

// Serve serves a FUSE connection with the default settings. See
// Server.Serve.
func Serve(c *fuse.Conn, fs FS) error {
	server := Server{
		FS: fs,
	}
	return server.Serve(c)
}

type nothing struct{}

type serveConn struct {
	meta         sync.Mutex
	fs           FS
	req          map[fuse.RequestID]*serveRequest
	node         []*serveNode
	handle       []*serveHandle
	freeNode     []fuse.NodeID
	freeHandle   []fuse.HandleID
	nodeGen      uint64
	debug        func(msg interface{})
	dynamicInode func(parent uint64, name string) uint64
}

type serveRequest struct {
	Request fuse.Request
	cancel  func()
}

type serveNode struct {
	inode uint64
	node  Node
	refs  uint64
}

func (sn *serveNode) attr() (attr fuse.Attr) {
	attr = nodeAttr(sn.node)
	if attr.Inode == 0 {
		attr.Inode = sn.inode
	}
	return
}

type serveHandle struct {
	handle   Handle
	readData []byte
	nodeID   fuse.NodeID
}

// NodeRef can be embedded in a Node to recognize the same Node being
// returned from multiple Lookup, Create etc calls.
//
// Without this, each Node will get a new NodeID, causing spurious
// cache invalidations, extra lookups and aliasing anomalies. This may
// not matter for a simple, read-only filesystem.
type NodeRef struct {
	id         fuse.NodeID
	generation uint64
}

// nodeRef is only ever accessed while holding serveConn.meta
func (n *NodeRef) nodeRef() *NodeRef {
	return n
}

type nodeRef interface {
	nodeRef() *NodeRef
}

func (c *serveConn) saveNode(inode uint64, node Node) (id fuse.NodeID, gen uint64) {
	c.meta.Lock()
	defer c.meta.Unlock()

	var ref *NodeRef
	if nodeRef, ok := node.(nodeRef); ok {
		ref = nodeRef.nodeRef()

		if ref.id != 0 {
			// dropNode guarantees that NodeRef is zeroed at the same
			// time as the NodeID is removed from serveConn.node, as
			// guarded by c.meta; this means sn cannot be nil here
			sn := c.node[ref.id]
			sn.refs++
			return ref.id, ref.generation
		}
	}

	sn := &serveNode{inode: inode, node: node, refs: 1}
	if n := len(c.freeNode); n > 0 {
		id = c.freeNode[n-1]
		c.freeNode = c.freeNode[:n-1]
		c.node[id] = sn
		c.nodeGen++
	} else {
		id = fuse.NodeID(len(c.node))
		c.node = append(c.node, sn)
	}
	gen = c.nodeGen
	if ref != nil {
		ref.id = id
		ref.generation = gen
	}
	return
}

func (c *serveConn) saveHandle(handle Handle, nodeID fuse.NodeID) (id fuse.HandleID) {
	c.meta.Lock()
	shandle := &serveHandle{handle: handle, nodeID: nodeID}
	if n := len(c.freeHandle); n > 0 {
		id = c.freeHandle[n-1]
		c.freeHandle = c.freeHandle[:n-1]
		c.handle[id] = shandle
	} else {
		id = fuse.HandleID(len(c.handle))
		c.handle = append(c.handle, shandle)
	}
	c.meta.Unlock()
	return
}

type nodeRefcountDropBug struct {
	N    uint64
	Refs uint64
	Node fuse.NodeID
}

func (n *nodeRefcountDropBug) String() string {
	return fmt.Sprintf("bug: trying to drop %d of %d references to %v", n.N, n.Refs, n.Node)
}

func (c *serveConn) dropNode(id fuse.NodeID, n uint64) (forget bool) {
	c.meta.Lock()
	defer c.meta.Unlock()
	snode := c.node[id]

	if snode == nil {
		// this should only happen if refcounts kernel<->us disagree
		// *and* two ForgetRequests for the same node race each other;
		// this indicates a bug somewhere
		c.debug(nodeRefcountDropBug{N: n, Node: id})

		// we may end up triggering Forget twice, but that's better
		// than not even once, and that's the best we can do
		return true
	}

	if n > snode.refs {
		c.debug(nodeRefcountDropBug{N: n, Refs: snode.refs, Node: id})
		n = snode.refs
	}

	snode.refs -= n
	if snode.refs == 0 {
		c.node[id] = nil
		if nodeRef, ok := snode.node.(nodeRef); ok {
			ref := nodeRef.nodeRef()
			*ref = NodeRef{}
		}
		c.freeNode = append(c.freeNode, id)
		return true
	}
	return false
}

func (c *serveConn) dropHandle(id fuse.HandleID) {
	c.meta.Lock()
	c.handle[id] = nil
	c.freeHandle = append(c.freeHandle, id)
	c.meta.Unlock()
}

type missingHandle struct {
	Handle    fuse.HandleID
	MaxHandle fuse.HandleID
}

func (m missingHandle) String() string {
	return fmt.Sprint("missing handle", m.Handle, m.MaxHandle)
}

// Returns nil for invalid handles.
func (c *serveConn) getHandle(id fuse.HandleID) (shandle *serveHandle) {
	c.meta.Lock()
	defer c.meta.Unlock()
	if id < fuse.HandleID(len(c.handle)) {
		shandle = c.handle[uint(id)]
	}
	if shandle == nil {
		c.debug(missingHandle{
			Handle:    id,
			MaxHandle: fuse.HandleID(len(c.handle)),
		})
	}
	return
}

type request struct {
	Op      string
	Request *fuse.Header
	In      interface{} `json:",omitempty"`
}

func (r request) String() string {
	return fmt.Sprintf("<- %s", r.In)
}

type logResponseHeader struct {
	ID fuse.RequestID
}

func (m logResponseHeader) String() string {
	return fmt.Sprintf("ID=%#x", m.ID)
}

type response struct {
	Op      string
	Request logResponseHeader
	Out     interface{} `json:",omitempty"`
	// Errno contains the errno value as a string, for example "EPERM".
	Errno string `json:",omitempty"`
	// Error may contain a free form error message.
	Error string `json:",omitempty"`
}

func (r response) errstr() string {
	s := r.Errno
	if r.Error != "" {
		// prefix the errno constant to the long form message
		s = s + ": " + r.Error
	}
	return s
}

func (r response) String() string {
	switch {
	case r.Errno != "" && r.Out != nil:
		return fmt.Sprintf("-> %s error=%s %s", r.Request, r.errstr(), r.Out)
	case r.Errno != "":
		return fmt.Sprintf("-> %s error=%s", r.Request, r.errstr())
	case r.Out != nil:
		// make sure (seemingly) empty values are readable
		switch r.Out.(type) {
		case string:
			return fmt.Sprintf("-> %s %q", r.Request, r.Out)
		case []byte:
			return fmt.Sprintf("-> %s [% x]", r.Request, r.Out)
		default:
			return fmt.Sprintf("-> %s %s", r.Request, r.Out)
		}
	default:
		return fmt.Sprintf("-> %s", r.Request)
	}
}

type logMissingNode struct {
	MaxNode fuse.NodeID
}

func opName(req fuse.Request) string {
	t := reflect.Indirect(reflect.ValueOf(req)).Type()
	s := t.Name()
	s = strings.TrimSuffix(s, "Request")
	return s
}

type logLinkRequestOldNodeNotFound struct {
	Request *fuse.Header
	In      *fuse.LinkRequest
}

func (m *logLinkRequestOldNodeNotFound) String() string {
	return fmt.Sprintf("In LinkRequest (request %#x), node %d not found", m.Request.Hdr().ID, m.In.OldNode)
}

type renameNewDirNodeNotFound struct {
	Request *fuse.Header
	In      *fuse.RenameRequest
}

func (m *renameNewDirNodeNotFound) String() string {
	return fmt.Sprintf("In RenameRequest (request %#x), node %d not found", m.Request.Hdr().ID, m.In.NewDir)
}

func (c *serveConn) serve(r fuse.Request) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := &serveRequest{Request: r, cancel: cancel}

	c.debug(request{
		Op:      opName(r),
		Request: r.Hdr(),
		In:      r,
	})
	var node Node
	var snode *serveNode
	c.meta.Lock()
	hdr := r.Hdr()
	if id := hdr.Node; id != 0 {
		if id < fuse.NodeID(len(c.node)) {
			snode = c.node[uint(id)]
		}
		if snode == nil {
			c.meta.Unlock()
			c.debug(response{
				Op:      opName(r),
				Request: logResponseHeader{ID: hdr.ID},
				Error:   fuse.ESTALE.ErrnoName(),
				// this is the only place that sets both Error and
				// Out; not sure if i want to do that; might get rid
				// of len(c.node) things altogether
				Out: logMissingNode{
					MaxNode: fuse.NodeID(len(c.node)),
				},
			})
			r.RespondError(fuse.ESTALE)
			return
		}
		node = snode.node
	}
	if c.req[hdr.ID] != nil {
		// This happens with OSXFUSE.  Assume it's okay and
		// that we'll never see an interrupt for this one.
		// Otherwise everything wedges.  TODO: Report to OSXFUSE?
		//
		// TODO this might have been because of missing done() calls
	} else {
		c.req[hdr.ID] = req
	}
	c.meta.Unlock()

	// Call this before responding.
	// After responding is too late: we might get another request
	// with the same ID and be very confused.
	done := func(resp interface{}) {
		msg := response{
			Op:      opName(r),
			Request: logResponseHeader{ID: hdr.ID},
		}
		if err, ok := resp.(error); ok {
			msg.Error = err.Error()
			if ferr, ok := err.(fuse.ErrorNumber); ok {
				errno := ferr.Errno()
				msg.Errno = errno.ErrnoName()
				if errno == err {
					// it's just a fuse.Errno with no extra detail;
					// skip the textual message for log readability
					msg.Error = ""
				}
			} else {
				msg.Errno = fuse.DefaultErrno.ErrnoName()
			}
		} else {
			msg.Out = resp
		}
		c.debug(msg)

		c.meta.Lock()
		delete(c.req, hdr.ID)
		c.meta.Unlock()
	}

	switch r := r.(type) {
	default:
		// Note: To FUSE, ENOSYS means "this server never implements this request."
		// It would be inappropriate to return ENOSYS for other operations in this
		// switch that might only be unavailable in some contexts, not all.
		done(fuse.ENOSYS)
		r.RespondError(fuse.ENOSYS)

	// FS operations.
	case *fuse.InitRequest:
		s := &fuse.InitResponse{
			MaxWrite: 128 * 1024,
			Flags:    fuse.InitBigWrites,
		}
		if fs, ok := c.fs.(FSIniter); ok {
			if err := fs.Init(ctx, r, s); err != nil {
				done(err)
				r.RespondError(err)
				break
			}
		}
		done(s)
		r.Respond(s)

	case *fuse.StatfsRequest:
		s := &fuse.StatfsResponse{}
		if fs, ok := c.fs.(FSStatfser); ok {
			if err := fs.Statfs(ctx, r, s); err != nil {
				done(err)
				r.RespondError(err)
				break
			}
		}
		done(s)
		r.Respond(s)

	// Node operations.
	case *fuse.GetattrRequest:
		s := &fuse.GetattrResponse{}
		if n, ok := node.(NodeGetattrer); ok {
			if err := n.Getattr(ctx, r, s); err != nil {
				done(err)
				r.RespondError(err)
				break
			}
		} else {
			s.AttrValid = attrValidTime
			s.Attr = snode.attr()
		}
		done(s)
		r.Respond(s)

	case *fuse.SetattrRequest:
		s := &fuse.SetattrResponse{}
		if n, ok := node.(NodeSetattrer); ok {
			if err := n.Setattr(ctx, r, s); err != nil {
				done(err)
				r.RespondError(err)
				break
			}
			done(s)
			r.Respond(s)
			break
		}

		if s.AttrValid == 0 {
			s.AttrValid = attrValidTime
		}
		s.Attr = snode.attr()
		done(s)
		r.Respond(s)

	case *fuse.SymlinkRequest:
		s := &fuse.SymlinkResponse{}
		n, ok := node.(NodeSymlinker)
		if !ok {
			done(fuse.EIO) // XXX or EPERM like Mkdir?
			r.RespondError(fuse.EIO)
			break
		}
		n2, err := n.Symlink(ctx, r)
		if err != nil {
			done(err)
			r.RespondError(err)
			break
		}
		c.saveLookup(&s.LookupResponse, snode, r.NewName, n2)
		done(s)
		r.Respond(s)

	case *fuse.ReadlinkRequest:
		n, ok := node.(NodeReadlinker)
		if !ok {
			done(fuse.EIO) /// XXX or EPERM?
			r.RespondError(fuse.EIO)
			break
		}
		target, err := n.Readlink(ctx, r)
		if err != nil {
			done(err)
			r.RespondError(err)
			break
		}
		done(target)
		r.Respond(target)

	case *fuse.LinkRequest:
		n, ok := node.(NodeLinker)
		if !ok {
			done(fuse.EIO) /// XXX or EPERM?
			r.RespondError(fuse.EIO)
			break
		}
		c.meta.Lock()
		var oldNode *serveNode
		if int(r.OldNode) < len(c.node) {
			oldNode = c.node[r.OldNode]
		}
		c.meta.Unlock()
		if oldNode == nil {
			c.debug(logLinkRequestOldNodeNotFound{
				Request: r.Hdr(),
				In:      r,
			})
			done(fuse.EIO)
			r.RespondError(fuse.EIO)
			break
		}
		n2, err := n.Link(ctx, r, oldNode.node)
		if err != nil {
			done(err)
			r.RespondError(err)
			break
		}
		s := &fuse.LookupResponse{}
		c.saveLookup(s, snode, r.NewName, n2)
		done(s)
		r.Respond(s)

	case *fuse.RemoveRequest:
		n, ok := node.(NodeRemover)
		if !ok {
			done(fuse.EIO) /// XXX or EPERM?
			r.RespondError(fuse.EIO)
			break
		}
		err := n.Remove(ctx, r)
		if err != nil {
			done(err)
			r.RespondError(err)
			break
		}
		done(nil)
		r.Respond()

	case *fuse.AccessRequest:
		if n, ok := node.(NodeAccesser); ok {
			if err := n.Access(ctx, r); err != nil {
				done(err)
				r.RespondError(err)
				break
			}
		}
		done(nil)
		r.Respond()

	case *fuse.LookupRequest:
		var n2 Node
		var err error
		s := &fuse.LookupResponse{}
		if n, ok := node.(NodeStringLookuper); ok {
			n2, err = n.Lookup(ctx, r.Name)
		} else if n, ok := node.(NodeRequestLookuper); ok {
			n2, err = n.Lookup(ctx, r, s)
		} else {
			done(fuse.ENOENT)
			r.RespondError(fuse.ENOENT)
			break
		}
		if err != nil {
			done(err)
			r.RespondError(err)
			break
		}
		c.saveLookup(s, snode, r.Name, n2)
		done(s)
		r.Respond(s)

	case *fuse.MkdirRequest:
		s := &fuse.MkdirResponse{}
		n, ok := node.(NodeMkdirer)
		if !ok {
			done(fuse.EPERM)
			r.RespondError(fuse.EPERM)
			break
		}
		n2, err := n.Mkdir(ctx, r)
		if err != nil {
			done(err)
			r.RespondError(err)
			break
		}
		c.saveLookup(&s.LookupResponse, snode, r.Name, n2)
		done(s)
		r.Respond(s)

	case *fuse.OpenRequest:
		s := &fuse.OpenResponse{}
		var h2 Handle
		if n, ok := node.(NodeOpener); ok {
			hh, err := n.Open(ctx, r, s)
			if err != nil {
				done(err)
				r.RespondError(err)
				break
			}
			h2 = hh
		} else {
			h2 = node
		}
		s.Handle = c.saveHandle(h2, hdr.Node)
		done(s)
		r.Respond(s)

	case *fuse.CreateRequest:
		n, ok := node.(NodeCreater)
		if !ok {
			// If we send back ENOSYS, FUSE will try mknod+open.
			done(fuse.EPERM)
			r.RespondError(fuse.EPERM)
			break
		}
		s := &fuse.CreateResponse{OpenResponse: fuse.OpenResponse{}}
		n2, h2, err := n.Create(ctx, r, s)
		if err != nil {
			done(err)
			r.RespondError(err)
			break
		}
		c.saveLookup(&s.LookupResponse, snode, r.Name, n2)
		s.Handle = c.saveHandle(h2, hdr.Node)
		done(s)
		r.Respond(s)

	case *fuse.GetxattrRequest:
		n, ok := node.(NodeGetxattrer)
		if !ok {
			done(fuse.ENOTSUP)
			r.RespondError(fuse.ENOTSUP)
			break
		}
		s := &fuse.GetxattrResponse{}
		err := n.Getxattr(ctx, r, s)
		if err != nil {
			done(err)
			r.RespondError(err)
			break
		}
		if r.Size != 0 && uint64(len(s.Xattr)) > uint64(r.Size) {
			done(fuse.ERANGE)
			r.RespondError(fuse.ERANGE)
			break
		}
		done(s)
		r.Respond(s)

	case *fuse.ListxattrRequest:
		n, ok := node.(NodeListxattrer)
		if !ok {
			done(fuse.ENOTSUP)
			r.RespondError(fuse.ENOTSUP)
			break
		}
		s := &fuse.ListxattrResponse{}
		err := n.Listxattr(ctx, r, s)
		if err != nil {
			done(err)
			r.RespondError(err)
			break
		}
		if r.Size != 0 && uint64(len(s.Xattr)) > uint64(r.Size) {
			done(fuse.ERANGE)
			r.RespondError(fuse.ERANGE)
			break
		}
		done(s)
		r.Respond(s)

	case *fuse.SetxattrRequest:
		n, ok := node.(NodeSetxattrer)
		if !ok {
			done(fuse.ENOTSUP)
			r.RespondError(fuse.ENOTSUP)
			break
		}
		err := n.Setxattr(ctx, r)
		if err != nil {
			done(err)
			r.RespondError(err)
			break
		}
		done(nil)
		r.Respond()

	case *fuse.RemovexattrRequest:
		n, ok := node.(NodeRemovexattrer)
		if !ok {
			done(fuse.ENOTSUP)
			r.RespondError(fuse.ENOTSUP)
			break
		}
		err := n.Removexattr(ctx, r)
		if err != nil {
			done(err)
			r.RespondError(err)
			break
		}
		done(nil)
		r.Respond()

	case *fuse.ForgetRequest:
		forget := c.dropNode(hdr.Node, r.N)
		if forget {
			n, ok := node.(NodeForgetter)
			if ok {
				n.Forget()
			}
		}
		done(nil)
		r.Respond()

	// Handle operations.
	case *fuse.ReadRequest:
		shandle := c.getHandle(r.Handle)
		if shandle == nil {
			done(fuse.ESTALE)
			r.RespondError(fuse.ESTALE)
			return
		}
		handle := shandle.handle

		s := &fuse.ReadResponse{Data: make([]byte, 0, r.Size)}
		if r.Dir {
			if h, ok := handle.(HandleReadDirAller); ok {
				if shandle.readData == nil {
					dirs, err := h.ReadDirAll(ctx)
					if err != nil {
						done(err)
						r.RespondError(err)
						break
					}
					var data []byte
					for _, dir := range dirs {
						if dir.Inode == 0 {
							dir.Inode = c.dynamicInode(snode.inode, dir.Name)
						}
						data = fuse.AppendDirent(data, dir)
					}
					shandle.readData = data
				}
				fuseutil.HandleRead(r, s, shandle.readData)
				done(s)
				r.Respond(s)
				break
			}
		} else {
			if h, ok := handle.(HandleReadAller); ok {
				if shandle.readData == nil {
					data, err := h.ReadAll(ctx)
					if err != nil {
						done(err)
						r.RespondError(err)
						break
					}
					if data == nil {
						data = []byte{}
					}
					shandle.readData = data
				}
				fuseutil.HandleRead(r, s, shandle.readData)
				done(s)
				r.Respond(s)
				break
			}
			h, ok := handle.(HandleReader)
			if !ok {
				fmt.Printf("NO READ FOR %T\n", handle)
				done(fuse.EIO)
				r.RespondError(fuse.EIO)
				break
			}
			if err := h.Read(ctx, r, s); err != nil {
				done(err)
				r.RespondError(err)
				break
			}
		}
		done(s)
		r.Respond(s)

	case *fuse.WriteRequest:
		shandle := c.getHandle(r.Handle)
		if shandle == nil {
			done(fuse.ESTALE)
			r.RespondError(fuse.ESTALE)
			return
		}

		s := &fuse.WriteResponse{}
		if h, ok := shandle.handle.(HandleWriter); ok {
			if err := h.Write(ctx, r, s); err != nil {
				done(err)
				r.RespondError(err)
				break
			}
			done(s)
			r.Respond(s)
			break
		}
		done(fuse.EIO)
		r.RespondError(fuse.EIO)

	case *fuse.FlushRequest:
		shandle := c.getHandle(r.Handle)
		if shandle == nil {
			done(fuse.ESTALE)
			r.RespondError(fuse.ESTALE)
			return
		}
		handle := shandle.handle

		if h, ok := handle.(HandleFlusher); ok {
			if err := h.Flush(ctx, r); err != nil {
				done(err)
				r.RespondError(err)
				break
			}
		}
		done(nil)
		r.Respond()

	case *fuse.ReleaseRequest:
		shandle := c.getHandle(r.Handle)
		if shandle == nil {
			done(fuse.ESTALE)
			r.RespondError(fuse.ESTALE)
			return
		}
		handle := shandle.handle

		// No matter what, release the handle.
		c.dropHandle(r.Handle)

		if h, ok := handle.(HandleReleaser); ok {
			if err := h.Release(ctx, r); err != nil {
				done(err)
				r.RespondError(err)
				break
			}
		}
		done(nil)
		r.Respond()

	case *fuse.DestroyRequest:
		if fs, ok := c.fs.(FSDestroyer); ok {
			fs.Destroy()
		}
		done(nil)
		r.Respond()

	case *fuse.RenameRequest:
		c.meta.Lock()
		var newDirNode *serveNode
		if int(r.NewDir) < len(c.node) {
			newDirNode = c.node[r.NewDir]
		}
		c.meta.Unlock()
		if newDirNode == nil {
			c.debug(renameNewDirNodeNotFound{
				Request: r.Hdr(),
				In:      r,
			})
			done(fuse.EIO)
			r.RespondError(fuse.EIO)
			break
		}
		n, ok := node.(NodeRenamer)
		if !ok {
			done(fuse.EIO) // XXX or EPERM like Mkdir?
			r.RespondError(fuse.EIO)
			break
		}
		err := n.Rename(ctx, r, newDirNode.node)
		if err != nil {
			done(err)
			r.RespondError(err)
			break
		}
		done(nil)
		r.Respond()

	case *fuse.MknodRequest:
		n, ok := node.(NodeMknoder)
		if !ok {
			done(fuse.EIO)
			r.RespondError(fuse.EIO)
			break
		}
		n2, err := n.Mknod(ctx, r)
		if err != nil {
			done(err)
			r.RespondError(err)
			break
		}
		s := &fuse.LookupResponse{}
		c.saveLookup(s, snode, r.Name, n2)
		done(s)
		r.Respond(s)

	case *fuse.FsyncRequest:
		n, ok := node.(NodeFsyncer)
		if !ok {
			done(fuse.EIO)
			r.RespondError(fuse.EIO)
			break
		}
		err := n.Fsync(ctx, r)
		if err != nil {
			done(err)
			r.RespondError(err)
			break
		}
		done(nil)
		r.Respond()

	case *fuse.InterruptRequest:
		c.meta.Lock()
		ireq := c.req[r.IntrID]
		if ireq != nil && ireq.cancel != nil {
			ireq.cancel()
			ireq.cancel = nil
		}
		c.meta.Unlock()
		done(nil)
		r.Respond()

		/*	case *FsyncdirRequest:
				done(ENOSYS)
				r.RespondError(ENOSYS)

			case *GetlkRequest, *SetlkRequest, *SetlkwRequest:
				done(ENOSYS)
				r.RespondError(ENOSYS)

			case *BmapRequest:
				done(ENOSYS)
				r.RespondError(ENOSYS)

			case *SetvolnameRequest, *GetxtimesRequest, *ExchangeRequest:
				done(ENOSYS)
				r.RespondError(ENOSYS)
		*/
	}
}

func (c *serveConn) saveLookup(s *fuse.LookupResponse, snode *serveNode, elem string, n2 Node) {
	s.Attr = nodeAttr(n2)
	if s.Attr.Inode == 0 {
		s.Attr.Inode = c.dynamicInode(snode.inode, elem)
	}

	s.Node, s.Generation = c.saveNode(s.Attr.Inode, n2)
	if s.EntryValid == 0 {
		s.EntryValid = entryValidTime
	}
	if s.AttrValid == 0 {
		s.AttrValid = attrValidTime
	}
}

// DataHandle returns a read-only Handle that satisfies reads
// using the given data.
func DataHandle(data []byte) Handle {
	return &dataHandle{data}
}

type dataHandle struct {
	data []byte
}

func (d *dataHandle) ReadAll(ctx context.Context) ([]byte, error) {
	return d.data, nil
}

// GenerateDynamicInode returns a dynamic inode.
//
// The parent inode and current entry name are used as the criteria
// for choosing a pseudorandom inode. This makes it likely the same
// entry will get the same inode on multiple runs.
func GenerateDynamicInode(parent uint64, name string) uint64 {
	h := fnv.New64a()
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], parent)
	_, _ = h.Write(buf[:])
	_, _ = h.Write([]byte(name))
	var inode uint64
	for {
		inode = h.Sum64()
		if inode != 0 {
			break
		}
		// there's a tiny probability that result is zero; change the
		// input a little and try again
		_, _ = h.Write([]byte{'x'})
	}
	return inode
}
