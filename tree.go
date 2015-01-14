package restic

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"syscall"
	"time"

	"github.com/juju/arrar"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
)

type Tree struct {
	Nodes []*Node `json:"nodes"`
	Map   *Map    `json:"map"`
}

type Node struct {
	Name       string       `json:"name"`
	Type       string       `json:"type"`
	Mode       os.FileMode  `json:"mode,omitempty"`
	ModTime    time.Time    `json:"mtime,omitempty"`
	AccessTime time.Time    `json:"atime,omitempty"`
	ChangeTime time.Time    `json:"ctime,omitempty"`
	UID        uint32       `json:"uid"`
	GID        uint32       `json:"gid"`
	User       string       `json:"user,omitempty"`
	Group      string       `json:"group,omitempty"`
	Inode      uint64       `json:"inode,omitempty"`
	Size       uint64       `json:"size,omitempty"`
	Links      uint64       `json:"links,omitempty"`
	LinkTarget string       `json:"linktarget,omitempty"`
	Device     uint64       `json:"device,omitempty"`
	Content    []backend.ID `json:"content"`
	Subtree    backend.ID   `json:"subtree,omitempty"`

	Error string `json:"error,omitempty"`

	tree *Tree

	path string
	err  error
}

var (
	ErrNodeNotFound      = errors.New("named node not found")
	ErrNodeAlreadyInTree = errors.New("node already present")
)

type Blob struct {
	ID          backend.ID `json:"id,omitempty"`
	Offset      uint64     `json:"offset,omitempty"`
	Size        uint64     `json:"size,omitempty"`
	Storage     backend.ID `json:"sid,omitempty"`   // encrypted ID
	StorageSize uint64     `json:"ssize,omitempty"` // encrypted Size
}

type Blobs []Blob

func (n Node) String() string {
	switch n.Type {
	case "file":
		return fmt.Sprintf("%s %5d %5d %6d %s %s",
			n.Mode, n.UID, n.GID, n.Size, n.ModTime, n.Name)
	case "dir":
		return fmt.Sprintf("%s %5d %5d %6d %s %s",
			n.Mode|os.ModeDir, n.UID, n.GID, n.Size, n.ModTime, n.Name)
	}

	return fmt.Sprintf("<Node(%s) %s>", n.Type, n.Name)
}

func NewTree() *Tree {
	return &Tree{
		Nodes: []*Node{},
		Map:   NewMap(),
	}
}

func (t Tree) String() string {
	return fmt.Sprintf("Tree<%d nodes, %d blobs>", len(t.Nodes), len(t.Map.list))
}

func LoadTree(s Server, blob Blob) (*Tree, error) {
	tree := &Tree{}
	err := s.LoadJSON(backend.Tree, blob, tree)
	if err != nil {
		return nil, err
	}

	return tree, nil
}

// LoadTreeRecursive loads the tree and all subtrees via s.
func LoadTreeRecursive(path string, s Server, blob Blob) (*Tree, error) {
	// TODO: load subtrees in parallel
	tree, err := LoadTree(s, blob)
	if err != nil {
		return nil, err
	}

	for _, n := range tree.Nodes {
		n.path = filepath.Join(path, n.Name)
		if n.Type == "dir" && n.Subtree != nil {
			subtreeBlob, err := tree.Map.FindID(n.Subtree)
			if err != nil {
				return nil, err
			}

			t, err := LoadTreeRecursive(n.path, s, subtreeBlob)
			if err != nil {
				return nil, err
			}

			n.tree = t
		}
	}

	return tree, nil
}

