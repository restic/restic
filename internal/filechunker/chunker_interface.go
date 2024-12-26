package filechunker

type ChunkerI interface {
	Next() (ChunkI, error)
}

type ChunkI interface {
	PcHash() [32]byte // precomputed hash or zero-byte filled value to indicate missing pre computed hash
	Size() uint64
	Data() []byte // should fetch data lazily on first call
	Release()
}

type RawDataChunk struct {
	buf  []byte
	hash [32]byte
}

func NewRawDataChunk(buf []byte) *RawDataChunk {
	return &RawDataChunk{
		buf:  buf,
		hash: [32]byte{}, // return zero-id to signal that it needs to be computed
	}
}

func NewRawDataChunkWithPreComputedHash(buf []byte, hash [32]byte) *RawDataChunk {
	return &RawDataChunk{
		buf:  buf,
		hash: hash,
	}
}

// Data implements ChunkI.
func (r *RawDataChunk) Data() []byte {
	return r.buf
}

// Hash implements ChunkI.
func (r *RawDataChunk) PcHash() [32]byte {
	return r.hash
}

// Release implements ChunkI.
func (r *RawDataChunk) Release() {
	// ignore
}

// Size implements ChunkI.
func (r *RawDataChunk) Size() uint64 {
	return uint64(len(r.buf))
}
