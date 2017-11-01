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
		buf := make([]byte, 0, size+crypto.Extension)

		nonce := crypto.NewRandomNonce()
		ciphertext := k.Seal(buf[:0], nonce, data, nil)
		rtest.Assert(t, len(ciphertext) == len(data)+k.Overhead(),
			"ciphertext length does not match: want %d, got %d",
			len(data)+crypto.Extension, len(ciphertext))

		plaintext := make([]byte, 0, len(ciphertext))
		plaintext, err := k.Open(plaintext[:0], nonce, ciphertext, nil)
		rtest.OK(t, err)
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

	ciphertext := make([]byte, 0, size/2)
	nonce := crypto.NewRandomNonce()
	ciphertext = k.Seal(ciphertext[:0], nonce, data, nil)
	// this must extend the slice
	rtest.Assert(t, cap(ciphertext) > size/2,
		"expected extended slice, but capacity is only %d bytes",
		cap(ciphertext))

	// check for the correct plaintext
	plaintext := make([]byte, len(ciphertext))
	plaintext, err = k.Open(plaintext[:0], nonce, ciphertext, nil)
	rtest.OK(t, err)
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

	nonce := crypto.NewRandomNonce()
	ciphertext = k.Seal(ciphertext, nonce, data, nil)

	// use the same buffer for decryption
	ciphertext, err = k.Open(ciphertext[:0], nonce, ciphertext, nil)
	rtest.OK(t, err)
	rtest.Assert(t, bytes.Equal(ciphertext, data),
		"wrong plaintext returned")
}

func encrypt(t testing.TB, k *crypto.Key, data, ciphertext, nonce []byte) []byte {
	prefixlen := len(ciphertext)
	ciphertext = k.Seal(ciphertext, nonce, data, nil)
	if len(ciphertext) != len(data)+k.Overhead()+prefixlen {
		t.Fatalf("destination slice has wrong length, want %d, got %d",
			len(data)+k.Overhead(), len(ciphertext))
	}

	return ciphertext
}

func decryptNewSliceAndCompare(t testing.TB, k *crypto.Key, data, ciphertext, nonce []byte) {
	plaintext := make([]byte, 0, len(ciphertext))
	decryptAndCompare(t, k, data, ciphertext, nonce, plaintext)
}

func decryptAndCompare(t testing.TB, k *crypto.Key, data, ciphertext, nonce, dst []byte) {
	prefix := make([]byte, len(dst))
	copy(prefix, dst)

	plaintext, err := k.Open(dst, nonce, ciphertext, nil)
	if err != nil {
		t.Fatalf("unable to decrypt ciphertext: %v", err)
	}

	if len(data)+len(prefix) != len(plaintext) {
		t.Fatalf("wrong plaintext returned, want %d bytes, got %d", len(data)+len(prefix), len(plaintext))
	}

	if !bytes.Equal(plaintext[:len(prefix)], prefix) {
		t.Fatal("prefix is wrong")
	}

	if !bytes.Equal(plaintext[len(prefix):], data) {
		t.Fatal("wrong plaintext returned")
	}
}

func TestAppendOpen(t *testing.T) {
	k := crypto.NewRandomKey()
	nonce := crypto.NewRandomNonce()

	data := make([]byte, 600)
	_, err := io.ReadFull(rand.Reader, data)
	rtest.OK(t, err)
	ciphertext := encrypt(t, k, data, nil, nonce)

	// we need to test several different cases:
	//  * destination slice is nil
	//  * destination slice is empty and has enough capacity
	//  * destination slice is empty and does not have enough capacity
	//  * destination slice contains data and has enough capacity
	//  * destination slice contains data and does not have enough capacity

	// destination slice is nil
	t.Run("nil", func(t *testing.T) {
		var plaintext []byte
		decryptAndCompare(t, k, data, ciphertext, nonce, plaintext)
	})

	// destination slice is empty and has enough capacity
	t.Run("empty-large", func(t *testing.T) {
		plaintext := make([]byte, 0, len(data)+100)
		decryptAndCompare(t, k, data, ciphertext, nonce, plaintext)
	})

	// destination slice is empty and does not have enough capacity
	t.Run("empty-small", func(t *testing.T) {
		plaintext := make([]byte, 0, len(data)/2)
		decryptAndCompare(t, k, data, ciphertext, nonce, plaintext)
	})

	// destination slice contains data and has enough capacity
	t.Run("prefix-large", func(t *testing.T) {
		plaintext := make([]byte, 0, len(data)+100)
		plaintext = append(plaintext, []byte("foobar")...)
		decryptAndCompare(t, k, data, ciphertext, nonce, plaintext)
	})

	// destination slice contains data and does not have enough capacity
	t.Run("prefix-small", func(t *testing.T) {
		plaintext := make([]byte, 0, len(data)/2)
		plaintext = append(plaintext, []byte("foobar")...)
		decryptAndCompare(t, k, data, ciphertext, nonce, plaintext)
	})
}

