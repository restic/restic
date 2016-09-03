package crypto_test

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"restic/crypto"
	. "restic/test"

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
		data := Random(42, size)
		buf := make([]byte, size+crypto.Extension)

		ciphertext, err := crypto.Encrypt(k, buf, data)
		OK(t, err)
		Assert(t, len(ciphertext) == len(data)+crypto.Extension,
			"ciphertext length does not match: want %d, got %d",
			len(data)+crypto.Extension, len(ciphertext))

		plaintext := make([]byte, len(ciphertext))
		n, err := crypto.Decrypt(k, plaintext, ciphertext)
		OK(t, err)
		plaintext = plaintext[:n]
		Assert(t, len(plaintext) == len(data),
			"plaintext length does not match: want %d, got %d",
			len(data), len(plaintext))

		Equals(t, plaintext, data)
	}
}

func TestSmallBuffer(t *testing.T) {
	k := crypto.NewRandomKey()

	size := 600
	data := make([]byte, size)
	_, err := io.ReadFull(rand.Reader, data)
	OK(t, err)

	ciphertext := make([]byte, size/2)
	ciphertext, err = crypto.Encrypt(k, ciphertext, data)
	// this must extend the slice
	Assert(t, cap(ciphertext) > size/2,
		"expected extended slice, but capacity is only %d bytes",
		cap(ciphertext))

	// check for the correct plaintext
	plaintext := make([]byte, len(ciphertext))
	n, err := crypto.Decrypt(k, plaintext, ciphertext)
	OK(t, err)
	plaintext = plaintext[:n]
	Assert(t, bytes.Equal(plaintext, data),
		"wrong plaintext returned")
}

func TestSameBuffer(t *testing.T) {
	k := crypto.NewRandomKey()

	size := 600
	data := make([]byte, size)
	_, err := io.ReadFull(rand.Reader, data)
	OK(t, err)

	ciphertext := make([]byte, 0, size+crypto.Extension)

	ciphertext, err = crypto.Encrypt(k, ciphertext, data)
	OK(t, err)

	// use the same buffer for decryption
	n, err := crypto.Decrypt(k, ciphertext, ciphertext)
	OK(t, err)
	ciphertext = ciphertext[:n]
	Assert(t, bytes.Equal(ciphertext, data),
		"wrong plaintext returned")
}

func TestCornerCases(t *testing.T) {
	k := crypto.NewRandomKey()

	// nil plaintext should encrypt to the empty string
	// nil ciphertext should allocate a new slice for the ciphertext
	c, err := crypto.Encrypt(k, nil, nil)
	OK(t, err)

	Assert(t, len(c) == crypto.Extension,
		"wrong length returned for ciphertext, expected 0, got %d",
		len(c))

	// this should decrypt to nil
	n, err := crypto.Decrypt(k, nil, c)
	OK(t, err)
	Equals(t, 0, n)

	// test encryption for same slice, this should return an error
	_, err = crypto.Encrypt(k, c, c)
	Equals(t, crypto.ErrInvalidCiphertext, err)
}

func TestLargeEncrypt(t *testing.T) {
	if !testLargeCrypto {
		t.SkipNow()
	}

	k := crypto.NewRandomKey()

	for _, size := range []int{chunker.MaxSize, chunker.MaxSize + 1, chunker.MaxSize + 1<<20} {
		data := make([]byte, size)
		_, err := io.ReadFull(rand.Reader, data)
		OK(t, err)

		ciphertext, err := crypto.Encrypt(k, make([]byte, size+crypto.Extension), data)
		OK(t, err)

		plaintext, err := crypto.Decrypt(k, []byte{}, ciphertext)
		OK(t, err)

		Equals(t, plaintext, data)
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
		_, err := crypto.Encrypt(k, buf, data)
		OK(b, err)
	}
}

func BenchmarkDecrypt(b *testing.B) {
	size := 8 << 20 // 8MiB
	data := make([]byte, size)

	k := crypto.NewRandomKey()

	plaintext := make([]byte, size)
	ciphertext := make([]byte, size+crypto.Extension)

	ciphertext, err := crypto.Encrypt(k, ciphertext, data)
	OK(b, err)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		_, err = crypto.Decrypt(k, plaintext, ciphertext)
		OK(b, err)
	}
}
