package repository

import (
	"math/rand"
	"sort"
	"strings"
	"testing"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

type mapcache map[restic.Handle]bool

func (c mapcache) Has(h restic.Handle) bool { return c[h] }

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
			h := restic.Handle{Name: id.String(), Type: restic.PackFile}
			cache[h] = true
		}
	}

	copy(sorted[:], blobs[:])
	sort.SliceStable(sorted[:], func(i, j int) bool {
		hi := restic.Handle{Type: restic.PackFile, Name: sorted[i].PackID.String()}
		hj := restic.Handle{Type: restic.PackFile, Name: sorted[j].PackID.String()}
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
			h := restic.Handle{Name: id.String(), Type: restic.PackFile}
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

func TestBlobVerification(t *testing.T) {
	repo := TestRepository(t).(*Repository)

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
	repo := TestRepository(t).(*Repository)

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