func TestAppendSeal(t *testing.T) {
	k := crypto.NewRandomKey()

	data := make([]byte, 600)
	_, err := io.ReadFull(rand.Reader, data)
	rtest.OK(t, err)

	// we need to test several different cases:
	//  * destination slice is nil
	//  * destination slice is empty and has enough capacity
	//  * destination slice is empty and does not have enough capacity
	//  * destination slice contains data and has enough capacity
	//  * destination slice contains data and does not have enough capacity

	// destination slice is nil
	t.Run("nil", func(t *testing.T) {
		nonce := crypto.NewRandomNonce()
		var ciphertext []byte

		ciphertext = encrypt(t, k, data, ciphertext, nonce)
		decryptNewSliceAndCompare(t, k, data, ciphertext, nonce)
	})

	// destination slice is empty and has enough capacity
	t.Run("empty-large", func(t *testing.T) {
		nonce := crypto.NewRandomNonce()
		ciphertext := make([]byte, 0, len(data)+100)

		ciphertext = encrypt(t, k, data, ciphertext, nonce)
		decryptNewSliceAndCompare(t, k, data, ciphertext, nonce)
	})

	// destination slice is empty and does not have enough capacity
	t.Run("empty-small", func(t *testing.T) {
		nonce := crypto.NewRandomNonce()
		ciphertext := make([]byte, 0, len(data)/2)

		ciphertext = encrypt(t, k, data, ciphertext, nonce)
		decryptNewSliceAndCompare(t, k, data, ciphertext, nonce)
	})

	// destination slice contains data and has enough capacity
	t.Run("prefix-large", func(t *testing.T) {
		nonce := crypto.NewRandomNonce()
		ciphertext := make([]byte, 0, len(data)+100)
		ciphertext = append(ciphertext, []byte("foobar")...)

		ciphertext = encrypt(t, k, data, ciphertext, nonce)
		if string(ciphertext[:6]) != "foobar" {
			t.Errorf("prefix is missing")
		}
		decryptNewSliceAndCompare(t, k, data, ciphertext[6:], nonce)
	})

	// destination slice contains data and does not have enough capacity
	t.Run("prefix-small", func(t *testing.T) {
		nonce := crypto.NewRandomNonce()
		ciphertext := make([]byte, 0, len(data)/2)
		ciphertext = append(ciphertext, []byte("foobar")...)

		ciphertext = encrypt(t, k, data, ciphertext, nonce)
		if string(ciphertext[:6]) != "foobar" {
			t.Errorf("prefix is missing")
		}
		decryptNewSliceAndCompare(t, k, data, ciphertext[6:], nonce)
	})
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

		nonce := crypto.NewRandomNonce()
		ciphertext := k.Seal(make([]byte, size+k.Overhead()), nonce, data, nil)
		plaintext, err := k.Open([]byte{}, nonce, ciphertext, nil)
		rtest.OK(t, err)

		rtest.Equals(t, plaintext, data)
	}
}

func BenchmarkEncrypt(b *testing.B) {
	size := 8 << 20 // 8MiB
	data := make([]byte, size)

	k := crypto.NewRandomKey()
	buf := make([]byte, len(data)+crypto.Extension)
	nonce := crypto.NewRandomNonce()

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		_ = k.Seal(buf, nonce, data, nil)
	}
}

func BenchmarkDecrypt(b *testing.B) {
	size := 8 << 20 // 8MiB
	data := make([]byte, size)

	k := crypto.NewRandomKey()

	plaintext := make([]byte, 0, size)
	ciphertext := make([]byte, 0, size+crypto.Extension)
	nonce := crypto.NewRandomNonce()
	ciphertext = k.Seal(ciphertext, nonce, data, nil)

	var err error

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		_, err = k.Open(plaintext, nonce, ciphertext, nil)
		rtest.OK(b, err)
	}
}
