package restic_test

import (
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
		f, err := os.Open("/dev/urandom")
		ok(t, err)

		_, err = io.ReadFull(f, data)
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
