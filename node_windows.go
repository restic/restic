package restic

import (
	"fmt"
	"os"

	"github.com/restic/restic/debug"

	"syscall"
	"time"
)

func (node *Node) fill_extra(path string, fi os.FileInfo) (err error) {
	stat, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return
	}

	//we need access to os.File filedescriptor!!!!

	//syscall.ByHandleFileInformation {
	//    FileAttributes     uint32
	//    CreationTime       Filetime
	//    LastAccessTime     Filetime
	//    LastWriteTime      Filetime
	//    VolumeSerialNumber uint32         -->   node.Device
	//    FileSizeHigh       uint32         -->  can be used to calculate node.Size
	//    FileSizeLow        uint32
	//    NumberOfLinks      uint32         --> node.Links
	//    FileIndexHigh      uint32
	//    FileIndexLow       uint32         -->   Node.Inode= uint64(FileIndexLow) | uint64(FileIndexHigh)<<32
	//}
	//
	//file,err:=os.Open(path)
	//if err!=nil{
	//	return err
	//}
	//byhandlefi := &syscall.ByHandleFileInformation{}
	//
	//err=syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), byhandlefi)
	//if err!=nil{
	//	return err
	//}

	node.ChangeTime = time.Unix(0, stat.LastWriteTime.Nanoseconds())
	node.AccessTime = time.Unix(0, stat.LastAccessTime.Nanoseconds())

	//Todo GetSecurityInfo
	//node.UID
	//node.GID
	//node.User
	//node.Inode = uint64(byhandlefi.FileIndexLow) | uint64(byhandlefi.FileIndexHigh)<<32

	switch node.Type {
	case "file":
		node.Size = uint64(fi.Size())
		//node.Links = uint64(byhandlefi.NumberOfLinks)
	case "dir":
		// nothing to do
	case "symlink":
		node.LinkTarget, err = os.Readlink(path)
	case "dev":
		// nothing to do
	case "chardev":
		// nothing to do
	case "fifo":
		// nothing to do
	case "socket":
		// nothing to do
	default:
		panic(fmt.Sprintf("invalid node type %q", node.Type))
	}

	return err
}

func (node *Node) createDevAt(path string) error {
	return nil
}

func (node *Node) createCharDevAt(path string) error {
	return nil
}

func (node *Node) createFifoAt(path string) error {
	return nil
}

func (node *Node) isNewer(path string, fi os.FileInfo) bool {
	// if this node has a type other than "file", treat as if content has changed
	if node.Type != "file" {
		debug.Log("node.isNewer", "node %v is newer: not file", path)
		return true
	}

	// if the name or type has changed, this is surely something different
	tpe := nodeTypeFromFileInfo(path, fi)
	if node.Name != fi.Name() || node.Type != tpe {
		debug.Log("node.isNewer", "node %v is newer: name or type changed", path)
		return false
	}

	stat := fi.Sys().(*syscall.Win32FileAttributeData)

	changeTime := time.Unix(0, stat.LastWriteTime.Nanoseconds())

	//same here
	//file,err:=os.Open(path)
	//if err!=nil{
	//	return err
	//}
	//byhandlefi := &syscall.ByHandleFileInformation{}
	//
	//err=syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), byhandlefi)
	//if err!=nil{
	//	return err
	//}

	//inode := uint64(byhandlefi.FileIndexLow) | uint64(byhandlefi.FileIndexHigh)<<32

	//we can use latter
	size := uint64(fi.Size())

	// if timestamps or inodes differ, content has changed
	if node.ModTime != fi.ModTime() ||
		node.ChangeTime != changeTime ||
		//node.Inode != inode ||
		node.Size != size {
		debug.Log("node.isNewer", "node %v is newer: timestamp or inode changed", path)
		return false
	}

	// otherwise the node is assumed to have the same content
	debug.Log("node.isNewer", "node %v is not newer", path)
	return false
}
