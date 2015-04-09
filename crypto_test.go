package restic_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/chunker"
	. "github.com/restic/restic/test"
)

func TestEncryptDecrypt(t *testing.T) {
	s := setupBackend(t)
	defer teardownBackend(t, s)
	k := setupKey(t, s, testPassword)

	tests := []int{5, 23, 2<<18 + 23, 1 << 20}
	if *testLargeCrypto {
		tests = append(tests, 7<<20+123)
	}

	for _, size := range tests {
		data := make([]byte, size)
		_, err := io.ReadFull(randomReader(42, size), data)
		OK(t, err)

		ciphertext := restic.GetChunkBuf("TestEncryptDecrypt")
		n, err := k.Encrypt(ciphertext, data)
		OK(t, err)

		plaintext, err := k.Decrypt(nil, ciphertext[:n])
		OK(t, err)

		restic.FreeChunkBuf("TestEncryptDecrypt", ciphertext)

		Equals(t, plaintext, data)
	}
}

func TestSmallBuffer(t *testing.T) {
	s := setupBackend(t)
	defer teardownBackend(t, s)
	k := setupKey(t, s, testPassword)

	size := 600
	data := make([]byte, size)
	f, err := os.Open("/dev/urandom")
	OK(t, err)

	_, err = io.ReadFull(f, data)
	OK(t, err)

	ciphertext := make([]byte, size/2)
	_, err = k.Encrypt(ciphertext, data)
	// this must throw an error, since the target slice is too small
	Assert(t, err != nil && err == restic.ErrBufferTooSmall,
		"expected restic.ErrBufferTooSmall, got %#v", err)
}

func TestLargeEncrypt(t *testing.T) {
	if !*testLargeCrypto {
		t.SkipNow()
	}

	s := setupBackend(t)
	defer teardownBackend(t, s)
	k := setupKey(t, s, testPassword)

	for _, size := range []int{chunker.MaxSize, chunker.MaxSize + 1, chunker.MaxSize + 1<<20} {
		data := make([]byte, size)
		f, err := os.Open("/dev/urandom")
		OK(t, err)

		_, err = io.ReadFull(f, data)
		OK(t, err)

		ciphertext := make([]byte, size+restic.CiphertextExtension)
		n, err := k.Encrypt(ciphertext, data)
		OK(t, err)

		plaintext, err := k.Decrypt([]byte{}, ciphertext[:n])
		OK(t, err)

		Equals(t, plaintext, data)
	}
}

func BenchmarkEncryptWriter(b *testing.B) {
	size := 8 << 20 // 8MiB
	rd := randomReader(23, size)

	be := setupBackend(b)
	defer teardownBackend(b, be)
	k := setupKey(b, be, testPassword)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		rd.Seek(0, 0)
		wr := k.EncryptTo(ioutil.Discard)
		_, err := io.Copy(wr, rd)
		OK(b, err)
		OK(b, wr.Close())
	}
}

func BenchmarkEncrypt(b *testing.B) {
	size := 8 << 20 // 8MiB
	data := make([]byte, size)

	be := setupBackend(b)
	defer teardownBackend(b, be)
	k := setupKey(b, be, testPassword)

	buf := make([]byte, len(data)+restic.CiphertextExtension)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		_, err := k.Encrypt(buf, data)
		OK(b, err)
	}
}

func BenchmarkDecryptReader(b *testing.B) {
	be := setupBackend(b)
	defer teardownBackend(b, be)
	k := setupKey(b, be, testPassword)

	size := 8 << 20 // 8MiB
	buf := get_random(23, size)

	ciphertext := make([]byte, len(buf)+restic.CiphertextExtension)
	_, err := k.Encrypt(ciphertext, buf)
	OK(b, err)

	rd := bytes.NewReader(ciphertext)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		rd.Seek(0, 0)
		decRd, err := k.DecryptFrom(rd)
		OK(b, err)

		_, err = io.Copy(ioutil.Discard, decRd)
		OK(b, err)
	}
}

func BenchmarkEncryptDecryptReader(b *testing.B) {
	be := setupBackend(b)
	defer teardownBackend(b, be)
	k := setupKey(b, be, testPassword)

	size := 8 << 20 // 8MiB
	rd := randomReader(23, size)

	b.ResetTimer()
	b.SetBytes(int64(size))

	buf := bytes.NewBuffer(nil)
	for i := 0; i < b.N; i++ {
		rd.Seek(0, 0)
		buf.Reset()
		wr := k.EncryptTo(buf)
		_, err := io.Copy(wr, rd)
		OK(b, err)
		OK(b, wr.Close())

		r, err := k.DecryptFrom(buf)
		OK(b, err)

		_, err = io.Copy(ioutil.Discard, r)
		OK(b, err)
	}

	restic.PoolAlloc()
}

