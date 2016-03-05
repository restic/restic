package chunker

import (
	"errors"
	"io"
	"sync"
)

const (
	kiB = 1024
	miB = 1024 * kiB

	// WindowSize is the size of the sliding window.
	windowSize = 64

	// aim to create chunks of 20 bits or about 1MiB on average.
	averageBits = 20

	// MinSize is the default minimal size of a chunk.
	MinSize = 512 * kiB
	// MaxSize is the default maximal size of a chunk.
	MaxSize = 8 * miB

	splitmask = (1 << averageBits) - 1

	chunkerBufSize = 512 * kiB
)

type tables struct {
	out [256]Pol
	mod [256]Pol
}

// cache precomputed tables, these are read-only anyway
var cache struct {
	entries map[Pol]tables
	sync.Mutex
}

func init() {
	cache.entries = make(map[Pol]tables)
}

// Chunk is one content-dependent chunk of bytes whose end was cut when the
// Rabin Fingerprint had the value stored in Cut.
type Chunk struct {
	Start  uint
	Length uint
	Cut    uint64
	Data   []byte
}

type chunkerState struct {
	window [windowSize]byte
	wpos   int

	buf  []byte
	bpos uint
	bmax uint

	start uint
	count uint
	pos   uint

	pre uint // wait for this many bytes before start calculating an new chunk

	digest uint64
}

type chunkerConfig struct {
	MinSize, MaxSize uint

	pol               Pol
	polShift          uint
	tables            tables
	tablesInitialized bool

	rd     io.Reader
	closed bool
}

// Chunker splits content with Rabin Fingerprints.
type Chunker struct {
	chunkerConfig
	chunkerState
}

// New returns a new Chunker based on polynomial p that reads from rd
// with bufsize and pass all data to hash along the way.
func New(rd io.Reader, pol Pol) *Chunker {
	c := &Chunker{
		chunkerState: chunkerState{
			buf: make([]byte, chunkerBufSize),
		},
		chunkerConfig: chunkerConfig{
			pol:     pol,
			rd:      rd,
			MinSize: MinSize,
			MaxSize: MaxSize,
		},
	}

	c.reset()

	return c
}

// Reset reinitializes the chunker with a new reader and polynomial.
func (c *Chunker) Reset(rd io.Reader, pol Pol) {
	*c = Chunker{
		chunkerState: chunkerState{
			buf: c.buf,
		},
		chunkerConfig: chunkerConfig{
			pol:     pol,
			rd:      rd,
			MinSize: MinSize,
			MaxSize: MaxSize,
		},
	}

	c.reset()
}

func (c *Chunker) reset() {
	c.polShift = uint(c.pol.Deg() - 8)
	c.fillTables()

	for i := 0; i < windowSize; i++ {
		c.window[i] = 0
	}

	c.closed = false
	c.digest = 0
	c.wpos = 0
	c.count = 0
	c.digest = c.slide(c.digest, 1)
	c.start = c.pos

	// do not start a new chunk unless at least MinSize bytes have been read
	c.pre = c.MinSize - windowSize
}

// Calculate out_table and mod_table for optimization. Must be called only
// once. This implementation uses a cache in the global variable cache.
func (c *Chunker) fillTables() {
	// if polynomial hasn't been specified, do not compute anything for now
	if c.pol == 0 {
		return
	}

	c.tablesInitialized = true

	// test if the tables are cached for this polynomial
	cache.Lock()
	defer cache.Unlock()
	if t, ok := cache.entries[c.pol]; ok {
		c.tables = t
		return
	}

	// calculate table for sliding out bytes. The byte to slide out is used as
	// the index for the table, the value contains the following:
	// out_table[b] = Hash(b || 0 ||        ...        || 0)
	//                          \ windowsize-1 zero bytes /
	// To slide out byte b_0 for window size w with known hash
	// H := H(b_0 || ... || b_w), it is sufficient to add out_table[b_0]:
	//    H(b_0 || ... || b_w) + H(b_0 || 0 || ... || 0)
	//  = H(b_0 + b_0 || b_1 + 0 || ... || b_w + 0)
	//  = H(    0     || b_1 || ...     || b_w)
	//
	// Afterwards a new byte can be shifted in.
	for b := 0; b < 256; b++ {
		var h Pol

		h = appendByte(h, byte(b), c.pol)
		for i := 0; i < windowSize-1; i++ {
			h = appendByte(h, 0, c.pol)
		}
		c.tables.out[b] = h
	}

	// calculate table for reduction mod Polynomial
	k := c.pol.Deg()
	for b := 0; b < 256; b++ {
		// mod_table[b] = A | B, where A = (b(x) * x^k mod pol) and  B = b(x) * x^k
		//
		// The 8 bits above deg(Polynomial) determine what happens next and so
		// these bits are used as a lookup to this table. The value is split in
		// two parts: Part A contains the result of the modulus operation, part
		// B is used to cancel out the 8 top bits so that one XOR operation is
		// enough to reduce modulo Polynomial
		c.tables.mod[b] = Pol(uint64(b)<<uint(k)).Mod(c.pol) | (Pol(b) << uint(k))
	}

	cache.entries[c.pol] = c.tables
}

