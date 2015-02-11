package restic_test

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/chunker"
)

var testPassword = "foobar"
var testCleanup = flag.Bool("test.cleanup", true, "clean up after running tests (remove local backend directory with all content)")
var testLargeCrypto = flag.Bool("test.largecrypto", false, "also test crypto functions with large payloads")

func setupBackend(t testing.TB) restic.Server {
	tempdir, err := ioutil.TempDir("", "restic-test-")
	ok(t, err)

	b, err := backend.CreateLocal(tempdir)
	ok(t, err)

	return restic.NewServer(b)
}

func teardownBackend(t testing.TB, s restic.Server) {
	if !*testCleanup {
		l := s.Backend().(*backend.Local)
		t.Logf("leaving local backend at %s\n", l.Location())
		return
	}

	ok(t, s.Delete())
}

func setupKey(t testing.TB, s restic.Server, password string) *restic.Key {
	k, err := restic.CreateKey(s, password)
	ok(t, err)

	return k
}

func TestRepo(t *testing.T) {
	s := setupBackend(t)
	defer teardownBackend(t, s)
	_ = setupKey(t, s, testPassword)
}

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
		ok(t, err)

		ciphertext := restic.GetChunkBuf("TestEncryptDecrypt")
		n, err := k.Encrypt(ciphertext, data)
		ok(t, err)

		plaintext, err := k.Decrypt(ciphertext[:n])
		ok(t, err)

		restic.FreeChunkBuf("TestEncryptDecrypt", ciphertext)

		equals(t, plaintext, data)
	}
}

func TestSmallBuffer(t *testing.T) {
	s := setupBackend(t)
	defer teardownBackend(t, s)
	k := setupKey(t, s, testPassword)

	size := 600
	data := make([]byte, size)
	f, err := os.Open("/dev/urandom")
	ok(t, err)

	_, err = io.ReadFull(f, data)
	ok(t, err)

	ciphertext := make([]byte, size/2)
	_, err = k.Encrypt(ciphertext, data)
	// this must throw an error, since the target slice is too small
	assert(t, err != nil && err == restic.ErrBufferTooSmall,
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
		ok(t, err)

		_, err = io.ReadFull(f, data)
		ok(t, err)

		ciphertext := make([]byte, size+restic.CiphertextExtension)
		n, err := k.Encrypt(ciphertext, data)
		ok(t, err)

		plaintext, err := k.Decrypt(ciphertext[:n])
		ok(t, err)

		equals(t, plaintext, data)
	}
}

func BenchmarkEncryptReader(b *testing.B) {
	size := 8 << 20 // 8MiB
	rd := randomReader(23, size)

	be := setupBackend(b)
	defer teardownBackend(b, be)
	k := setupKey(b, be, testPassword)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		rd.Seek(0, 0)
		_, err := io.Copy(ioutil.Discard, k.EncryptFrom(rd))
		ok(b, err)
	}
}

func BenchmarkEncrypt(b *testing.B) {
	size := 8 << 20 // 8MiB
	data := make([]byte, size)

	be := setupBackend(b)
	defer teardownBackend(b, be)
	k := setupKey(b, be, testPassword)

	b.ResetTimer()
	b.SetBytes(int64(size))

	buf := restic.GetChunkBuf("BenchmarkEncrypt")
	for i := 0; i < b.N; i++ {
		_, err := k.Encrypt(buf, data)
		ok(b, err)
	}
	restic.FreeChunkBuf("BenchmarkEncrypt", buf)
}

func BenchmarkDecryptReader(b *testing.B) {
	be := setupBackend(b)
	defer teardownBackend(b, be)
	k := setupKey(b, be, testPassword)

	size := 8 << 20 // 8MiB
	buf := get_random(23, size)

	ciphertext := make([]byte, len(buf)+restic.CiphertextExtension)
	_, err := k.Encrypt(ciphertext, buf)
	ok(b, err)

	rd := bytes.NewReader(ciphertext)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		rd.Seek(0, 0)
		decRd, err := k.DecryptFrom(rd)
		ok(b, err)

		_, err = io.Copy(ioutil.Discard, decRd)
		ok(b, err)
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

	for i := 0; i < b.N; i++ {
		rd.Seek(0, 0)
		decRd, err := k.DecryptFrom(k.EncryptFrom(rd))
		ok(b, err)

		_, err = io.Copy(ioutil.Discard, decRd)
		ok(b, err)
	}
}

func BenchmarkDecrypt(b *testing.B) {
	size := 8 << 20 // 8MiB
	data := make([]byte, size)

	s := setupBackend(b)
	defer teardownBackend(b, s)
	k := setupKey(b, s, testPassword)

	ciphertext := restic.GetChunkBuf("BenchmarkDecrypt")
	n, err := k.Encrypt(ciphertext, data)
	ok(b, err)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		_, err := k.Decrypt(ciphertext[:n])
		ok(b, err)
	}
	restic.FreeChunkBuf("BenchmarkDecrypt", ciphertext)
}

func TestHashReader(t *testing.T) {
	tests := []int{5, 23, 2<<18 + 23, 1 << 20}
	if *testLargeCrypto {
		tests = append(tests, 7<<20+123)
	}

	for _, size := range tests {
		data := make([]byte, size)
		_, err := io.ReadFull(randomReader(42, size), data)
		ok(t, err)

		expectedHash := sha256.Sum256(data)

		rd := restic.NewHashReader(bytes.NewReader(data), sha256.New())

		target := bytes.NewBuffer(nil)
		n, err := io.Copy(target, rd)
		ok(t, err)

		assert(t, n == int64(size)+int64(len(expectedHash)),
			"HashReader: invalid number of bytes read: got %d, expected %d",
			n, size+len(expectedHash))

		r := target.Bytes()
		resultingHash := r[len(r)-len(expectedHash):]
		assert(t, bytes.Equal(expectedHash[:], resultingHash),
			"HashReader: hashes do not match: expected %02x, got %02x",
			expectedHash, resultingHash)

		// try to read again, must return io.EOF
		n2, err := rd.Read(make([]byte, 100))
		assert(t, n2 == 0, "HashReader returned %d additional bytes", n)
		assert(t, err == io.EOF, "HashReader returned %v instead of EOF", err)
	}
}

func TestEncryptStreamReader(t *testing.T) {
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
		ok(t, err)

		erd := k.EncryptFrom(bytes.NewReader(data))

		ciphertext, err := ioutil.ReadAll(erd)
		ok(t, err)

		l := len(data) + restic.CiphertextExtension
		assert(t, len(ciphertext) == l,
			"wrong ciphertext length: expected %d, got %d",
			l, len(ciphertext))

		// decrypt with default function
		plaintext, err := k.Decrypt(ciphertext)
		ok(t, err)
		assert(t, bytes.Equal(data, plaintext),
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
		ok(t, err)

		ciphertext := make([]byte, size+restic.CiphertextExtension)

		// encrypt with default function
		n, err := k.Encrypt(ciphertext, data)
		ok(t, err)
		assert(t, n == len(data)+restic.CiphertextExtension,
			"wrong number of bytes returned after encryption: expected %d, got %d",
			len(data)+restic.CiphertextExtension, n)

		rd, err := k.DecryptFrom(bytes.NewReader(ciphertext))
		ok(t, err)

		plaintext, err := ioutil.ReadAll(rd)
		ok(t, err)

		assert(t, bytes.Equal(data, plaintext),
			"wrong plaintext after decryption: expected %02x, got %02x",
			data, plaintext)
	}
}