func BenchmarkDecrypt(b *testing.B) {
	size := 8 << 20 // 8MiB
	data := make([]byte, size)

	s := setupBackend(b)
	defer teardownBackend(b, s)
	k := setupKey(b, s, testPassword)

	ciphertext := restic.GetChunkBuf("BenchmarkDecrypt")
	defer restic.FreeChunkBuf("BenchmarkDecrypt", ciphertext)
	plaintext := restic.GetChunkBuf("BenchmarkDecrypt")
	defer restic.FreeChunkBuf("BenchmarkDecrypt", plaintext)

	n, err := k.Encrypt(ciphertext, data)
	OK(b, err)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		plaintext, err = k.Decrypt(plaintext, ciphertext[:n])
		OK(b, err)
	}
}

func TestEncryptStreamWriter(t *testing.T) {
	s := setupBackend(t)
	defer teardownBackend(t, s)
	k := setupKey(t, s, testPassword)

	tests := []int{5, 23, 2<<18 + 23, 1 << 20}
	if *testLargeCrypto {
		tests = append(tests, 7<<20+123)
	}

	for _, size := range tests {
		data := make([]byte, size)
		_, err := io.ReadFull(randomReader(42, size), data)
		OK(t, err)

		ciphertext := bytes.NewBuffer(nil)
		wr := k.EncryptTo(ciphertext)

		_, err = io.Copy(wr, bytes.NewReader(data))
		OK(t, err)
		OK(t, wr.Close())

		l := len(data) + restic.CiphertextExtension
		Assert(t, len(ciphertext.Bytes()) == l,
			"wrong ciphertext length: expected %d, got %d",
			l, len(ciphertext.Bytes()))

		// decrypt with default function
		plaintext, err := k.Decrypt([]byte{}, ciphertext.Bytes())
		OK(t, err)
		Assert(t, bytes.Equal(data, plaintext),
			"wrong plaintext after decryption: expected %02x, got %02x",
			data, plaintext)
	}
}

func TestDecryptStreamReader(t *testing.T) {
	s := setupBackend(t)
	defer teardownBackend(t, s)
	k := setupKey(t, s, testPassword)

	tests := []int{5, 23, 2<<18 + 23, 1 << 20}
	if *testLargeCrypto {
		tests = append(tests, 7<<20+123)
	}

	for _, size := range tests {
		data := make([]byte, size)
		_, err := io.ReadFull(randomReader(42, size), data)
		OK(t, err)

		ciphertext := make([]byte, size+restic.CiphertextExtension)

		// encrypt with default function
		n, err := k.Encrypt(ciphertext, data)
		OK(t, err)
		Assert(t, n == len(data)+restic.CiphertextExtension,
			"wrong number of bytes returned after encryption: expected %d, got %d",
			len(data)+restic.CiphertextExtension, n)

		rd, err := k.DecryptFrom(bytes.NewReader(ciphertext))
		OK(t, err)

		plaintext, err := ioutil.ReadAll(rd)
		OK(t, err)

		Assert(t, bytes.Equal(data, plaintext),
			"wrong plaintext after decryption: expected %02x, got %02x",
			data, plaintext)
	}
}

func TestEncryptWriter(t *testing.T) {
	s := setupBackend(t)
	defer teardownBackend(t, s)
	k := setupKey(t, s, testPassword)

	tests := []int{5, 23, 2<<18 + 23, 1 << 20}
	if *testLargeCrypto {
		tests = append(tests, 7<<20+123)
	}

	for _, size := range tests {
		data := make([]byte, size)
		_, err := io.ReadFull(randomReader(42, size), data)
		OK(t, err)

		buf := bytes.NewBuffer(nil)
		wr := k.EncryptTo(buf)

		_, err = io.Copy(wr, bytes.NewReader(data))
		OK(t, err)
		OK(t, wr.Close())

		ciphertext := buf.Bytes()

		l := len(data) + restic.CiphertextExtension
		Assert(t, len(ciphertext) == l,
			"wrong ciphertext length: expected %d, got %d",
			l, len(ciphertext))

		// decrypt with default function
		plaintext, err := k.Decrypt([]byte{}, ciphertext)
		OK(t, err)
		Assert(t, bytes.Equal(data, plaintext),
			"wrong plaintext after decryption: expected %02x, got %02x",
			data, plaintext)
	}
}
