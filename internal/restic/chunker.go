package restic

// Chunker splits file content into variable-length chunks.
// Implementations are created by ChunkerFactory and reused across files via Reset.
type Chunker interface {
	// Reset reinitializes the chunker for a new file.
	Reset()
	// NextSplitPoint scans buf for a chunk boundary.
	// Returns index before which to split buf, or -1 if no boundary found in this buffer.
	// This operation is stateful. All buffers passed to it until a split point is found
	// then form a single chunk.
	NextSplitPoint(buf []byte) int
}

// ChunkerFactory creates chunkers configured for a specific repository.
type ChunkerFactory interface {
	NewChunker() Chunker
	// MaxChunkSize is the maximum size of a single chunk (used for output buffer pools).
	MaxChunkSize() int
	// ZeroChunk returns the ID of an all-zero chunk with minimum chunk size.
	ZeroChunk() ID
}
