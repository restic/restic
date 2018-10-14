package restic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/restic/restic/internal/errors"

	"bytes"
	"runtime"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fs"
)

// ExtendedAttribute is a tuple storing the xattr name and value.
type ExtendedAttribute struct {
	Name  string `json:"name"`
	Value []byte `json:"value"`
}

// Node is a file, directory or other item in a backup.
type Node struct {
	Name               string              `json:"name"`
	Type               string              `json:"type"`
	Mode               os.FileMode         `json:"mode,omitempty"`
	ModTime            time.Time           `json:"mtime,omitempty"`
	AccessTime         time.Time           `json:"atime,omitempty"`
	ChangeTime         time.Time           `json:"ctime,omitempty"`
	UID                uint32              `json:"uid"`
	GID                uint32              `json:"gid"`
	User               string              `json:"user,omitempty"`
	Group              string              `json:"group,omitempty"`
	Inode              uint64              `json:"inode,omitempty"`
	DeviceID           uint64              `json:"device_id,omitempty"` // device id of the file, stat.st_dev
	Size               uint64              `json:"size,omitempty"`
	Links              uint64              `json:"links,omitempty"`
	LinkTarget         string              `json:"linktarget,omitempty"`
	ExtendedAttributes []ExtendedAttribute `json:"extended_attributes,omitempty"`
	Device             uint64              `json:"device,omitempty"` // in case of Type == "dev", stat.st_rdev
	Content            IDs                 `json:"content"`
	Subtree            *ID                 `json:"subtree,omitempty"`

	Error string `json:"error,omitempty"`

	Path string `json:"-"`
}

// Nodes is a slice of nodes that can be sorted.
type Nodes []*Node

func (n Nodes) Len() int           { return len(n) }
func (n Nodes) Less(i, j int) bool { return n[i].Name < n[j].Name }
func (n Nodes) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }

func (node Node) String() string {
	var mode os.FileMode
	switch node.Type {
	case "file":
		mode = 0
	case "dir":
		mode = os.ModeDir
	case "symlink":
		mode = os.ModeSymlink
	case "dev":
		mode = os.ModeDevice
	case "chardev":
		mode = os.ModeDevice | os.ModeCharDevice
	case "fifo":
		mode = os.ModeNamedPipe
	case "socket":
		mode = os.ModeSocket
	}

	return fmt.Sprintf("%s %5d %5d %6d %s %s",
		mode|node.Mode, node.UID, node.GID, node.Size, node.ModTime, node.Name)
}

// NodeFromFileInfo returns a new node from the given path and FileInfo. It
// returns the first error that is encountered, together with a node.
func NodeFromFileInfo(path string, fi os.FileInfo) (*Node, error) {
	mask := os.ModePerm | os.ModeType | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	node := &Node{
		Path:    path,
		Name:    fi.Name(),
		Mode:    fi.Mode() & mask,
		ModTime: fi.ModTime(),
	}

	node.Type = nodeTypeFromFileInfo(fi)
	if node.Type == "file" {
		node.Size = uint64(fi.Size())
	}

	err := node.fillExtra(path, fi)
	return node, err
}

func nodeTypeFromFileInfo(fi os.FileInfo) string {
	switch fi.Mode() & (os.ModeType | os.ModeCharDevice) {
	case 0:
		return "file"
	case os.ModeDir:
		return "dir"
	case os.ModeSymlink:
		return "symlink"
	case os.ModeDevice | os.ModeCharDevice:
		return "chardev"
	case os.ModeDevice:
		return "dev"
	case os.ModeNamedPipe:
		return "fifo"
	case os.ModeSocket:
		return "socket"
	}

	return ""
}

// GetExtendedAttribute gets the extended attribute.
func (node Node) GetExtendedAttribute(a string) []byte {
	for _, attr := range node.ExtendedAttributes {
		if attr.Name == a {
			return attr.Value
		}
	}
	return nil
}