// CopyFrom recursively copies all content from other to t.
func (t Tree) CopyFrom(other *Tree, s *Server) error {
	debug.Log("Tree.CopyFrom", "CopyFrom(%v)\n", other)
	for _, node := range t.Nodes {
		// only process files and dirs
		if node.Type != "file" && node.Type != "dir" {
			continue
		}

		// find entry in other tree
		oldNode, err := other.Find(node.Name)

		// if the node could not be found or the type has changed, proceed to the next
		if err == ErrNodeNotFound || node.Type != oldNode.Type {
			debug.Log("Tree.CopyFrom", "  node %v is new\n", node)
			continue
		}

		if node.Type == "file" {
			// compare content
			if node.SameContent(oldNode) {
				debug.Log("Tree.CopyFrom", "  file node %v has same content\n", node)

				// check if all content is still available in the repository
				for _, id := range oldNode.Content {
					blob, err := other.Map.FindID(id)
					if err != nil {
						continue
					}

					if ok, err := s.Test(backend.Data, blob.Storage); !ok || err != nil {
						continue
					}
				}

				// copy Content
				node.Content = oldNode.Content

				// copy storage IDs
				for _, id := range node.Content {
					blob, err := other.Map.FindID(id)
					if err != nil {
						return err
					}

					debug.Log("Tree.CopyFrom", "    insert blob %v\n", blob)
					t.Map.Insert(blob)
				}
			}
		} else if node.Type == "dir" {
			// fill in all subtrees from old subtree
			err := node.tree.CopyFrom(oldNode.tree, s)
			if err != nil {
				return err
			}

			// check if tree has changed
			if node.tree.Equals(*oldNode.tree) {
				debug.Log("Tree.CopyFrom", "  tree node %v has same content\n", node)

				// if nothing has changed, copy subtree ID
				node.Subtree = oldNode.Subtree

				// and store blob in bloblist
				blob, err := other.Map.FindID(oldNode.Subtree)
				if err != nil {
					return err
				}

				debug.Log("Tree.CopyFrom", "    insert blob %v\n", blob)
				t.Map.Insert(blob)
			} else {
				debug.Log("Tree.CopyFrom", "  trees are not equal: %v\n", node)
				debug.Log("Tree.CopyFrom", "    %#v\n", node.tree)
				debug.Log("Tree.CopyFrom", "    %#v\n", oldNode.tree)
			}
		}
	}

	return nil
}

// Equals returns true if t and other have exactly the same nodes and map.
func (t Tree) Equals(other Tree) bool {
	if len(t.Nodes) != len(other.Nodes) {
		debug.Log("Tree.Equals", "tree.Equals(): trees have different number of nodes")
		return false
	}

	if !t.Map.Equals(other.Map) {
		debug.Log("Tree.Equals", "tree.Equals(): maps aren't equal")
		return false
	}

	for i := 0; i < len(t.Nodes); i++ {
		if !t.Nodes[i].Equals(*other.Nodes[i]) {
			debug.Log("Tree.Equals", "tree.Equals(): node %d is different:", i)
			debug.Log("Tree.Equals", "  %#v", t.Nodes[i])
			debug.Log("Tree.Equals", "  %#v", other.Nodes[i])
			return false
		}
	}

	return true
}

func (t *Tree) Insert(node *Node) error {
	pos, _, err := t.find(node.Name)
	if err == nil {
		// already present
		return ErrNodeAlreadyInTree
	}

	// insert blob
	// https://code.google.com/p/go-wiki/wiki/bliceTricks
	t.Nodes = append(t.Nodes, &Node{})
	copy(t.Nodes[pos+1:], t.Nodes[pos:])
	t.Nodes[pos] = node

	return nil
}

func (t Tree) find(name string) (int, *Node, error) {
	pos := sort.Search(len(t.Nodes), func(i int) bool {
		return t.Nodes[i].Name >= name
	})

	if pos < len(t.Nodes) && t.Nodes[pos].Name == name {
		return pos, t.Nodes[pos], nil
	}

	return pos, nil, ErrNodeNotFound
}

func (t Tree) Find(name string) (*Node, error) {
	_, node, err := t.find(name)
	return node, err
}

func (t Tree) Stat() Stat {
	s := Stat{}
	for _, n := range t.Nodes {
		switch n.Type {
		case "file":
			s.Files++
			s.Bytes += n.Size
		case "dir":
			s.Dirs++
			if n.tree != nil {
				s.Add(n.tree.Stat())
			}
		}
	}

	return s
}

func (t Tree) StatTodo() Stat {
	s := Stat{}
	for _, n := range t.Nodes {
		switch n.Type {
		case "file":
			if n.Content == nil {
				s.Files++
				s.Bytes += n.Size
			}
		case "dir":
			if n.Subtree == nil {
				s.Dirs++
				if n.tree != nil {
					s.Add(n.tree.StatTodo())
				}
			}
		}
	}

	return s
}

func (node Node) Tree() *Tree {
	return node.tree
}

