package restic

// Cache stores files and blobs locally.
type Cache interface {
	GetBlob(BlobHandle, []byte) (bool, error)
	PutBlob(BlobHandle, []byte) error
	DeleteBlob(BlobHandle) error
	HasBlob(BlobHandle) bool
	UpdateBlobs(idx BlobIndex) error

	GetFile(Handle, []byte) (bool, error)
	PutFile(Handle, []byte) error
	DeleteFile(Handle) error
	HasFile(Handle) bool
	UpdateFiles(idx FileIndex) error
}

// BlobIndex returns information about blobs stored in a repo.
type BlobIndex interface {
	Has(id ID, t BlobType) bool
}

// FileIndex returns information about files in a backend.
type FileIndex interface {
	Test(t FileType, name string) (bool, error)
}
