package chunker_test

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"io/ioutil"
	"math/rand"
	"testing"
	"time"

	"github.com/restic/chunker"
	. "github.com/restic/restic/test"
)

func parseDigest(s string) []byte {
	d, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}

	return d
}

type chunk struct {
	Length uint
	CutFP  uint64
	Digest []byte
}

// polynomial used for all the tests below
const testPol = chunker.Pol(0x3DA3358B4DC173)

// created for 32MB of random data out of math/rand's Uint32() seeded by
// constant 23
//
// chunking configuration:
// window size 64, avg chunksize 1<<20, min chunksize 1<<19, max chunksize 1<<23
// polynom 0x3DA3358B4DC173
var chunks1 = []chunk{
	chunk{2163460, 0x000b98d4cdf00000, parseDigest("4b94cb2cf293855ea43bf766731c74969b91aa6bf3c078719aabdd19860d590d")},
	chunk{643703, 0x000d4e8364d00000, parseDigest("5727a63c0964f365ab8ed2ccf604912f2ea7be29759a2b53ede4d6841e397407")},
	chunk{1528956, 0x0015a25c2ef00000, parseDigest("a73759636a1e7a2758767791c69e81b69fb49236c6929e5d1b654e06e37674ba")},
	chunk{1955808, 0x00102a8242e00000, parseDigest("c955fb059409b25f07e5ae09defbbc2aadf117c97a3724e06ad4abd2787e6824")},
	chunk{2222372, 0x00045da878000000, parseDigest("6ba5e9f7e1b310722be3627716cf469be941f7f3e39a4c3bcefea492ec31ee56")},
	chunk{2538687, 0x00198a8179900000, parseDigest("8687937412f654b5cfe4a82b08f28393a0c040f77c6f95e26742c2fc4254bfde")},
	chunk{609606, 0x001d4e8d17100000, parseDigest("5da820742ff5feb3369112938d3095785487456f65a8efc4b96dac4be7ebb259")},
	chunk{1205738, 0x000a7204dd600000, parseDigest("cc70d8fad5472beb031b1aca356bcab86c7368f40faa24fe5f8922c6c268c299")},
	chunk{959742, 0x00183e71e1400000, parseDigest("4065bdd778f95676c92b38ac265d361f81bff17d76e5d9452cf985a2ea5a4e39")},
	chunk{4036109, 0x001fec043c700000, parseDigest("b9cf166e75200eb4993fc9b6e22300a6790c75e6b0fc8f3f29b68a752d42f275")},
	chunk{1525894, 0x000b1574b1500000, parseDigest("2f238180e4ca1f7520a05f3d6059233926341090f9236ce677690c1823eccab3")},
	chunk{1352720, 0x00018965f2e00000, parseDigest("afd12f13286a3901430de816e62b85cc62468c059295ce5888b76b3af9028d84")},
	chunk{811884, 0x00155628aa100000, parseDigest("42d0cdb1ee7c48e552705d18e061abb70ae7957027db8ae8db37ec756472a70a")},
	chunk{1282314, 0x001909a0a1400000, parseDigest("819721c2457426eb4f4c7565050c44c32076a56fa9b4515a1c7796441730eb58")},
	chunk{1318021, 0x001cceb980000000, parseDigest("842eb53543db55bacac5e25cb91e43cc2e310fe5f9acc1aee86bdf5e91389374")},
	chunk{948640, 0x0011f7a470a00000, parseDigest("b8e36bf7019bb96ac3fb7867659d2167d9d3b3148c09fe0de45850b8fe577185")},
	chunk{645464, 0x00030ce2d9400000, parseDigest("5584bd27982191c3329f01ed846bfd266e96548dfa87018f745c33cfc240211d")},
	chunk{533758, 0x0004435c53c00000, parseDigest("4da778a25b72a9a0d53529eccfe2e5865a789116cb1800f470d8df685a8ab05d")},
	chunk{1128303, 0x0000c48517800000, parseDigest("08c6b0b38095b348d80300f0be4c5184d2744a17147c2cba5cc4315abf4c048f")},
	chunk{800374, 0x000968473f900000, parseDigest("820284d2c8fd243429674c996d8eb8d3450cbc32421f43113e980f516282c7bf")},
	chunk{2453512, 0x001e197c92600000, parseDigest("5fa870ed107c67704258e5e50abe67509fb73562caf77caa843b5f243425d853")},
	chunk{2651975, 0x000ae6c868000000, parseDigest("181347d2bbec32bef77ad5e9001e6af80f6abcf3576549384d334ee00c1988d8")},
	chunk{237392, 0x0000000000000001, parseDigest("fcd567f5d866357a8e299fd5b2359bb2c8157c30395229c4e9b0a353944a7978")},
}

