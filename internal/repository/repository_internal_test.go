package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/go-cmp/cmp"
	"github.com/klauspost/compress/zstd"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

type mapcache map[backend.Handle]bool

func (c mapcache) Has(h backend.Handle) bool { return c[h] }

func TestSortCachedPacksFirst(t *testing.T) {
	var (
		blobs, sorted [100]restic.PackedBlob

		cache = make(mapcache)
		r     = rand.New(rand.NewSource(1261))
	)

	for i := 0; i < len(blobs); i++ {
		var id restic.ID
		r.Read(id[:])
		blobs[i] = restic.PackedBlob{PackID: id}

		if i%3 == 0 {
			h := backend.Handle{Name: id.String(), Type: backend.PackFile}
			cache[h] = true
		}
	}

	copy(sorted[:], blobs[:])
	sort.SliceStable(sorted[:], func(i, j int) bool {
		hi := backend.Handle{Type: backend.PackFile, Name: sorted[i].PackID.String()}
		hj := backend.Handle{Type: backend.PackFile, Name: sorted[j].PackID.String()}
		return cache.Has(hi) && !cache.Has(hj)
	})

	sortCachedPacksFirst(cache, blobs[:])
	rtest.Equals(t, sorted, blobs)
}

func BenchmarkSortCachedPacksFirst(b *testing.B) {
	const nblobs = 512 // Corresponds to a file of ca. 2GB.

	var (
		blobs [nblobs]restic.PackedBlob
		cache = make(mapcache)
		r     = rand.New(rand.NewSource(1261))
	)

	for i := 0; i < nblobs; i++ {
		var id restic.ID
		r.Read(id[:])
		blobs[i] = restic.PackedBlob{PackID: id}

		if i%3 == 0 {
			h := backend.Handle{Name: id.String(), Type: backend.PackFile}
			cache[h] = true
		}
	}

	var cpy [nblobs]restic.PackedBlob
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		copy(cpy[:], blobs[:])
		sortCachedPacksFirst(cache, cpy[:])
	}
}

// buildPackfileWithoutHeader returns a manually built pack file without a header.
func buildPackfileWithoutHeader(blobSizes []int, key *crypto.Key, compress bool) (blobs []restic.Blob, packfile []byte) {
	opts := []zstd.EOption{
		// Set the compression level configured.
		zstd.WithEncoderLevel(zstd.SpeedDefault),
		// Disable CRC, we have enough checks in place, makes the
		// compressed data four bytes shorter.
		zstd.WithEncoderCRC(false),
		// Set a window of 512kbyte, so we have good lookbehind for usual
		// blob sizes.
		zstd.WithWindowSize(512 * 1024),
	}
	enc, err := zstd.NewWriter(nil, opts...)
	if err != nil {
		panic(err)
	}

	var offset uint
	for i, size := range blobSizes {
		plaintext := rtest.Random(800+i, size)
		id := restic.Hash(plaintext)
		uncompressedLength := uint(0)
		if compress {
			uncompressedLength = uint(len(plaintext))
			plaintext = enc.EncodeAll(plaintext, nil)
		}

		// we use a deterministic nonce here so the whole process is
		// deterministic, last byte is the blob index
		var nonce = []byte{
			0x15, 0x98, 0xc0, 0xf7, 0xb9, 0x65, 0x97, 0x74,
			0x12, 0xdc, 0xd3, 0x62, 0xa9, 0x6e, 0x20, byte(i),
		}

		before := len(packfile)
		packfile = append(packfile, nonce...)
		packfile = key.Seal(packfile, nonce, plaintext, nil)
		after := len(packfile)

		ciphertextLength := after - before

		blobs = append(blobs, restic.Blob{
			BlobHandle: restic.BlobHandle{
				Type: restic.DataBlob,
				ID:   id,
			},
			Length:             uint(ciphertextLength),
			UncompressedLength: uncompressedLength,
			Offset:             offset,
		})

		offset = uint(len(packfile))
	}

	return blobs, packfile
}

func TestStreamPack(t *testing.T) {
	TestAllVersions(t, testStreamPack)
}

