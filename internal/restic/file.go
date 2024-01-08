package restic

import (
	"io"
)

type LazyFileMetadata interface {
	FileMetadata
	Exist() bool
	Clean() LazyFileMetadata
	Abs() (LazyFileMetadata, error)
	PathComponents(includeRelative bool) (components []string, virtualPrefix bool)
	RootDirectory() RootDirectory
	OpenFile() (FileContent, error)
	Init() error
}

type FileMetadata interface {
	FilePath
	Name() string   // base name of the file
	Size() int64    // length in bytes for regular files; system-dependent for others
	Mode() FileMode // file mode
	FileChanged(node *Node) bool
	Node(snPath string, withAtime bool) (*Node, error)
	Children() ([]LazyFileMetadata, error)
	ChildrenWithFlag(flags int) ([]LazyFileMetadata, error)
	DeviceID() (deviceID uint64, err error)
	Equal(FileMetadata) bool
}

type FileMode int

const (
	REGULAR FileMode = iota
	DIRECTORY
	SOCKET
	OTHER
)

func (m FileMode) IsRegular() bool {
	return m == REGULAR
}

func (m FileMode) IsDir() bool {
	return m == DIRECTORY
}

type FilePath interface {
	Path() string
	AbsPath() (string, error)
}

type FileContent interface {
	io.Reader
	io.Closer
	Metadata() (FileMetadata, error)
}

type RootDirectory interface {
	Path() string
	Join(string) RootDirectory
	Equal(RootDirectory) bool
	StatDir() (FileMetadata, error)
}