// Next returns the position and length of the next chunk of data. If an error
// occurs while reading, the error is returned. Afterwards, the state of the
// current chunk is undefined. When the last chunk has been returned, all
// subsequent calls yield an io.EOF error.
func (c *Chunker) Next(data []byte) (Chunk, error) {
	data = data[:0]
	if !c.tablesInitialized {
		return Chunk{}, errors.New("tables for polynomial computation not initialized")
	}

	tabout := c.tables.out
	tabmod := c.tables.mod
	polShift := c.polShift
	minSize := c.MinSize
	maxSize := c.MaxSize
	buf := c.buf
	for {
		if c.bpos >= c.bmax {
			n, err := io.ReadFull(c.rd, buf[:])

			if err == io.ErrUnexpectedEOF {
				err = nil
			}

			// io.ReadFull only returns io.EOF when no bytes could be read. If
			// this is the case and we're in this branch, there are no more
			// bytes to buffer, so this was the last chunk. If a different
			// error has occurred, return that error and abandon the current
			// chunk.
			if err == io.EOF && !c.closed {
				c.closed = true

				// return current chunk, if any bytes have been processed
				if c.count > 0 {
					return Chunk{
						Start:  c.start,
						Length: c.count,
						Cut:    c.digest,
						Data:   data,
					}, nil
				}
			}

			if err != nil {
				return Chunk{}, err
			}

			c.bpos = 0
			c.bmax = uint(n)
		}

		// check if bytes have to be dismissed before starting a new chunk
		if c.pre > 0 {
			n := c.bmax - c.bpos
			if c.pre > uint(n) {
				c.pre -= uint(n)
				data = append(data, buf[c.bpos:c.bmax]...)

				c.count += uint(n)
				c.pos += uint(n)
				c.bpos = c.bmax

				continue
			}

			data = append(data, buf[c.bpos:c.bpos+c.pre]...)

			c.bpos += c.pre
			c.count += c.pre
			c.pos += c.pre
			c.pre = 0
		}

		add := c.count
		digest := c.digest
		win := c.window
		wpos := c.wpos
		for _, b := range buf[c.bpos:c.bmax] {
			// slide(b)
			out := win[wpos]
			win[wpos] = b
			digest ^= uint64(tabout[out])
			wpos = (wpos + 1) % windowSize

			// updateDigest
			index := byte(digest >> polShift)
			digest <<= 8
			digest |= uint64(b)

			digest ^= uint64(tabmod[index])
			// end manual inline

			add++
			if add < minSize {
				continue
			}

			if (digest&splitmask) == 0 || add >= maxSize {
				i := add - c.count - 1
				data = append(data, c.buf[c.bpos:c.bpos+uint(i)+1]...)
				c.count = add
				c.pos += uint(i) + 1
				c.bpos += uint(i) + 1
				c.buf = buf

				chunk := Chunk{
					Start:  c.start,
					Length: c.count,
					Cut:    digest,
					Data:   data,
				}

				c.reset()

				return chunk, nil
			}
		}
		c.digest = digest
		c.window = win
		c.wpos = wpos

		steps := c.bmax - c.bpos
		if steps > 0 {
			data = append(data, c.buf[c.bpos:c.bpos+steps]...)
		}
		c.count += steps
		c.pos += steps
		c.bpos = c.bmax
	}
}

func updateDigest(digest uint64, polShift uint, tab tables, b byte) (newDigest uint64) {
	index := digest >> polShift
	digest <<= 8
	digest |= uint64(b)

	digest ^= uint64(tab.mod[index])
	return digest
}

func (c *Chunker) slide(digest uint64, b byte) (newDigest uint64) {
	out := c.window[c.wpos]
	c.window[c.wpos] = b
	digest ^= uint64(c.tables.out[out])
	c.wpos = (c.wpos + 1) % windowSize

	digest = updateDigest(digest, c.polShift, c.tables, b)
	return digest
}

func appendByte(hash Pol, b byte, pol Pol) Pol {
	hash <<= 8
	hash |= Pol(b)

	return hash.Mod(pol)
}
