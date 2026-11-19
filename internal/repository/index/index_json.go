package index

import (
	"bytes"
	"encoding/hex"
	"math"

	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
)

// A hand-written reader for the index format.
//
// Loading the index dominates the cost of every restic command on a large
// repository, and profiling shows encoding/json's reflection-based decoder
// accounting for roughly half of it: checkValid walks the whole buffer once
// just to validate it, then rescanLiteral and stateInString walk it again per
// value, and every pack and blob is materialised into a packJSON/blobJSON
// slice that is copied into the index and immediately discarded.
//
// The schema here is fixed and tiny -- hex IDs, small integers, and a
// two-valued type enum -- so parsing it directly is both much faster and
// allocation-free: entries are stored into the index as they are read.
//
// The parser is deliberately narrow. It recognises exactly the constructs
// restic's own encoder emits, and anything else -- escapes in strings,
// fractional or exponent numbers, duplicate keys, malformed input -- makes it
// bounce, so DecodeIndex re-parses the buffer with encoding/json. That keeps
// encoding/json as the reference implementation for every unusual input and
// as the single source of the error messages users see, which matters more
// here than squeezing out the last few per cent.

// maxSkipDepth bounds recursion while skipping a field the index format does
// not define. The real format nests three deep; anything beyond this bounces
// rather than risking the stack.
const maxSkipDepth = 32

// The members each object defines, for skippable below.
var (
	keysIndex = [][]byte{[]byte("packs")}
	keysPack  = [][]byte{[]byte("id"), []byte("blobs")}
	keysBlob  = [][]byte{
		[]byte("id"), []byte("type"), []byte("offset"),
		[]byte("length"), []byte("uncompressed_length"),
	}
)

// skippable reports whether key can be treated as a member the format does not
// define, and therefore ignored.
//
// It is not enough for the key to differ from every defined name: when no field
// matches a JSON key exactly, encoding/json falls back to a case-insensitive
// match, so `{"pACks": ...}` still decodes into Packs. Rather than reimplement
// that folding, anything that could match under bytes.EqualFold -- which is the
// relation encoding/json's own foldName is documented to implement -- is left
// to encoding/json.
func skippable(key []byte, known [][]byte) bool {
	for _, k := range known {
		if bytes.EqualFold(key, k) {
			return false
		}
	}
	return true
}

// jsonParser reads a JSON document without allocating.
type jsonParser struct {
	buf []byte
	pos int
}

// decodeIndexFast parses buf into a new Index. It reports false if the input
// contains anything it does not handle, in which case nothing may be assumed
// about the index it returns.
func decodeIndexFast(buf []byte) (*Index, bool) {
	p := jsonParser{buf: buf}
	idx := NewIndex()

	if !p.accept('{') {
		return nil, false
	}
	if !p.accept('}') {
		seenPacks := false
		for {
			key, ok := p.str()
			if !ok || !p.accept(':') {
				return nil, false
			}

			if string(key) == "packs" {
				// A repeated key would be last-one-wins for encoding/json but
				// append-both here, so hand it over rather than diverge.
				if seenPacks {
					return nil, false
				}
				seenPacks = true
				if !p.packs(idx) {
					return nil, false
				}
			} else if !skippable(key, keysIndex) || !p.skip() {
				return nil, false
			}

			if p.accept(',') {
				continue
			}
			if p.accept('}') {
				break
			}
			return nil, false
		}
	}

	// json.Unmarshal rejects anything but whitespace after the value.
	p.ws()
	if p.pos != len(p.buf) {
		return nil, false
	}

	return idx, true
}

// packs reads the value of the top-level "packs" member.
func (p *jsonParser) packs(idx *Index) bool {
	if p.peek() == 'n' {
		return p.literal("null")
	}
	if !p.accept('[') {
		return false
	}
	if p.accept(']') {
		return true
	}
	for {
		if !p.pack(idx) {
			return false
		}
		if p.accept(',') {
			continue
		}
		if p.accept(']') {
			return true
		}
		return false
	}
}

