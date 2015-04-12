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

		ciphertext := restic.GetChunkBuf("TestEncryptDecrypt")
		n, err := crypto.Encrypt(k, ciphertext, data)
		OK(t, err)

		plaintext, err := crypto.Decrypt(k, nil, ciphertext[:n])
		OK(t, err)

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
	_, err = crypto.Encrypt(k, ciphertext, data)
	// this must throw an error, since the target slice is too small
	Assert(t, err != nil && err == crypto.ErrBufferTooSmall,
		"expected restic.ErrBufferTooSmall, got %#v", err)
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

		ciphertext := make([]byte, size+crypto.Extension)
		n, err := crypto.Encrypt(k, ciphertext, data)
		OK(t, err)

		plaintext, err := crypto.Decrypt(k, []byte{}, ciphertext[:n])
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
		_, err := io.Copy(wr, rd)
		OK(b, err)
		OK(b, wr.Close())
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

	n, err := crypto.Encrypt(k, ciphertext, data)
	OK(b, err)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		plaintext, err = crypto.Decrypt(k, plaintext, ciphertext[:n])
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
		n, err := crypto.Encrypt(k, ciphertext, data)
		OK(t, err)
		Assert(t, n == len(data)+crypto.Extension,
			"wrong number of bytes returned after encryption: expected %d, got %d",
			len(data)+crypto.Extension, n)

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
