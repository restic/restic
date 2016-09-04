package restic

// Cache stores blobs locally.
type Cache interface {
	GetBlob(BlobHandle, []byte) (bool, error)
	PutBlob(BlobHandle, []byte) error
	DeleteBlob(BlobHandle) error
	HasBlob(BlobHandle) bool
	UpdateBlobs(idx BlobIndex) error
}

// BlobIndex returns information about blobs stored in a repo.
type BlobIndex interface {
	Has(id ID, t BlobType) bool
}