func (node *Node) fill_extra(path string, fi os.FileInfo) (err error) {
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}

	node.ChangeTime = time.Unix(stat.Ctim.Unix())
	node.AccessTime = time.Unix(stat.Atim.Unix())
	node.UID = stat.Uid
	node.GID = stat.Gid

	// TODO: cache uid lookup
	if u, nil := user.LookupId(strconv.Itoa(int(stat.Uid))); err == nil {
		node.User = u.Username
	}

	// TODO: implement getgrnam() or use https://github.com/kless/osutil
	// if g, nil := user.LookupId(strconv.Itoa(int(stat.Uid))); err == nil {
	// 	node.User = u.Username
	// }

	node.Inode = stat.Ino

	switch node.Type {
	case "file":
		node.Size = uint64(stat.Size)
		node.Links = stat.Nlink
	case "dir":
		// nothing to do
	case "symlink":
		node.LinkTarget, err = os.Readlink(path)
	case "dev":
		node.Device = stat.Rdev
	case "chardev":
		node.Device = stat.Rdev
	case "fifo":
		// nothing to do
	case "socket":
		// nothing to do
	default:
		panic(fmt.Sprintf("invalid node type %q", node.Type))
	}

	return err
}

func NodeFromFileInfo(path string, fi os.FileInfo) (*Node, error) {
	node := GetNode()
	node.path = path
	node.Name = fi.Name()
	node.Mode = fi.Mode() & os.ModePerm
	node.ModTime = fi.ModTime()

	switch fi.Mode() & (os.ModeType | os.ModeCharDevice) {
	case 0:
		node.Type = "file"
	case os.ModeDir:
		node.Type = "dir"
	case os.ModeSymlink:
		node.Type = "symlink"
	case os.ModeDevice | os.ModeCharDevice:
		node.Type = "chardev"
	case os.ModeDevice:
		node.Type = "dev"
	case os.ModeNamedPipe:
		node.Type = "fifo"
	case os.ModeSocket:
		node.Type = "socket"
	}

	err := node.fill_extra(path, fi)
	return node, err
}

func (t Tree) CreateNodeAt(node *Node, s Server, path string) error {
	switch node.Type {
	case "dir":
		err := os.Mkdir(path, node.Mode)
		if err != nil {
			return arrar.Annotate(err, "Mkdir")
		}

		err = os.Lchown(path, int(node.UID), int(node.GID))
		if err != nil {
			return arrar.Annotate(err, "Lchown")
		}

		var utimes = []syscall.Timespec{
			syscall.NsecToTimespec(node.AccessTime.UnixNano()),
			syscall.NsecToTimespec(node.ModTime.UnixNano()),
		}
		err = syscall.UtimesNano(path, utimes)
		if err != nil {
			return arrar.Annotate(err, "Utimesnano")
		}
	case "file":
		// TODO: handle hard links
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
		defer f.Close()
		if err != nil {
			return arrar.Annotate(err, "OpenFile")
		}

		for _, blobid := range node.Content {
			blob, err := t.Map.FindID(blobid)
			if err != nil {
				return arrar.Annotate(err, "Find Blob")
			}

			buf, err := s.Load(backend.Data, blob)
			if err != nil {
				return arrar.Annotate(err, "Load")
			}

			_, err = f.Write(buf)
			if err != nil {
				return arrar.Annotate(err, "Write")
			}
		}

		f.Close()

		err = os.Lchown(path, int(node.UID), int(node.GID))
		if err != nil {
			return arrar.Annotate(err, "Lchown")
		}

		var utimes = []syscall.Timespec{
			syscall.NsecToTimespec(node.AccessTime.UnixNano()),
			syscall.NsecToTimespec(node.ModTime.UnixNano()),
		}
		err = syscall.UtimesNano(path, utimes)
		if err != nil {
			return arrar.Annotate(err, "Utimesnano")
		}
	case "symlink":
		err := os.Symlink(node.LinkTarget, path)
		if err != nil {
			return arrar.Annotate(err, "Symlink")
		}

		err = os.Lchown(path, int(node.UID), int(node.GID))
		if err != nil {
			return arrar.Annotate(err, "Lchown")
		}

		f, err := os.OpenFile(path, O_PATH|syscall.O_NOFOLLOW, 0600)
		defer f.Close()
		if err != nil {
			return arrar.Annotate(err, "OpenFile")
		}

		// TODO: Get Futimes() working on older Linux kernels (fails with 3.2.0)
		// var utimes = []syscall.Timeval{
		// 	syscall.NsecToTimeval(node.AccessTime.UnixNano()),
		// 	syscall.NsecToTimeval(node.ModTime.UnixNano()),
		// }
		// err = syscall.Futimes(int(f.Fd()), utimes)
		// if err != nil {
		// 	return arrar.Annotate(err, "Futimes")
		// }

		return nil
	case "dev":
		err := syscall.Mknod(path, syscall.S_IFBLK|0600, int(node.Device))
		if err != nil {
			return arrar.Annotate(err, "Mknod")
		}
	case "chardev":
		err := syscall.Mknod(path, syscall.S_IFCHR|0600, int(node.Device))
		if err != nil {
			return arrar.Annotate(err, "Mknod")
		}
	case "fifo":
		err := syscall.Mkfifo(path, 0600)
		if err != nil {
			return arrar.Annotate(err, "Mkfifo")
		}
	case "socket":
		// nothing to do, we do not restore sockets
	default:
		return fmt.Errorf("filetype %q not implemented!\n", node.Type)
	}

	err := os.Chmod(path, node.Mode)
	if err != nil {
		return arrar.Annotate(err, "Chmod")
	}

	err = os.Chown(path, int(node.UID), int(node.GID))
	if err != nil {
		return arrar.Annotate(err, "Chown")
	}

	err = os.Chtimes(path, node.AccessTime, node.ModTime)
	if err != nil {
		return arrar.Annotate(err, "Chtimes")
	}

	return nil
}

