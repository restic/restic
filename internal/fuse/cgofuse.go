// +build windows

package fuse

import (
	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"fmt"
	"github.com/billziss-gh/cgofuse/examples/shared"
	cgofuse "github.com/billziss-gh/cgofuse/fuse"
	"github.com/restic/restic/internal/debug"
	"golang.org/x/net/context"
	"os"
	"strings"
	"sync"
	"time"
)

func (self *Cgofs) synchronize() func() {
	self.lock.Lock()
	return func() {
		self.lock.Unlock()
	}
}

type DirNode interface {
	fs.Node
	fs.HandleReadDirAller
	fs.NodeStringLookuper
}

type Cgofs struct {
	cgofuse.FileSystemBase
	lock    sync.Mutex
	root    *Root
	ctx     context.Context
	openmap map[uint64]fs.Node
}

func getInode(n fs.Node, ctx context.Context) uint64 {
	attr := fuse.Attr{}

	n.Attr(ctx, &attr)

	return attr.Inode

}

func NewCgofs(ctx context.Context, root *Root) *Cgofs {
	ret := &Cgofs{
		ctx:     ctx,
		root:    root,
		openmap: make(map[uint64]fs.Node),
	}

	ret.openmap[root.inode] = root

	for _, v := range root.entries {
		ret.openmap[getInode(v, ctx)] = v
	}

	return ret
}

func split(path string) []string {
	return strings.Split(path, "/")
}

func (self *Cgofs) lookupNode(ctx context.Context, path string, ancestor fs.Node) (prnt fs.Node, name string, node fs.Node, err error) {
	fmt.Printf("lookup for %s\n", path)
	prnt = self.root
	name = ""
	node = self.root
	err = nil
	for _, c := range split(path) {
		if "" != c {
			if 255 < len(c) {
				panic(cgofuse.Error(-cgofuse.ENAMETOOLONG))
			}
			prnt, name = node, c

			dirNode, ok := node.(DirNode)
			if ok {
				fmt.Printf("%s is dir\n", name)
				//this node is a dir
				node, err = dirNode.Lookup(ctx, c)
				if nil != ancestor && node == ancestor {
					name = "" // special case loop condition
					return
				}
			} else {
				fmt.Printf("%s is file\n", name)
				//this node is a file
				return
			}
		}
	}
	return
}
func (self *Cgofs) openNode(path string, dir bool) (int, uint64) {
	_, _, node, err := self.lookupNode(self.ctx, path, nil)
	if nil != err {
		debug.Log("have errror %v", err)
		return -cgofuse.ENOENT, ^uint64(0)
	}
	if nil == node {
		debug.Log("no node found when openNode")
		return -cgofuse.ENOENT, ^uint64(0)
	}

	fmt.Printf("found node %s\n", path)

	return 0, getInode(node, self.ctx)
}

func (self *Cgofs) Open(path string, flags int) (errc int, fh uint64) {
	defer trace(path, flags)(&errc, &fh)
	defer self.synchronize()()
	return self.openNode(path, false)
}

func (self *Cgofs) Opendir(path string) (errc int, fh uint64) {
	defer trace(path)(&errc, &fh)
	debug.Log("open dir %v", path)
	return self.openNode(path, true)
}

func trace(vals ...interface{}) func(vals ...interface{}) {
	uid, gid, _ := cgofuse.Getcontext()
	return shared.Trace(1, fmt.Sprintf("[uid=%v,gid=%v]", uid, gid), vals...)
}

func (self *Cgofs) Releasedir(path string, fh uint64) (errc int) {
	defer trace(path, fh)(&errc)
	defer self.synchronize()()
	return 0
}

var dt_mode_map = map[fuse.DirentType]uint32{fuse.DT_Unknown: 0, fuse.DT_File: cgofuse.S_IFREG, fuse.DT_Link: cgofuse.S_IFLNK, fuse.DT_Dir: cgofuse.S_IFDIR, fuse.DT_Char: cgofuse.S_IFCHR, fuse.DT_Socket: cgofuse.S_IFSOCK, fuse.DT_Block: cgofuse.S_IFBLK, fuse.DT_FIFO: cgofuse.S_IFIFO}

func direntToStat(ent *fuse.Dirent, mode uint32) *cgofuse.Stat_t {

	return &cgofuse.Stat_t{
		Ino:  ent.Inode,
		Mode: dt_mode_map[ent.Type] | mode,
	}

}