// CreateAt creates the node at the given path but does NOT restore node meta data.
func (node *Node) CreateAt(ctx context.Context, path string, repo Repository) error {
	debug.Log("create node %v at %v", node.Name, path)

	switch node.Type {
	case "dir":
		if err := node.createDirAt(path); err != nil {
			return err
		}
	case "file":
		if err := node.createFileAt(ctx, path, repo); err != nil {
			return err
		}
	case "symlink":
		if err := node.createSymlinkAt(path); err != nil {
			return err
		}
	case "dev":
		if err := node.createDevAt(path); err != nil {
			return err
		}
	case "chardev":
		if err := node.createCharDevAt(path); err != nil {
			return err
		}
	case "fifo":
		if err := node.createFifoAt(path); err != nil {
			return err
		}
	case "socket":
		return nil
	default:
		return errors.Errorf("filetype %q not implemented!\n", node.Type)
	}

	return nil
}

// RestoreMetadata restores node metadata
func (node Node) RestoreMetadata(path string) error {
	err := node.restoreMetadata(path)
	if err != nil {
		debug.Log("restoreMetadata(%s) error %v", path, err)
	}

	return err
}

func (node Node) restoreMetadata(path string) error {
	var firsterr error

	if err := lchown(path, int(node.UID), int(node.GID)); err != nil {
		// Like "cp -a" and "rsync -a" do, we only report lchown permission errors
		// if we run as root.
		// On Windows, Geteuid always returns -1, and we always report lchown
		// permission errors.
		if os.Geteuid() > 0 && os.IsPermission(err) {
			debug.Log("not running as root, ignoring lchown permission error for %v: %v",
				path, err)
		} else {
			firsterr = errors.Wrap(err, "Lchown")
		}
	}

	if node.Type != "symlink" {
		if err := fs.Chmod(path, node.Mode); err != nil {
			if firsterr != nil {
				firsterr = errors.Wrap(err, "Chmod")
			}
		}
	}

	if err := node.RestoreTimestamps(path); err != nil {
		debug.Log("error restoring timestamps for dir %v: %v", path, err)
		if firsterr != nil {
			firsterr = err
		}
	}

	if err := node.restoreExtendedAttributes(path); err != nil {
		debug.Log("error restoring extended attributes for %v: %v", path, err)
		if firsterr != nil {
			firsterr = err
		}
	}

	return firsterr
}

func (node Node) restoreExtendedAttributes(path string) error {
	for _, attr := range node.ExtendedAttributes {
		err := Setxattr(path, attr.Name, attr.Value)
		if err != nil {
			return err
		}
	}
	return nil
}

func (node Node) RestoreTimestamps(path string) error {
	var utimes = [...]syscall.Timespec{
		syscall.NsecToTimespec(node.AccessTime.UnixNano()),
		syscall.NsecToTimespec(node.ModTime.UnixNano()),
	}

	if node.Type == "symlink" {
		return node.restoreSymlinkTimestamps(path, utimes)
	}

	if err := syscall.UtimesNano(path, utimes[:]); err != nil {
		return errors.Wrap(err, "UtimesNano")
	}

	return nil
}

func (node Node) createDirAt(path string) error {
	err := fs.Mkdir(path, node.Mode)
	if err != nil && !os.IsExist(err) {
		return errors.Wrap(err, "Mkdir")
	}

	return nil
}

func (node Node) createFileAt(ctx context.Context, path string, repo Repository) error {
	f, err := fs.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return errors.Wrap(err, "OpenFile")
	}

	err = node.writeNodeContent(ctx, repo, f)
	closeErr := f.Close()

	if err != nil {
		return err
	}

	if closeErr != nil {
		return errors.Wrap(closeErr, "Close")
	}

	return nil
}

func (node Node) writeNodeContent(ctx context.Context, repo Repository, f *os.File) error {
	var buf []byte
	for _, id := range node.Content {
		size, found := repo.LookupBlobSize(id, DataBlob)
		if !found {
			return errors.Errorf("id %v not found in repository", id)
		}

		buf = buf[:cap(buf)]
		if len(buf) < CiphertextLength(int(size)) {
			buf = NewBlobBuffer(int(size))
		}

		n, err := repo.LoadBlob(ctx, DataBlob, id, buf)
		if err != nil {
			return err
		}
		buf = buf[:n]

		_, err = f.Write(buf)
		if err != nil {
			return errors.Wrap(err, "Write")
		}
	}

	return nil
}

func (node Node) createSymlinkAt(path string) error {
	// Windows does not allow non-admins to create soft links.
	if runtime.GOOS == "windows" {
		return nil
	}
	err := fs.Symlink(node.LinkTarget, path)
	if err != nil {
		return errors.Wrap(err, "Symlink")
	}

	return nil
}

