package repository

import (
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/restic"
)

func beFileType(t restic.FileType) backend.FileType {
	return backend.FileType(t)
}

func beHandle(t restic.FileType, name string) backend.Handle {
	return backend.Handle{Type: backend.FileType(t), Name: name}
}

func beHandleWithMetadata(t restic.FileType, name string, isMetadata bool) backend.Handle {
	return backend.Handle{Type: backend.FileType(t), Name: name, IsMetadata: isMetadata}
}

// Compile-time checks that restic and backend FileType constants match. A constant mismatch
// would be an out-of-bounds access that is detected by the compiler.
var (
	_ = [1]struct{}{}[backend.PackFile-backend.FileType(restic.PackFile)]
	_ = [1]struct{}{}[backend.KeyFile-backend.FileType(restic.KeyFile)]
	_ = [1]struct{}{}[backend.LockFile-backend.FileType(restic.LockFile)]
	_ = [1]struct{}{}[backend.SnapshotFile-backend.FileType(restic.SnapshotFile)]
	_ = [1]struct{}{}[backend.IndexFile-backend.FileType(restic.IndexFile)]
	_ = [1]struct{}{}[backend.ConfigFile-backend.FileType(restic.ConfigFile)]
)