// test if nullbytes are correctly split, even if length is a multiple of MinSize.
var chunks2 = []chunk{
	chunk{chunker.MinSize, 0, parseDigest("07854d2fef297a06ba81685e660c332de36d5d18d546927d30daad6d7fda1541")},
	chunk{chunker.MinSize, 0, parseDigest("07854d2fef297a06ba81685e660c332de36d5d18d546927d30daad6d7fda1541")},
	chunk{chunker.MinSize, 0, parseDigest("07854d2fef297a06ba81685e660c332de36d5d18d546927d30daad6d7fda1541")},
	chunk{chunker.MinSize, 0, parseDigest("07854d2fef297a06ba81685e660c332de36d5d18d546927d30daad6d7fda1541")},
}

func testWithData(t *testing.T, chnker *chunker.Chunker, testChunks []chunk) []*chunker.Chunk {
	chunks := []*chunker.Chunk{}

	pos := uint(0)
	for i, chunk := range testChunks {
		c, err := chnker.Next()

		if err != nil {
			t.Fatalf("Error returned with chunk %d: %v", i, err)
		}

		if c == nil {
			t.Fatalf("Nil chunk returned")
		}

		if c != nil {
			if c.Start != pos {
				t.Fatalf("Start for chunk %d does not match: expected %d, got %d",
					i, pos, c.Start)
			}

			if c.Length != chunk.Length {
				t.Fatalf("Length for chunk %d does not match: expected %d, got %d",
					i, chunk.Length, c.Length)
			}

			if c.Cut != chunk.CutFP {
				t.Fatalf("Cut fingerprint for chunk %d/%d does not match: expected %016x, got %016x",
					i, len(chunks)-1, chunk.CutFP, c.Cut)
			}

			if c.Digest != nil && !bytes.Equal(c.Digest, chunk.Digest) {
				t.Fatalf("Digest fingerprint for chunk %d/%d does not match: expected %02x, got %02x",
					i, len(chunks)-1, chunk.Digest, c.Digest)
			}

			pos += c.Length
			chunks = append(chunks, c)
		}
	}

	c, err := chnker.Next()

	if c != nil {
		t.Fatal("additional non-nil chunk returned")
	}

	if err != io.EOF {
		t.Fatal("wrong error returned after last chunk")
	}

	return chunks
}

func getRandom(seed, count int) []byte {
	buf := make([]byte, count)

	rnd := rand.New(rand.NewSource(23))
	for i := 0; i < count; i += 4 {
		r := rnd.Uint32()
		buf[i] = byte(r)
		buf[i+1] = byte(r >> 8)
		buf[i+2] = byte(r >> 16)
		buf[i+3] = byte(r >> 24)
	}

	return buf
}

func TestChunker(t *testing.T) {
	// setup data source
	buf := getRandom(23, 32*1024*1024)
	ch := chunker.New(bytes.NewReader(buf), testPol, sha256.New())
	chunks := testWithData(t, ch, chunks1)

	// test reader
	for i, c := range chunks {
		rd := c.Reader(bytes.NewReader(buf))

		h := sha256.New()
		n, err := io.Copy(h, rd)
		if err != nil {
			t.Fatalf("io.Copy(): %v", err)
		}

		if uint(n) != chunks1[i].Length {
			t.Fatalf("reader returned wrong number of bytes: expected %d, got %d",
				chunks1[i].Length, n)
		}

		d := h.Sum(nil)
		if !bytes.Equal(d, chunks1[i].Digest) {
			t.Fatalf("wrong hash returned: expected %02x, got %02x",
				chunks1[i].Digest, d)
		}
	}

	// setup nullbyte data source
	buf = bytes.Repeat([]byte{0}, len(chunks2)*chunker.MinSize)
	ch = chunker.New(bytes.NewReader(buf), testPol, sha256.New())

	testWithData(t, ch, chunks2)
}