// pack reads one pack object and stores its blobs.
func (p *jsonParser) pack(idx *Index) bool {
	if !p.accept('{') {
		return false
	}

	// Reserve the pack's slot up front and fill in its ID when the "id" member
	// turns up. restic writes "id" before "blobs", but this way the order does
	// not matter, and a pack object with no "id" at all still reserves a slot
	// holding the zero ID -- which is what decoding into packJSON does.
	packIdx := idx.addToPacks(restic.ID{})

	if p.accept('}') {
		return true
	}

	seenID, seenBlobs := false, false
	for {
		key, ok := p.str()
		if !ok || !p.accept(':') {
			return false
		}

		switch string(key) {
		case "id":
			if seenID || !p.id(&idx.packs[packIdx]) {
				return false
			}
			seenID = true
		case "blobs":
			if seenBlobs || !p.blobs(idx, packIdx) {
				return false
			}
			seenBlobs = true
		default:
			if !skippable(key, keysPack) || !p.skip() {
				return false
			}
		}

		if p.accept(',') {
			continue
		}
		if p.accept('}') {
			return true
		}
		return false
	}
}

// blobs reads a pack's "blobs" array, storing each entry against packIdx.
func (p *jsonParser) blobs(idx *Index, packIdx uint32) bool {
	if p.peek() == 'n' {
		return p.literal("null")
	}
	if !p.accept('[') {
		return false
	}
	if p.accept(']') {
		return true
	}
	for {
		if !p.blob(idx, packIdx) {
			return false
		}
		if p.accept(',') {
			continue
		}
		if p.accept(']') {
			return true
		}
		return false
	}
}

// Bit per recognised blob member, to detect a repeated one.
const (
	blobSeenID = 1 << iota
	blobSeenType
	blobSeenOffset
	blobSeenLength
	blobSeenUncompressedLength
)

// blob reads one blob object and stores it.
func (p *jsonParser) blob(idx *Index, packIdx uint32) bool {
	if !p.accept('{') {
		return false
	}

	var b pack.Blob
	if !p.accept('}') {
		seen := 0
		for {
			key, ok := p.str()
			if !ok || !p.accept(':') {
				return false
			}

			var bit int
			switch string(key) {
			case "id":
				bit = blobSeenID
				if !p.id(&b.ID) {
					return false
				}
			case "type":
				bit = blobSeenType
				if !p.blobType(&b.Type) {
					return false
				}
			case "offset":
				bit = blobSeenOffset
				if !p.sizeVal(&b.Offset) {
					return false
				}
			case "length":
				bit = blobSeenLength
				if !p.sizeVal(&b.Length) {
					return false
				}
			case "uncompressed_length":
				bit = blobSeenUncompressedLength
				if !p.sizeVal(&b.UncompressedLength) {
					return false
				}
			default:
				if !skippable(key, keysBlob) || !p.skip() {
					return false
				}
			}

			if bit != 0 {
				if seen&bit != 0 {
					return false
				}
				seen |= bit
			}

			if p.accept(',') {
				continue
			}
			if p.accept('}') {
				break
			}
			return false
		}
	}

	idx.store(packIdx, b)
	return true
}

// id decodes a 64-character hex string straight into dst. This is the other
// half of the win over encoding/json, which reaches restic.ID.UnmarshalJSON
// only after copying the quoted string into a fresh []byte.
func (p *jsonParser) id(dst *restic.ID) bool {
	s, ok := p.str()
	if !ok || len(s) != hex.EncodedLen(len(dst)) {
		return false
	}
	_, err := hex.Decode(dst[:], s)
	return err == nil
}

func (p *jsonParser) blobType(dst *restic.BlobType) bool {
	s, ok := p.str()
	if !ok {
		return false
	}
	switch string(s) {
	case "data":
		*dst = restic.DataBlob
	case "tree":
		*dst = restic.TreeBlob
	default:
		return false
	}
	return true
}

// sizeVal reads an offset or length. Values above MaxUint32 bounce: Index.store
// panics on them, and encoding/json should stay the one that gets there.
func (p *jsonParser) sizeVal(dst *uint) bool {
	v, ok := p.uintVal()
	if !ok || v > math.MaxUint32 {
		return false
	}
	*dst = uint(v)
	return true
}

// ---------------------------------------------------------------- scanning

func (p *jsonParser) ws() {
	for p.pos < len(p.buf) {
		switch p.buf[p.pos] {
		case ' ', '\t', '\r', '\n':
			p.pos++
		default:
			return
		}
	}
}

// peek returns the next significant byte, or 0 at end of input.
func (p *jsonParser) peek() byte {
	p.ws()
	if p.pos < len(p.buf) {
		return p.buf[p.pos]
	}
	return 0
}