func testStreamPack(t *testing.T, version uint) {
	dec, err := zstd.NewReader(nil)
	if err != nil {
		panic(dec)
	}
	defer dec.Close()

	// always use the same key for deterministic output
	key := testKey(t)

	blobSizes := []int{
		5522811,
		10,
		5231,
		18812,
		123123,
		13522811,
		12301,
		892242,
		28616,
		13351,
		252287,
		188883,
		3522811,
		18883,
	}

	var compress bool
	switch version {
	case 1:
		compress = false
	case 2:
		compress = true
	default:
		t.Fatal("test does not support repository version", version)
	}

	packfileBlobs, packfile := buildPackfileWithoutHeader(blobSizes, &key, compress)

	loadCalls := 0
	shortFirstLoad := false

	loadBytes := func(length int, offset int64) []byte {
		data := packfile

		if offset > int64(len(data)) {
			offset = 0
			length = 0
		}
		data = data[offset:]

		if length > len(data) {
			length = len(data)
		}
		if shortFirstLoad {
			length /= 2
			shortFirstLoad = false
		}

		return data[:length]
	}

	load := func(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
		data := loadBytes(length, offset)
		if shortFirstLoad {
			data = data[:len(data)/2]
			shortFirstLoad = false
		}

		loadCalls++

		err := fn(bytes.NewReader(data))
		if err == nil {
			return nil
		}
		var permanent *backoff.PermanentError
		if errors.As(err, &permanent) {
			return err
		}

		// retry loading once
		return fn(bytes.NewReader(loadBytes(length, offset)))
	}

	// first, test regular usage
	t.Run("regular", func(t *testing.T) {
		tests := []struct {
			blobs          []restic.Blob
			calls          int
			shortFirstLoad bool
		}{
			{packfileBlobs[1:2], 1, false},
			{packfileBlobs[2:5], 1, false},
			{packfileBlobs[2:8], 1, false},
			{[]restic.Blob{
				packfileBlobs[0],
				packfileBlobs[4],
				packfileBlobs[2],
			}, 1, false},
			{[]restic.Blob{
				packfileBlobs[0],
				packfileBlobs[len(packfileBlobs)-1],
			}, 2, false},
			{packfileBlobs[:], 1, true},
		}

		for _, test := range tests {
			t.Run("", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				gotBlobs := make(map[restic.ID]int)

				handleBlob := func(blob restic.BlobHandle, buf []byte, err error) error {
					gotBlobs[blob.ID]++

					id := restic.Hash(buf)
					if !id.Equal(blob.ID) {
						t.Fatalf("wrong id %v for blob %s returned", id, blob.ID)
					}

					return err
				}

				wantBlobs := make(map[restic.ID]int)
				for _, blob := range test.blobs {
					wantBlobs[blob.ID] = 1
				}

				loadCalls = 0
				shortFirstLoad = test.shortFirstLoad
				err := streamPack(ctx, load, nil, dec, &key, restic.ID{}, test.blobs, handleBlob)
				if err != nil {
					t.Fatal(err)
				}

				if !cmp.Equal(wantBlobs, gotBlobs) {
					t.Fatal(cmp.Diff(wantBlobs, gotBlobs))
				}
				rtest.Equals(t, test.calls, loadCalls)
			})
		}
	})
	shortFirstLoad = false

	// next, test invalid uses, which should return an error
	t.Run("invalid", func(t *testing.T) {
		tests := []struct {
			blobs []restic.Blob
			err   string
		}{
			{
				// pass one blob several times
				blobs: []restic.Blob{
					packfileBlobs[3],
					packfileBlobs[8],
					packfileBlobs[3],
					packfileBlobs[4],
				},
				err: "overlapping blobs in pack",
			},

			{
				// pass something that's not a valid blob in the current pack file
				blobs: []restic.Blob{
					{
						Offset: 123,
						Length: 20000,
					},
				},
				err: "ciphertext verification failed",
			},

			{
				// pass a blob that's too small
				blobs: []restic.Blob{
					{
						Offset: 123,
						Length: 10,
					},
				},
				err: "invalid blob length",
			},
		}

		for _, test := range tests {
			t.Run("", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				handleBlob := func(blob restic.BlobHandle, buf []byte, err error) error {
					return err
				}

				err := streamPack(ctx, load, nil, dec, &key, restic.ID{}, test.blobs, handleBlob)
				if err == nil {
					t.Fatalf("wanted error %v, got nil", test.err)
				}

				if !strings.Contains(err.Error(), test.err) {
					t.Fatalf("wrong error returned, it should contain %q but was %q", test.err, err)
				}
			})
		}
	})
}

