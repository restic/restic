package restic

// FileType is the type of a file in the repository.
// Numeric values must match backend.FileType; enforced in internal/repository/filetype.go.
type FileType uint8

// These are the different data types a backend can store.
const (
	PackFile FileType = 1 + iota
	KeyFile
	LockFile
	SnapshotFile
	IndexFile
	ConfigFile
)

// Keep in sync with backend.FileType.String().
func (t FileType) String() string {
	s := "invalid"
	switch t {
	case PackFile:
		// Spelled "data" instead of "pack" for historical reasons.
		s = "data"
	case KeyFile:
		s = "key"
	case LockFile:
		s = "lock"
	case SnapshotFile:
		s = "snapshot"
	case IndexFile:
		s = "index"
	case ConfigFile:
		s = "config"
	}
	return s
}

// WriteableFileType defines the different data types that can be modified via SaveUnpacked or RemoveUnpacked.
type WriteableFileType FileType

const (
	// WriteableSnapshotFile is the WriteableFileType for snapshots.
	WriteableSnapshotFile = WriteableFileType(SnapshotFile)
)

func (w *WriteableFileType) ToFileType() FileType {
	switch *w {
	case WriteableSnapshotFile:
		return SnapshotFile
	default:
		panic("invalid WriteableFileType")
	}
}

type FileTypes interface {
	FileType | WriteableFileType
}