func TestChunkerWithRandomPolynomial(t *testing.T) {
	// setup data source
	buf := getRandom(23, 32*1024*1024)

	// generate a new random polynomial
	start := time.Now()
	p, err := chunker.RandomPolynomial()
	OK(t, err)
	t.Logf("generating random polynomial took %v", time.Since(start))

	start = time.Now()
	ch := chunker.New(bytes.NewReader(buf), p, sha256.New())
	t.Logf("creating chunker took %v", time.Since(start))

	// make sure that first chunk is different
	c, err := ch.Next()

	Assert(t, c.Cut != chunks1[0].CutFP,
		"Cut point is the same")
	Assert(t, c.Length != chunks1[0].Length,
		"Length is the same")
	Assert(t, !bytes.Equal(c.Digest, chunks1[0].Digest),
		"Digest is the same")
}

func TestChunkerWithoutHash(t *testing.T) {
	// setup data source
	buf := getRandom(23, 32*1024*1024)

	ch := chunker.New(bytes.NewReader(buf), testPol, nil)
	chunks := testWithData(t, ch, chunks1)

	// test reader
	for i, c := range chunks {
		rd := c.Reader(bytes.NewReader(buf))

		buf2, err := ioutil.ReadAll(rd)
		if err != nil {
			t.Fatalf("io.Copy(): %v", err)
		}

		if uint(len(buf2)) != chunks1[i].Length {
			t.Fatalf("reader returned wrong number of bytes: expected %d, got %d",
				chunks1[i].Length, uint(len(buf2)))
		}

		if uint(len(buf2)) != chunks1[i].Length {
			t.Fatalf("wrong number of bytes returned: expected %02x, got %02x",
				chunks[i].Length, len(buf2))
		}

		if !bytes.Equal(buf[c.Start:c.Start+c.Length], buf2) {
			t.Fatalf("invalid data for chunk returned: expected %02x, got %02x",
				buf[c.Start:c.Start+c.Length], buf2)
		}
	}

	// setup nullbyte data source
	buf = bytes.Repeat([]byte{0}, len(chunks2)*chunker.MinSize)
	ch = chunker.New(bytes.NewReader(buf), testPol, sha256.New())

	testWithData(t, ch, chunks2)
}

func benchmarkChunker(b *testing.B, hash hash.Hash) {
	size := 10 * 1024 * 1024
	rd := bytes.NewReader(getRandom(23, size))

	b.ResetTimer()
	b.SetBytes(int64(size))

	var chunks int
	for i := 0; i < b.N; i++ {
		chunks = 0

		rd.Seek(0, 0)
		ch := chunker.New(rd, testPol, hash)

		for {
			_, err := ch.Next()

			if err == io.EOF {
				break
			}

			if err != nil {
				b.Fatalf("Unexpected error occurred: %v", err)
			}

			chunks++
		}
	}

	b.Logf("%d chunks, average chunk size: %d bytes", chunks, size/chunks)
}

func BenchmarkChunkerWithSHA256(b *testing.B) {
	benchmarkChunker(b, sha256.New())
}

func BenchmarkChunkerWithMD5(b *testing.B) {
	benchmarkChunker(b, md5.New())
}

func BenchmarkChunker(b *testing.B) {
	benchmarkChunker(b, nil)
}

func BenchmarkNewChunker(b *testing.B) {
	p, err := chunker.RandomPolynomial()
	OK(b, err)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		chunker.New(bytes.NewBuffer(nil), p, nil)
	}
}
