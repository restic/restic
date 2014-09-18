package chunker

import (
	"io"
	"sync"
)

const (
	KiB = 1024
	MiB = 1024 * KiB

	// randomly generated irreducible polynomial of degree 53 in Z_2[X]
	Polynomial = 0x3DA3358B4DC173

	// use a sliding window of 64 byte.
	WindowSize = 64

	// aim to create chunks of 20 bits or about 1MiB on average.
	AverageBits = 20

	// Chunks should be in the range of 512KiB to 8MiB.
	MinSize = 512 * KiB
	MaxSize = 8 * MiB

	splitmask = (1 << AverageBits) - 1
)

var (
	pol_shift = deg(Polynomial) - 8
	once      sync.Once
	mod_table [256]uint64
	out_table [256]uint64
)

// A chunk is one content-dependent chunk of bytes whose end was cut when the
// Rabin Fingerprint had the value stored in Cut.
type Chunk struct {
	Start  int
	Length int
	Cut    uint64
	Data   []byte
}

// A chunker takes a stream of bytes and emits average size chunks.
type Chunker interface {
	// Next returns the next chunk of data. If an error occurs while reading,
	// the error is returned. The state of the current chunk is undefined. When
	// the last chunk has been returned, all subsequent calls yield a nil chunk
	// and an io.EOF error.
	Next() (*Chunk, error)
}

// A chunker internally holds everything needed to split content.
type chunker struct {
	rd     io.Reader
	closed bool

	window []byte
	wpos   int

	buf  []byte
	bpos int
	bmax int

	data  []byte
	start int
	count int
	pos   int

	digest uint64
}

// New returns a new Chunker that reads from data from rd.
func New(rd io.Reader) Chunker {
	c := &chunker{
		rd: rd,

		window: make([]byte, WindowSize),

		buf: make([]byte, MaxSize),

		data: make([]byte, 0, MaxSize),
	}

	once.Do(c.fill_tables)
	c.reset()

	return c
}

func (c *chunker) reset() {
	for i := 0; i < WindowSize; i++ {
		c.window[i] = 0
	}
	c.digest = 0
	c.wpos = 0
	c.pos = 0
	c.count = 0
	c.slide(1)
	c.data = make([]byte, 0, MaxSize)
}

// Calculate out_table and mod_table for optimization. Must be called only once.
func (c *chunker) fill_tables() {
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
		var hash uint64

		hash = append_byte(hash, byte(b), Polynomial)
		for i := 0; i < WindowSize-1; i++ {
			hash = append_byte(hash, 0, Polynomial)
		}
		out_table[b] = hash
	}

	// calculate table for reduction mod Polynomial
	k := deg(Polynomial)
	for b := 0; b < 256; b++ {
		// mod_table[b] = A | B, where A = (b(x) * x^k mod pol) and  B = b(x) * x^k
		//
		// The 8 bits above deg(Polynomial) determine what happens next and so
		// these bits are used as a lookup to this table. The value is split in
		// two parts: Part A contains the result of the modulus operation, part
		// B is used to cancel out the 8 top bits so that one XOR operation is
		// enough to reduce modulo Polynomial
		mod_table[b] = mod(uint64(b)<<uint(k), Polynomial) | (uint64(b) << uint(k))
	}
}

func (c *chunker) Next() (*Chunk, error) {
	for {
		if c.bpos >= c.bmax {
			n, err := io.ReadFull(c.rd, c.buf)

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

				// return current chunk
				return &Chunk{
					Start:  c.start,
					Length: c.count,
					Cut:    c.digest,
					Data:   c.data,
				}, nil
			}

			if err != nil {
				return nil, err
			}

			c.bpos = 0
			c.bmax = n
		}

		for i, b := range c.buf[c.bpos:c.bmax] {
			// inline c.slide(b) and append(b) to increase performance
			out := c.window[c.wpos]
			c.window[c.wpos] = b
			c.digest ^= out_table[out]
			c.wpos = (c.wpos + 1) % WindowSize

			// c.append(b)
			index := c.digest >> uint(pol_shift)
			c.digest <<= 8
			c.digest |= uint64(b)

			c.digest ^= mod_table[index]

			if (c.count+i+1 >= MinSize && (c.digest&splitmask) == 0) || c.count+i+1 >= MaxSize {
				c.data = append(c.data, c.buf[c.bpos:c.bpos+i]...)
				c.count += i + 1
				c.pos += i + 1
				c.bpos += i + 1

				chunk := &Chunk{
					Start:  c.start,
					Length: c.count,
					Cut:    c.digest,
					Data:   c.data,
				}

				// keep position
				pos := c.pos
				c.reset()
				c.pos = pos
				c.start = pos

				return chunk, nil
			}
		}

		steps := c.bmax - c.bpos
		if steps > 0 {
			c.data = append(c.data, c.buf[c.bpos:c.bpos+steps]...)
		}
		c.count += steps
		c.pos += steps
		c.bpos = c.bmax
	}

	return nil, nil
}

func (c *chunker) append(b byte) {
	index := c.digest >> uint(pol_shift)
	c.digest <<= 8
	c.digest |= uint64(b)

	c.digest ^= mod_table[index]
}

func (c *chunker) slide(b byte) {
	out := c.window[c.wpos]
	c.window[c.wpos] = b
	c.digest ^= out_table[out]
	c.wpos = (c.wpos + 1) % WindowSize

	c.append(b)
}

func append_byte(hash uint64, b byte, pol uint64) uint64 {
	hash <<= 8
	hash |= uint64(b)

	return mod(hash, pol)
}

// Mod calculates the remainder of x divided by p.
func mod(x, p uint64) uint64 {
	for deg(x) >= deg(p) {
		shift := uint(deg(x) - deg(p))

		x = x ^ (p << shift)
	}

	return x
}

// Deg returns the degree of the polynomial p, this is equivalent to the number
// of the highest bit set in p.
func deg(p uint64) int {
	var mask uint64 = 0x8000000000000000

	for i := 0; i < 64; i++ {
		if mask&p > 0 {
			return 63 - i
		}

		mask >>= 1
	}

	return -1
}