func TestBlobVerification(t *testing.T) {
	repo := TestRepository(t)

	type DamageType string
	const (
		damageData       DamageType = "data"
		damageCompressed DamageType = "compressed"
		damageCiphertext DamageType = "ciphertext"
	)

	for _, test := range []struct {
		damage DamageType
		msg    string
	}{
		{"", ""},
		{damageData, "hash mismatch"},
		{damageCompressed, "decompression failed"},
		{damageCiphertext, "ciphertext verification failed"},
	} {
		plaintext := rtest.Random(800, 1234)
		id := restic.Hash(plaintext)
		if test.damage == damageData {
			plaintext[42] ^= 0x42
		}

		uncompressedLength := uint(len(plaintext))
		plaintext = repo.getZstdEncoder().EncodeAll(plaintext, nil)

		if test.damage == damageCompressed {
			plaintext = plaintext[:len(plaintext)-8]
		}

		nonce := crypto.NewRandomNonce()
		ciphertext := append([]byte{}, nonce...)
		ciphertext = repo.Key().Seal(ciphertext, nonce, plaintext, nil)

		if test.damage == damageCiphertext {
			ciphertext[42] ^= 0x42
		}

		err := repo.verifyCiphertext(ciphertext, int(uncompressedLength), id)
		if test.msg == "" {
			rtest.Assert(t, err == nil, "expected no error, got %v", err)
		} else {
			rtest.Assert(t, strings.Contains(err.Error(), test.msg), "expected error to contain %q, got %q", test.msg, err)
		}
	}
}

func TestUnpackedVerification(t *testing.T) {
	repo := TestRepository(t)

	type DamageType string
	const (
		damageData       DamageType = "data"
		damageCompressed DamageType = "compressed"
		damageCiphertext DamageType = "ciphertext"
	)

	for _, test := range []struct {
		damage DamageType
		msg    string
	}{
		{"", ""},
		{damageData, "data mismatch"},
		{damageCompressed, "decompression failed"},
		{damageCiphertext, "ciphertext verification failed"},
	} {
		plaintext := rtest.Random(800, 1234)
		orig := append([]byte{}, plaintext...)
		if test.damage == damageData {
			plaintext[42] ^= 0x42
		}

		compressed := []byte{2}
		compressed = repo.getZstdEncoder().EncodeAll(plaintext, compressed)

		if test.damage == damageCompressed {
			compressed = compressed[:len(compressed)-8]
		}

		nonce := crypto.NewRandomNonce()
		ciphertext := append([]byte{}, nonce...)
		ciphertext = repo.Key().Seal(ciphertext, nonce, compressed, nil)

		if test.damage == damageCiphertext {
			ciphertext[42] ^= 0x42
		}

		err := repo.verifyUnpacked(ciphertext, restic.IndexFile, orig)
		if test.msg == "" {
			rtest.Assert(t, err == nil, "expected no error, got %v", err)
		} else {
			rtest.Assert(t, strings.Contains(err.Error(), test.msg), "expected error to contain %q, got %q", test.msg, err)
		}
	}
}

func testKey(t *testing.T) crypto.Key {
	const jsonKey = `{"mac":{"k":"eQenuI8adktfzZMuC8rwdA==","r":"k8cfAly2qQSky48CQK7SBA=="},"encrypt":"MKO9gZnRiQFl8mDUurSDa9NMjiu9MUifUrODTHS05wo="}`

	var key crypto.Key
	err := json.Unmarshal([]byte(jsonKey), &key)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestStreamPackFallback(t *testing.T) {
	dec, err := zstd.NewReader(nil)
	if err != nil {
		panic(dec)
	}
	defer dec.Close()

	test := func(t *testing.T, failLoad bool) {
		key := testKey(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		plaintext := rtest.Random(800, 42)
		blobID := restic.Hash(plaintext)
		blobs := []restic.Blob{
			{
				Length: uint(crypto.CiphertextLength(len(plaintext))),
				Offset: 0,
				BlobHandle: restic.BlobHandle{
					ID:   blobID,
					Type: restic.DataBlob,
				},
			},
		}

		var loadPack backendLoadFn
		if failLoad {
			loadPack = func(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
				return errors.New("load error")
			}
		} else {
			loadPack = func(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
				// just return an empty array to provoke an error
				data := make([]byte, length)
				return fn(bytes.NewReader(data))
			}
		}

		loadBlob := func(ctx context.Context, t restic.BlobType, id restic.ID, buf []byte) ([]byte, error) {
			if id == blobID {
				return plaintext, nil
			}
			return nil, errors.New("unknown blob")
		}

		blobOK := false
		handleBlob := func(blob restic.BlobHandle, buf []byte, err error) error {
			rtest.OK(t, err)
			rtest.Equals(t, blobID, blob.ID)
			rtest.Equals(t, plaintext, buf)
			blobOK = true
			return err
		}

		err := streamPack(ctx, loadPack, loadBlob, dec, &key, restic.ID{}, blobs, handleBlob)
		rtest.OK(t, err)
		rtest.Assert(t, blobOK, "blob failed to load")
	}

	t.Run("corrupted blob", func(t *testing.T) {
		test(t, false)
	})

	// test fallback for failed pack loading
	t.Run("failed load", func(t *testing.T) {
		test(t, true)
	})
}