func (self *Cgofs) Readdir(
	path string,
	fill func(name string, stat *cgofuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) (errc int) {
	defer trace(path, fill, ofst, fh)(&errc)
	defer self.synchronize()()
	dirNode := self.openmap[fh].(DirNode)

	debug.Log("readdir %s, %d", path, fh)

	ents, err := dirNode.ReadDirAll(self.ctx)

	if err != nil {
		return -cgofuse.ENOENT
	}

	_, is_snapshots := dirNode.(*SnapshotsDir)

	for _, ent := range ents {

		orMode := 00777

		//latest dir under snapshots not supported yet
		if is_snapshots && ent.Name == "latest" {
			continue
		}

		fill(ent.Name, direntToStat(&ent, uint32(orMode)), 0)
	}

	return 0

}

func (self *Cgofs) getNode(path string, fh uint64) (fs.Node, error) {
	if ^uint64(0) == fh {
		_, _, node, err := self.lookupNode(self.ctx, path, nil)
		return node, err
	} else {
		return self.openmap[fh], nil
	}
}

func timeToSpec(t time.Time) cgofuse.Timespec {
	return cgofuse.Timespec{
		Sec:  int64(t.Second()),
		Nsec: int64(t.Nanosecond()),
	}
}

var modeMap = map[os.FileMode]uint32{
	os.ModeDir:        cgofuse.S_IFDIR,
	os.ModeSymlink:    cgofuse.S_IFLNK,
	os.ModeNamedPipe:  cgofuse.S_IFIFO,
	os.ModeCharDevice: cgofuse.S_IFCHR,
}

func attrToStat(a *fuse.Attr, t *cgofuse.Stat_t) {

	permission := a.Mode & 0777

	mappedMode, has := modeMap[a.Mode-permission]
	fmt.Printf("permssion is %d, from mode is %d, convert ot %d\n", permission, a.Mode-permission, mappedMode)
	if !has {
		mappedMode = cgofuse.S_IFREG
	}

	t.Ino = a.Inode
	t.Gid = a.Gid
	t.Uid = a.Uid
	t.Mode = mappedMode | uint32(a.Mode|permission)
	t.Atim = timeToSpec(a.Atime)
	t.Ctim = timeToSpec(a.Ctime)
	t.Mtim = timeToSpec(a.Mtime)
	t.Nlink = a.Nlink
	t.Size = int64(a.Size)
	t.Blksize = int64(a.BlockSize)
	t.Blocks = int64(a.Blocks)

}

func (self *Cgofs) Getattr(path string, stat *cgofuse.Stat_t, fh uint64) (errc int) {
	defer trace(path, fh)(&errc, stat)
	defer self.synchronize()()
	node, err := self.getNode(path, fh)
	if nil != err {
		debug.Log("have error when found for %s , %d", path, fh)
		return -cgofuse.ENOENT
	}
	if nil == node {
		debug.Log("no node found for %s , %d", path, fh)
		return -cgofuse.ENOENT
	}

	debug.Log("node found for %s , %d", path, fh)

	a := fuse.Attr{}

	self.openmap[getInode(node, self.ctx)] = node

	node.Attr(self.ctx, &a)

	attrToStat(&a, stat)

	return 0
}

func (self *Cgofs) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {

	defer trace(path, buff, ofst, fh)(&n)
	defer self.synchronize()()
	node, err := self.getNode(path, fh)
	if nil != err {
		return -cgofuse.ENOENT
	}
	if nil == node {
		return -cgofuse.ENOENT
	}

	f := node.(*file)

	resp := &fuse.ReadResponse{Data: buff}
	f.read(self.ctx, len(buff), ofst, resp)

	return len(resp.Data)
}

func (self *Cgofs) Release(path string, fh uint64) (errc int) {
	defer trace(path, fh)(&errc)
	defer self.synchronize()()
	return self.closeNode(fh)
}

func (self *Cgofs) closeNode(fh uint64) int {
	node := self.openmap[fh]
	f, is_file := node.(*file)
	if !is_file {
		f.release()
	}
	return 0
}

func (self *Cgofs) Readlink(path string) (errc int, target string) {
	defer trace(path)(&errc, &target)
	defer self.synchronize()()
	_, _, node, err := self.lookupNode(self.ctx, path, nil)
	if nil == node {
		return -cgofuse.ENOENT, ""
	}

	if nil != err {
		return -cgofuse.ENOENT, ""
	}

	//TODO construct ReadlinkRequest ?
	target, err = node.(fs.NodeReadlinker).Readlink(self.ctx, nil)

	if err != nil {
		return -cgofuse.ENOENT, ""
	}

	return 0, target
}

var _ = DirNode(&TagsDir{})
var _ = DirNode(&dir{})
var _ = DirNode(&SnapshotsIDSDir{})
var _ = DirNode(&SnapshotsDir{})
var _ = DirNode(&HostsDir{})
var _ = fs.Node(&snapshotLink{})
var _ = fs.Node(&file{})
var _ = fs.Node(&other{})
