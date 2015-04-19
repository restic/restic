package crypto_test

import (
	"bytes"
	"flag"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/chunker"
	"github.com/restic/restic/crypto"
	. "github.com/restic/restic/test"
)

var testLargeCrypto = flag.Bool("test.largecrypto", false, "also test crypto functions with large payloads")

func TestEncryptDecrypt(t *testing.T) {
	k := crypto.NewKey()

	tests := []int{5, 23, 2<<18 + 23, 1 << 20}
	if *testLargeCrypto {
		tests = append(tests, 7<<20+123)
	}

	for _, size := range tests {
		data := make([]byte, size)
		_, err := io.ReadFull(RandomReader(42, size), data)
		OK(t, err)

		ciphertext, err := crypto.Encrypt(k, restic.GetChunkBuf("TestEncryptDecrypt"), data)
		OK(t, err)
		Assert(t, len(ciphertext) == len(data)+crypto.Extension,
			"ciphertext length does not match: want %d, got %d",
			len(data)+crypto.Extension, len(ciphertext))

		plaintext, err := crypto.Decrypt(k, nil, ciphertext)
		OK(t, err)
		Assert(t, len(plaintext) == len(data),
			"plaintext length does not match: want %d, got %d",
			len(data), len(plaintext))

		restic.FreeChunkBuf("TestEncryptDecrypt", ciphertext)

		Equals(t, plaintext, data)
	}
}

func TestSmallBuffer(t *testing.T) {
	k := crypto.NewKey()

	size := 600
	data := make([]byte, size)
	f, err := os.Open("/dev/urandom")
	OK(t, err)

	_, err = io.ReadFull(f, data)
	OK(t, err)

	ciphertext := make([]byte, size/2)
	ciphertext, err = crypto.Encrypt(k, ciphertext, data)
	// this must extend the slice
	Assert(t, cap(ciphertext) > size/2,
		"expected extended slice, but capacity is only %d bytes",
		cap(ciphertext))

	// check for the correct plaintext
	plaintext, err := crypto.Decrypt(k, nil, ciphertext)
	OK(t, err)
	Assert(t, bytes.Equal(plaintext, data),
		"wrong plaintext returned")
}

func TestSameBuffer(t *testing.T) {
	k := crypto.NewKey()

	size := 600
	data := make([]byte, size)
	f, err := os.Open("/dev/urandom")
	OK(t, err)

	_, err = io.ReadFull(f, data)
	OK(t, err)

	ciphertext := make([]byte, 0, size+crypto.Extension)

	ciphertext, err = crypto.Encrypt(k, ciphertext, data)
	OK(t, err)

	// use the same buffer for decryption
	ciphertext, err = crypto.Decrypt(k, ciphertext, ciphertext)
	OK(t, err)
	Assert(t, bytes.Equal(ciphertext, data),
		"wrong plaintext returned")
}

func TestLargeEncrypt(t *testing.T) {
	if !*testLargeCrypto {
		t.SkipNow()
	}

	k := crypto.NewKey()

	for _, size := range []int{chunker.MaxSize, chunker.MaxSize + 1, chunker.MaxSize + 1<<20} {
		data := make([]byte, size)
		f, err := os.Open("/dev/urandom")
		OK(t, err)

		_, err = io.ReadFull(f, data)
		OK(t, err)

		ciphertext, err := crypto.Encrypt(k, make([]byte, size+crypto.Extension), data)
		OK(t, err)

		plaintext, err := crypto.Decrypt(k, []byte{}, ciphertext)
		OK(t, err)

		Equals(t, plaintext, data)
	}
}

func BenchmarkEncryptWriter(b *testing.B) {
	size := 8 << 20 // 8MiB
	rd := RandomReader(23, size)

	k := crypto.NewKey()

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		rd.Seek(0, 0)
		wr := crypto.EncryptTo(k, ioutil.Discard)
		n, err := io.Copy(wr, rd)
		OK(b, err)
		OK(b, wr.Close())
		Assert(b, n == int64(size),
			"not enough bytes writter: want %d, got %d", size, n)
	}
}

func BenchmarkEncrypt(b *testing.B) {
	size := 8 << 20 // 8MiB
	data := make([]byte, size)

	k := crypto.NewKey()
	buf := make([]byte, len(data)+crypto.Extension)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		_, err := crypto.Encrypt(k, buf, data)
		OK(b, err)
	}
}

func BenchmarkDecryptReader(b *testing.B) {
	size := 8 << 20 // 8MiB
	buf := Random(23, size)
	k := crypto.NewKey()

	ciphertext := make([]byte, len(buf)+crypto.Extension)
	_, err := crypto.Encrypt(k, ciphertext, buf)
	OK(b, err)

	rd := bytes.NewReader(ciphertext)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		rd.Seek(0, 0)
		decRd, err := crypto.DecryptFrom(k, rd)
		OK(b, err)

		_, err = io.Copy(ioutil.Discard, decRd)
		OK(b, err)
	}
}

