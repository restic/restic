package restic

// Pack stores the pack ID and the contents of a pack file.
type Pack struct {
	ID    ID
	Blobs []Blob
}