func (node Node) SameContent(olderNode *Node) bool {
	// if this node has a type other than "file", treat as if content has changed
	if node.Type != "file" {
		return false
	}

	// if the name or type has changed, this is surely something different
	if node.Name != olderNode.Name || node.Type != olderNode.Type {
		return false
	}

	// if timestamps or inodes differ, content has changed
	if node.ModTime != olderNode.ModTime ||
		node.ChangeTime != olderNode.ChangeTime ||
		node.Inode != olderNode.Inode {
		return false
	}

	// otherwise the node is assumed to have the same content
	return true
}

func (node Node) MarshalJSON() ([]byte, error) {
	type nodeJSON Node
	nj := nodeJSON(node)
	name := strconv.Quote(node.Name)
	nj.Name = name[1 : len(name)-1]

	return json.Marshal(nj)
}

func (node *Node) UnmarshalJSON(data []byte) error {
	type nodeJSON Node
	var nj *nodeJSON = (*nodeJSON)(node)

	err := json.Unmarshal(data, nj)
	if err != nil {
		return err
	}

	nj.Name, err = strconv.Unquote(`"` + nj.Name + `"`)
	return err
}

func (node Node) Equals(other Node) bool {
	// TODO: add generatored code for this
	if node.Name != other.Name {
		return false
	}
	if node.Type != other.Type {
		return false
	}
	if node.Mode != other.Mode {
		return false
	}
	if node.ModTime != other.ModTime {
		return false
	}
	if node.AccessTime != other.AccessTime {
		return false
	}
	if node.ChangeTime != other.ChangeTime {
		return false
	}
	if node.UID != other.UID {
		return false
	}
	if node.GID != other.GID {
		return false
	}
	if node.User != other.User {
		return false
	}
	if node.Group != other.Group {
		return false
	}
	if node.Inode != other.Inode {
		return false
	}
	if node.Size != other.Size {
		return false
	}
	if node.Links != other.Links {
		return false
	}
	if node.LinkTarget != other.LinkTarget {
		return false
	}
	if node.Device != other.Device {
		return false
	}
	if node.Content != nil && other.Content == nil {
		return false
	} else if node.Content == nil && other.Content != nil {
		return false
	} else if node.Content != nil && other.Content != nil {
		if len(node.Content) != len(other.Content) {
			return false
		}

		for i := 0; i < len(node.Content); i++ {
			if !node.Content[i].Equal(other.Content[i]) {
				return false
			}
		}
	}

	if !node.Subtree.Equal(other.Subtree) {
		return false
	}

	if node.Error != other.Error {
		return false
	}

	return true
}

func (b Blob) Free() {
	if b.ID != nil {
		b.ID.Free()
	}

	if b.Storage != nil {
		b.Storage.Free()
	}
}

func (b Blob) Valid() bool {
	if b.ID == nil || b.Storage == nil || b.StorageSize == 0 {
		return false
	}

	return true
}

func (b Blob) String() string {
	return fmt.Sprintf("Blob<%s -> %s>",
		b.ID.Str(),
		b.Storage.Str())
}
