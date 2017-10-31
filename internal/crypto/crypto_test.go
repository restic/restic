package crypto_test

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"github.com/restic/restic/internal/crypto"
	rtest "github.com/restic/restic/internal/test"

	"github.com/restic/chunker"
)

const testLargeCrypto = false

func TestEncryptDecrypt(t *testing.T) {
	k := crypto.NewRandomKey()

	tests := []int{5, 23, 2<<18 + 23, 1 << 20}
	if testLargeCrypto {
		tests = append(tests, 7<<20+123)
	}

	for _, size := range tests {
		data := rtest.Random(42, size)
		buf := make([]byte, size+crypto.Extension)

		ciphertext, err := k.Encrypt(buf, data)
		rtest.OK(t, err)
		rtest.Assert(t, len(ciphertext) == len(data)+crypto.Extension,
			"ciphertext length does not match: want %d, got %d",
			len(data)+crypto.Extension, len(ciphertext))

		plaintext := make([]byte, len(ciphertext))
		n, err := k.Decrypt(plaintext, ciphertext)
		rtest.OK(t, err)
		plaintext = plaintext[:n]
		rtest.Assert(t, len(plaintext) == len(data),
			"plaintext length does not match: want %d, got %d",
			len(data), len(plaintext))

		rtest.Equals(t, plaintext, data)
	}
}

func TestSmallBuffer(t *testing.T) {
	k := crypto.NewRandomKey()

	size := 600
	data := make([]byte, size)
	_, err := io.ReadFull(rand.Reader, data)
	rtest.OK(t, err)

	ciphertext := make([]byte, size/2)
	ciphertext, err = k.Encrypt(ciphertext, data)
	// this must extend the slice
	rtest.Assert(t, cap(ciphertext) > size/2,
		"expected extended slice, but capacity is only %d bytes",
		cap(ciphertext))

	// check for the correct plaintext
	plaintext := make([]byte, len(ciphertext))
	n, err := k.Decrypt(plaintext, ciphertext)
	rtest.OK(t, err)
	plaintext = plaintext[:n]
	rtest.Assert(t, bytes.Equal(plaintext, data),
		"wrong plaintext returned")
}

func TestSameBuffer(t *testing.T) {
	k := crypto.NewRandomKey()

	size := 600
	data := make([]byte, size)
	_, err := io.ReadFull(rand.Reader, data)
	rtest.OK(t, err)

	ciphertext := make([]byte, 0, size+crypto.Extension)

	ciphertext, err = k.Encrypt(ciphertext, data)
	rtest.OK(t, err)

	// use the same buffer for decryption
	n, err := k.Decrypt(ciphertext, ciphertext)
	rtest.OK(t, err)
	ciphertext = ciphertext[:n]
	rtest.Assert(t, bytes.Equal(ciphertext, data),
		"wrong plaintext returned")
}

func TestCornerCases(t *testing.T) {
	k := crypto.NewRandomKey()

	// nil plaintext should encrypt to the empty string
	// nil ciphertext should allocate a new slice for the ciphertext
	c, err := k.Encrypt(nil, nil)
	rtest.OK(t, err)

	rtest.Assert(t, len(c) == crypto.Extension,
		"wrong length returned for ciphertext, expected 0, got %d",
		len(c))

	// this should decrypt to nil
	n, err := k.Decrypt(nil, c)
	rtest.OK(t, err)
	rtest.Equals(t, 0, n)

	// test encryption for same slice, this should return an error
	_, err = k.Encrypt(c, c)
	rtest.Equals(t, crypto.ErrInvalidCiphertext, err)
}

func TestLargeEncrypt(t *testing.T) {
	if !testLargeCrypto {
		t.SkipNow()
	}

	k := crypto.NewRandomKey()

	for _, size := range []int{chunker.MaxSize, chunker.MaxSize + 1, chunker.MaxSize + 1<<20} {
		data := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, data)
		rtest.OK(t, err)

		ciphertext, err := k.Encrypt(make([]byte, size+crypto.Extension), data)
		rtest.OK(t, err)

		plaintext, err := k.Decrypt([]byte{}, ciphertext)
		rtest.OK(t, err)

		rtest.Equals(t, plaintext, data)
	}
}

func BenchmarkEncrypt(b *testing.B) {
	size := 8 << 20 // 8MiB
	data := make([]byte, size)

	k := crypto.NewRandomKey()
	buf := make([]byte, len(data)+crypto.Extension)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		_, err := k.Encrypt(buf, data)
		rtest.OK(b, err)
	}
}

func BenchmarkDecrypt(b *testing.B) {
	size := 8 << 20 // 8MiB
	data := make([]byte, size)

	k := crypto.NewRandomKey()

	plaintext := make([]byte, size)
	ciphertext := make([]byte, size+crypto.Extension)

	ciphertext, err := k.Encrypt(ciphertext, data)
	rtest.OK(b, err)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		_, err = k.Decrypt(plaintext, ciphertext)
		rtest.OK(b, err)
	}
}