func (node *Node) createDevAt(path string) error {
	return mknod(path, syscall.S_IFBLK|0600, int(node.Device))
}

func (node *Node) createCharDevAt(path string) error {
	return mknod(path, syscall.S_IFCHR|0600, int(node.Device))
}

func (node *Node) createFifoAt(path string) error {
	return mkfifo(path, 0600)
}

func (node Node) MarshalJSON() ([]byte, error) {
	if node.ModTime.Year() < 0 || node.ModTime.Year() > 9999 {
		err := errors.Errorf("node %v has invalid ModTime year %d: %v",
			node.Path, node.ModTime.Year(), node.ModTime)
		return nil, err
	}

	if node.ChangeTime.Year() < 0 || node.ChangeTime.Year() > 9999 {
		err := errors.Errorf("node %v has invalid ChangeTime year %d: %v",
			node.Path, node.ChangeTime.Year(), node.ChangeTime)
		return nil, err
	}

	if node.AccessTime.Year() < 0 || node.AccessTime.Year() > 9999 {
		err := errors.Errorf("node %v has invalid AccessTime year %d: %v",
			node.Path, node.AccessTime.Year(), node.AccessTime)
		return nil, err
	}

	type nodeJSON Node
	nj := nodeJSON(node)
	name := strconv.Quote(node.Name)
	nj.Name = name[1 : len(name)-1]

	return json.Marshal(nj)
}

func (node *Node) UnmarshalJSON(data []byte) error {
	type nodeJSON Node
	nj := (*nodeJSON)(node)

	err := json.Unmarshal(data, nj)
	if err != nil {
		return errors.Wrap(err, "Unmarshal")
	}

	nj.Name, err = strconv.Unquote(`"` + nj.Name + `"`)
	return errors.Wrap(err, "Unquote")
}