// accept consumes the next significant byte if it is c.
func (p *jsonParser) accept(c byte) bool {
	p.ws()
	if p.pos < len(p.buf) && p.buf[p.pos] == c {
		p.pos++
		return true
	}
	return false
}

func (p *jsonParser) literal(s string) bool {
	p.ws()
	if len(p.buf)-p.pos < len(s) || string(p.buf[p.pos:p.pos+len(s)]) != s {
		return false
	}
	p.pos += len(s)
	return true
}

// str returns the raw contents of a JSON string, as a slice of the input.
// A string holding a backslash bounces: the index format never needs an
// escape, and decoding them is exactly the subtlety this parser stays out of.
func (p *jsonParser) str() ([]byte, bool) {
	if !p.accept('"') {
		return nil, false
	}
	start := p.pos
	for p.pos < len(p.buf) {
		switch c := p.buf[p.pos]; {
		case c == '"':
			s := p.buf[start:p.pos]
			p.pos++
			return s, true
		case c == '\\':
			return nil, false
		case c < 0x20:
			// An unescaped control character is invalid JSON, and
			// encoding/json rejects it. Do not be more permissive.
			return nil, false
		default:
			p.pos++
		}
	}
	return nil, false
}

// uintVal reads a plain non-negative integer, which is all the index format
// uses. A sign, a fraction or an exponent bounces, because encoding/json
// rejects those for an integer field and this parser must not accept more.
func (p *jsonParser) uintVal() (uint64, bool) {
	p.ws()

	start := p.pos
	var v uint64
	for p.pos < len(p.buf) {
		c := p.buf[p.pos]
		if c < '0' || c > '9' {
			break
		}
		if v > (math.MaxUint64-9)/10 {
			return 0, false
		}
		v = v*10 + uint64(c-'0')
		p.pos++
	}
	if p.pos == start {
		return 0, false
	}
	if p.buf[start] == '0' && p.pos-start > 1 {
		return 0, false // leading zero
	}
	if p.pos < len(p.buf) {
		switch p.buf[p.pos] {
		case '.', 'e', 'E':
			return 0, false
		}
	}
	return v, true
}

// skip consumes one complete JSON value, for members the index format does
// not define -- "supersedes", and whatever a future version adds.
func (p *jsonParser) skip() bool { return p.skipDepth(0) }

func (p *jsonParser) skipDepth(depth int) bool {
	if depth > maxSkipDepth {
		return false
	}

	switch p.peek() {
	case '{':
		p.pos++
		if p.accept('}') {
			return true
		}
		for {
			if _, ok := p.str(); !ok {
				return false
			}
			if !p.accept(':') || !p.skipDepth(depth+1) {
				return false
			}
			if p.accept(',') {
				continue
			}
			return p.accept('}')
		}
	case '[':
		p.pos++
		if p.accept(']') {
			return true
		}
		for {
			if !p.skipDepth(depth + 1) {
				return false
			}
			if p.accept(',') {
				continue
			}
			return p.accept(']')
		}
	case '"':
		_, ok := p.str()
		return ok
	case 't':
		return p.literal("true")
	case 'f':
		return p.literal("false")
	case 'n':
		return p.literal("null")
	default:
		return p.skipNumber()
	}
}

// skipNumber consumes a JSON number of any form. Unlike uintVal it only needs
// the number's extent, so it accepts the whole grammar.
func (p *jsonParser) skipNumber() bool {
	p.ws()

	if p.pos < len(p.buf) && p.buf[p.pos] == '-' {
		p.pos++
	}

	intStart := p.pos
	if !p.digits() || (p.buf[intStart] == '0' && p.pos-intStart > 1) {
		return false
	}

	if p.pos < len(p.buf) && p.buf[p.pos] == '.' {
		p.pos++
		if !p.digits() {
			return false
		}
	}

	if p.pos < len(p.buf) && (p.buf[p.pos] == 'e' || p.buf[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.buf) && (p.buf[p.pos] == '+' || p.buf[p.pos] == '-') {
			p.pos++
		}
		if !p.digits() {
			return false
		}
	}

	return true
}

func (p *jsonParser) digits() bool {
	start := p.pos
	for p.pos < len(p.buf) && p.buf[p.pos] >= '0' && p.buf[p.pos] <= '9' {
		p.pos++
	}
	return p.pos > start
}