func BenchmarkEncryptDecryptReader(b *testing.B) {
	k := crypto.NewKey()

	size := 8 << 20 // 8MiB
	rd := RandomReader(23, size)

	b.ResetTimer()
	b.SetBytes(int64(size))

	buf := bytes.NewBuffer(nil)
	for i := 0; i < b.N; i++ {
		rd.Seek(0, 0)
		buf.Reset()
		wr := crypto.EncryptTo(k, buf)
		_, err := io.Copy(wr, rd)
		OK(b, err)
		OK(b, wr.Close())

		r, err := crypto.DecryptFrom(k, buf)
		OK(b, err)

		_, err = io.Copy(ioutil.Discard, r)
		OK(b, err)
	}

	restic.PoolAlloc()
}

func BenchmarkDecrypt(b *testing.B) {
	size := 8 << 20 // 8MiB
	data := make([]byte, size)

	k := crypto.NewKey()

	ciphertext := restic.GetChunkBuf("BenchmarkDecrypt")
	defer restic.FreeChunkBuf("BenchmarkDecrypt", ciphertext)
	plaintext := restic.GetChunkBuf("BenchmarkDecrypt")
	defer restic.FreeChunkBuf("BenchmarkDecrypt", plaintext)

	ciphertext, err := crypto.Encrypt(k, ciphertext, data)
	OK(b, err)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		plaintext, err = crypto.Decrypt(k, plaintext, ciphertext)
		OK(b, err)
	}
}

func TestEncryptStreamWriter(t *testing.T) {
	k := crypto.NewKey()

	tests := []int{5, 23, 2<<18 + 23, 1 << 20}
	if *testLargeCrypto {
		tests = append(tests, 7<<20+123)
	}

	for _, size := range tests {
		data := make([]byte, size)
		_, err := io.ReadFull(RandomReader(42, size), data)
		OK(t, err)

		ciphertext := bytes.NewBuffer(nil)
		wr := crypto.EncryptTo(k, ciphertext)

		_, err = io.Copy(wr, bytes.NewReader(data))
		OK(t, err)
		OK(t, wr.Close())

		l := len(data) + crypto.Extension
		Assert(t, len(ciphertext.Bytes()) == l,
			"wrong ciphertext length: expected %d, got %d",
			l, len(ciphertext.Bytes()))

		// decrypt with default function
		plaintext, err := crypto.Decrypt(k, []byte{}, ciphertext.Bytes())
		OK(t, err)
		Assert(t, bytes.Equal(data, plaintext),
			"wrong plaintext after decryption: expected %02x, got %02x",
			data, plaintext)
	}
}

func TestDecryptStreamReader(t *testing.T) {
	k := crypto.NewKey()

	tests := []int{5, 23, 2<<18 + 23, 1 << 20}
	if *testLargeCrypto {
		tests = append(tests, 7<<20+123)
	}

	for _, size := range tests {
		data := make([]byte, size)
		_, err := io.ReadFull(RandomReader(42, size), data)
		OK(t, err)

		ciphertext := make([]byte, size+crypto.Extension)

		// encrypt with default function
		ciphertext, err = crypto.Encrypt(k, ciphertext, data)
		OK(t, err)
		Assert(t, len(ciphertext) == len(data)+crypto.Extension,
			"wrong number of bytes returned after encryption: expected %d, got %d",
			len(data)+crypto.Extension, len(ciphertext))

		rd, err := crypto.DecryptFrom(k, bytes.NewReader(ciphertext))
		OK(t, err)

		plaintext, err := ioutil.ReadAll(rd)
		OK(t, err)

		Assert(t, bytes.Equal(data, plaintext),
			"wrong plaintext after decryption: expected %02x, got %02x",
			data, plaintext)
	}
}

func TestEncryptWriter(t *testing.T) {
	k := crypto.NewKey()

	tests := []int{5, 23, 2<<18 + 23, 1 << 20}
	if *testLargeCrypto {
		tests = append(tests, 7<<20+123)
	}

	for _, size := range tests {
		data := make([]byte, size)
		_, err := io.ReadFull(RandomReader(42, size), data)
		OK(t, err)

		buf := bytes.NewBuffer(nil)
		wr := crypto.EncryptTo(k, buf)

		_, err = io.Copy(wr, bytes.NewReader(data))
		OK(t, err)
		OK(t, wr.Close())

		ciphertext := buf.Bytes()

		l := len(data) + crypto.Extension
		Assert(t, len(ciphertext) == l,
			"wrong ciphertext length: expected %d, got %d",
			l, len(ciphertext))

		// decrypt with default function
		plaintext, err := crypto.Decrypt(k, []byte{}, ciphertext)
		OK(t, err)
		Assert(t, bytes.Equal(data, plaintext),
			"wrong plaintext after decryption: expected %02x, got %02x",
			data, plaintext)
	}
}