func (node Node) Equals(other Node) bool {
	if node.Name != other.Name {
		return false
	}
	if node.Type != other.Type {
		return false
	}
	if node.Mode != other.Mode {
		return false
	}
	if !node.ModTime.Equal(other.ModTime) {
		return false
	}
	if !node.AccessTime.Equal(other.AccessTime) {
		return false
	}
	if !node.ChangeTime.Equal(other.ChangeTime) {
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
	if node.DeviceID != other.DeviceID {
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
	if !node.sameContent(other) {
		return false
	}
	if !node.sameExtendedAttributes(other) {
		return false
	}
	if node.Subtree != nil {
		if other.Subtree == nil {
			return false
		}

		if !node.Subtree.Equal(*other.Subtree) {
			return false
		}
	} else {
		if other.Subtree != nil {
			return false
		}
	}
	if node.Error != other.Error {
		return false
	}

	return true
}

func (node Node) sameContent(other Node) bool {
	if node.Content == nil {
		return other.Content == nil
	}

	if other.Content == nil {
		return false
	}

	if len(node.Content) != len(other.Content) {
		return false
	}

	for i := 0; i < len(node.Content); i++ {
		if !node.Content[i].Equal(other.Content[i]) {
			return false
		}
	}
	return true
}

func (node Node) sameExtendedAttributes(other Node) bool {
	if len(node.ExtendedAttributes) != len(other.ExtendedAttributes) {
		return false
	}

	// build a set of all attributes that node has
	type mapvalue struct {
		value   []byte
		present bool
	}
	attributes := make(map[string]mapvalue)
	for _, attr := range node.ExtendedAttributes {
		attributes[attr.Name] = mapvalue{value: attr.Value}
	}

	for _, attr := range other.ExtendedAttributes {
		v, ok := attributes[attr.Name]
		if !ok {
			// extended attribute is not set for node
			debug.Log("other node has attribute %v, which is not present in node", attr.Name)
			return false

		}

		if !bytes.Equal(v.value, attr.Value) {
			// attribute has different value
			debug.Log("attribute %v has different value", attr.Name)
			return false
		}

		// remember that this attribute is present in other.
		v.present = true
		attributes[attr.Name] = v
	}

	// check for attributes that are not present in other
	for name, v := range attributes {
		if !v.present {
			debug.Log("attribute %v not present in other node", name)
			return false
		}
	}

	return true
}

func (node *Node) fillUser(stat statT) error {
	node.UID = stat.uid()
	node.GID = stat.gid()

	username, err := lookupUsername(strconv.Itoa(int(stat.uid())))
	if err != nil {
		return err
	}

	group, err := lookupGroup(strconv.Itoa(int(stat.gid())))
	if err != nil {
		return err
	}

	node.User = username
	node.Group = group

	return nil
}

var (
	uidLookupCache      = make(map[string]string)
	uidLookupCacheMutex = sync.RWMutex{}
)

func lookupUsername(uid string) (string, error) {
	uidLookupCacheMutex.RLock()
	value, ok := uidLookupCache[uid]
	uidLookupCacheMutex.RUnlock()

	if ok {
		return value, nil
	}

	username := ""

	u, err := user.LookupId(uid)
	if err == nil {
		username = u.Username
	}

	uidLookupCacheMutex.Lock()
	uidLookupCache[uid] = username
	uidLookupCacheMutex.Unlock()

	return username, nil
}

var (
	gidLookupCache      = make(map[string]string)
	gidLookupCacheMutex = sync.RWMutex{}
)

func lookupGroup(gid string) (string, error) {
	gidLookupCacheMutex.RLock()
	value, ok := gidLookupCache[gid]
	gidLookupCacheMutex.RUnlock()

	if ok {
		return value, nil
	}

	group := ""

	g, err := user.LookupGroupId(gid)
	if err == nil {
		group = g.Name
	}

	gidLookupCacheMutex.Lock()
	gidLookupCache[gid] = group
	gidLookupCacheMutex.Unlock()

	return group, nil
}

func (node *Node) fillExtra(path string, fi os.FileInfo) error {
	stat, ok := toStatT(fi.Sys())
	if !ok {
		// fill minimal info with current values for uid, gid
		node.UID = uint32(os.Getuid())
		node.GID = uint32(os.Getgid())
		node.ChangeTime = node.ModTime
		return nil
	}

	node.Inode = uint64(stat.ino())
	node.DeviceID = uint64(stat.dev())

	node.fillTimes(stat)

	var err error

	if err = node.fillUser(stat); err != nil {
		return err
	}

	switch node.Type {
	case "file":
		node.Size = uint64(stat.size())
		node.Links = uint64(stat.nlink())
	case "dir":
	case "symlink":
		node.LinkTarget, err = fs.Readlink(path)
		node.Links = uint64(stat.nlink())
		if err != nil {
			return errors.Wrap(err, "Readlink")
		}
	case "dev":
		node.Device = uint64(stat.rdev())
		node.Links = uint64(stat.nlink())
	case "chardev":
		node.Device = uint64(stat.rdev())
		node.Links = uint64(stat.nlink())
	case "fifo":
	case "socket":
	default:
		return errors.Errorf("invalid node type %q", node.Type)
	}

	if err = node.fillExtendedAttributes(path); err != nil {
		return err
	}

	return nil
}

func (node *Node) fillExtendedAttributes(path string) error {
	if node.Type == "symlink" {
		return nil
	}

	xattrs, err := Listxattr(path)
	debug.Log("fillExtendedAttributes(%v) %v %v", path, xattrs, err)
	if err != nil {
		return err
	}

	node.ExtendedAttributes = make([]ExtendedAttribute, 0, len(xattrs))
	for _, attr := range xattrs {
		attrVal, err := Getxattr(path, attr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "can not obtain extended attribute %v for %v:\n", attr, path)
			continue
		}
		attr := ExtendedAttribute{
			Name:  attr,
			Value: attrVal,
		}

		node.ExtendedAttributes = append(node.ExtendedAttributes, attr)
	}

	return nil
}

type statT interface {
	dev() uint64
	ino() uint64
	nlink() uint64
	uid() uint32
	gid() uint32
	rdev() uint64
	size() int64
	atim() syscall.Timespec
	mtim() syscall.Timespec
	ctim() syscall.Timespec
}

func mkfifo(path string, mode uint32) (err error) {
	return mknod(path, mode|syscall.S_IFIFO, 0)
}

func (node *Node) fillTimes(stat statT) {
	ctim := stat.ctim()
	atim := stat.atim()
	node.ChangeTime = time.Unix(ctim.Unix())
	node.AccessTime = time.Unix(atim.Unix())
}

func changeTime(stat statT) time.Time {
	ctim := stat.ctim()
	return time.Unix(ctim.Unix())
}
