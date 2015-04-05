package chunker

import (
	"errors"
	"hash"
	"io"
	"sync"
)

const (
	KiB = 1024
	MiB = 1024 * KiB

	// WindowSize is the size of the sliding window.
	WindowSize = 64

	// aim to create chunks of 20 bits or about 1MiB on average.
	AverageBits = 20

	// Chunks should be in the range of 512KiB to 8MiB.
	MinSize = 512 * KiB
	MaxSize = 8 * MiB

	splitmask = (1 << AverageBits) - 1
)

type tables struct {
	out [256]uint64
	mod [256]uint64
}

// cache precomputed tables, these are read-only anyway
var cache struct {
	entries map[Pol]*tables
	sync.Mutex
}

func init() {
	cache.entries = make(map[Pol]*tables)
}

// A chunk is one content-dependent chunk of bytes whose end was cut when the
// Rabin Fingerprint had the value stored in Cut.
type Chunk struct {
	Start  uint
	Length uint
	Cut    uint64
	Digest []byte
}

func (c Chunk) Reader(r io.ReaderAt) io.Reader {
	return io.NewSectionReader(r, int64(c.Start), int64(c.Length))
}

// A chunker internally holds everything needed to split content.
type Chunker struct {
	pol       Pol
	pol_shift uint
	tables    *tables

	rd     io.Reader
	closed bool

	window [WindowSize]byte
	wpos   int

	buf  []byte
	bpos uint
	bmax uint

	start uint
	count uint
	pos   uint

	pre uint // wait for this many bytes before start calculating an new chunk

	digest uint64
	h      hash.Hash
}

// New returns a new Chunker based on polynomial p that reads from data from rd
// with bufsize and pass all data to hash along the way.
func New(rd io.Reader, p Pol, bufsize int, hash hash.Hash) (*Chunker, error) {
	c := &Chunker{
		buf: make([]byte, bufsize),
		h:   hash,
	}
	if err := c.Reset(rd, p); err != nil {
		return nil, err
	}

	return c, nil
}

// Reset restarts a chunker so that it can be reused with a different
// polynomial and reader.
func (c *Chunker) Reset(rd io.Reader, p Pol) error {
	c.pol = p
	c.pol_shift = uint(p.Deg() - 8)
	if err := c.fill_tables(); err != nil {
		return err
	}

	c.rd = rd

	for i := 0; i < WindowSize; i++ {
		c.window[i] = 0
	}
	c.closed = false
	c.digest = 0
	c.wpos = 0
	c.pos = 0
	c.start = 0
	c.count = 0
	c.slide(1)

	if c.h != nil {
		c.h.Reset()
	}

	// do not start a new chunk unless at least MinSize bytes have been read
	c.pre = MinSize - WindowSize

	return nil
}

// Calculate out_table and mod_table for optimization. Must be called only
// once. This implementation uses a cache in the global variable cache.
func (c *Chunker) fill_tables() error {
	// test if the tables are cached for this polynomial
	cache.Lock()
	defer cache.Unlock()
	if t, ok := cache.entries[c.pol]; ok {
		c.tables = t
		return nil
	}

	// else create a new entry
	c.tables = &tables{}
	cache.entries[c.pol] = c.tables

	// test irreducibility of p
	if !c.pol.Irreducible() {
		return errors.New("invalid polynomial")
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
		var hash uint64

		hash = append_byte(hash, byte(b), uint64(c.pol))
		for i := 0; i < WindowSize-1; i++ {
			hash = append_byte(hash, 0, uint64(c.pol))
		}
		c.tables.out[b] = hash
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
		c.tables.mod[b] = mod(uint64(b)<<uint(k), uint64(c.pol)) | (uint64(b) << uint(k))
	}

	return nil
}

// Next returns the position and length of the next chunk of data. If an error
// occurs while reading, the error is returned with a nil chunk. The state of
// the current chunk is undefined. When the last chunk has been returned, all
// subsequent calls yield a nil chunk and an io.EOF error.
func (c *Chunker) Next() (*Chunk, error) {
	for {
		if c.bpos >= c.bmax {
			n, err := io.ReadFull(c.rd, c.buf[:])

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
					return &Chunk{
						Start:  c.start,
						Length: c.count,
						Cut:    c.digest,
						Digest: c.hashDigest(),
					}, nil
				}
			}

			if err != nil {
				return nil, err
			}

			c.bpos = 0
			c.bmax = uint(n)
		}

		// check if bytes have to be dismissed before starting a new chunk
		if c.pre > 0 {
			n := c.bmax - c.bpos
			if c.pre > uint(n) {
				c.pre -= uint(n)
				c.updateHash(c.buf[c.bpos:c.bmax])

				c.count += uint(n)
				c.pos += uint(n)
				c.bpos = c.bmax

				continue
			}

			c.updateHash(c.buf[c.bpos : c.bpos+c.pre])

			c.bpos += c.pre
			c.count += c.pre
			c.pos += c.pre
			c.pre = 0
		}

		add := c.count
		for _, b := range c.buf[c.bpos:c.bmax] {
			// inline c.slide(b) and append(b) to increase performance
			out := c.window[c.wpos]
			c.window[c.wpos] = b
			c.digest ^= c.tables.out[out]
			c.wpos = (c.wpos + 1) % WindowSize

			// c.append(b)
			index := c.digest >> c.pol_shift
			c.digest <<= 8
			c.digest |= uint64(b)

			c.digest ^= c.tables.mod[index]
			// end inline

			add++
			if add < MinSize {
				continue
			}

			if (c.digest&splitmask) == 0 || add >= MaxSize {
				i := add - c.count - 1
				c.updateHash(c.buf[c.bpos : c.bpos+uint(i)+1])
				c.count = add
				c.pos += uint(i) + 1
				c.bpos += uint(i) + 1

				chunk := &Chunk{
					Start:  c.start,
					Length: c.count,
					Cut:    c.digest,
					Digest: c.hashDigest(),
				}

				if c.h != nil {
					c.h.Reset()
				}

				// reset chunker, but keep position
				pos := c.pos
				c.Reset(c.rd, c.pol)
				c.pos = pos
				c.start = pos
				c.pre = MinSize - WindowSize

				return chunk, nil
			}
		}

		steps := c.bmax - c.bpos
		if steps > 0 {
			c.updateHash(c.buf[c.bpos : c.bpos+steps])
		}
		c.count += steps
		c.pos += steps
		c.bpos = c.bmax
	}
}

func (c *Chunker) updateHash(data []byte) {
	if c.h != nil {
		// the hashes from crypto/sha* do not return an error
		_, err := c.h.Write(data)
		if err != nil {
			panic(err)
		}
	}
}

func (c *Chunker) hashDigest() []byte {
	if c.h == nil {
		return nil
	}

	return c.h.Sum(nil)
}

func (c *Chunker) append(b byte) {
	index := c.digest >> c.pol_shift
	c.digest <<= 8
	c.digest |= uint64(b)

	c.digest ^= c.tables.mod[index]
}

func (c *Chunker) slide(b byte) {
	out := c.window[c.wpos]
	c.window[c.wpos] = b
	c.digest ^= c.tables.out[out]
	c.wpos = (c.wpos + 1) % WindowSize

	c.append(b)
}

func append_byte(hash uint64, b byte, pol uint64) uint64 {
	hash <<= 8
	hash |= uint64(b)

	return mod(hash, pol)
}

// Mod calculates the remainder of x divided by p in F_2[X].
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
