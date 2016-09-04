package restic

// Cache stores blobs locally.
type Cache interface {
	GetBlob(BlobHandle, []byte) (bool, error)
	PutBlob(BlobHandle, []byte) error
	DeleteBlob(BlobHandle) error
	HasBlob(BlobHandle) bool
}
