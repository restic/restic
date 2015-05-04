package sftp

// sftp integration tests
// enable with -integration

import (
	"crypto/sha1"
	"flag"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/kr/fs"
)

const (
	READONLY  = true
	READWRITE = false

	debuglevel = "ERROR" // set to "DEBUG" for debugging
)

var testIntegration = flag.Bool("integration", false, "perform integration tests against sftp server process")
var testSftp = flag.String("sftp", "/usr/lib/openssh/sftp-server", "location of the sftp server binary")

// testClient returns a *Client connected to a localy running sftp-server
// the *exec.Cmd returned must be defer Wait'd.
func testClient(t testing.TB, readonly bool) (*Client, *exec.Cmd) {
	if !*testIntegration {
		t.Skip("skipping intergration test")
	}
	cmd := exec.Command(*testSftp, "-e", "-R", "-l", debuglevel) // log to stderr, read only
	if !readonly {
		cmd = exec.Command(*testSftp, "-e", "-l", debuglevel) // log to stderr
	}
	cmd.Stderr = os.Stdout
	pw, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	pr, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Skipf("could not start sftp-server process: %v", err)
	}

	sftp, err := NewClientPipe(pr, pw)
	if err != nil {
		t.Fatal(err)
	}

	if err := sftp.sendInit(); err != nil {
		defer cmd.Wait()
		t.Fatal(err)
	}
	if err := sftp.recvVersion(); err != nil {
		defer cmd.Wait()
		t.Fatal(err)
	}
	return sftp, cmd
}

func TestNewClient(t *testing.T) {
	sftp, cmd := testClient(t, READONLY)
	defer cmd.Wait()

	if err := sftp.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClientLstat(t *testing.T) {
	sftp, cmd := testClient(t, READONLY)
	defer cmd.Wait()
	defer sftp.Close()

	f, err := ioutil.TempFile("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	want, err := os.Lstat(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	got, err := sftp.Lstat(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	if !sameFile(want, got) {
		t.Fatalf("Lstat(%q): want %#v, got %#v", f.Name(), want, got)
	}
}

func TestClientLstatMissing(t *testing.T) {
	sftp, cmd := testClient(t, READONLY)
	defer cmd.Wait()
	defer sftp.Close()

	f, err := ioutil.TempFile("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(f.Name())

	_, err = sftp.Lstat(f.Name())
	if err1, ok := err.(*StatusError); !ok || err1.Code != ssh_FX_NO_SUCH_FILE {
		t.Fatalf("Lstat: want: %v, got %#v", ssh_FX_NO_SUCH_FILE, err)
	}
}

func TestClientMkdir(t *testing.T) {
	sftp, cmd := testClient(t, READWRITE)
	defer cmd.Wait()
	defer sftp.Close()

	dir, err := ioutil.TempDir("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	sub := path.Join(dir, "mkdir1")
	if err := sftp.Mkdir(sub); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(sub); err != nil {
		t.Fatal(err)
	}
}

func TestClientOpen(t *testing.T) {
	sftp, cmd := testClient(t, READONLY)
	defer cmd.Wait()
	defer sftp.Close()

	f, err := ioutil.TempFile("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	got, err := sftp.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if err := got.Close(); err != nil {
		t.Fatal(err)
	}
}

const seekBytes = 128 * 1024

type seek struct {
	offset int64
}

func (s seek) Generate(r *rand.Rand, _ int) reflect.Value {
	s.offset = int64(r.Int31n(seekBytes))
	return reflect.ValueOf(s)
}

func (s seek) set(t *testing.T, r io.ReadSeeker) {
	if _, err := r.Seek(s.offset, os.SEEK_SET); err != nil {
		t.Fatalf("error while seeking with %+v: %v", s, err)
	}
}

func (s seek) current(t *testing.T, r io.ReadSeeker) {
	const mid = seekBytes / 2

	skip := s.offset / 2
	if s.offset > mid {
		skip = -skip
	}

	if _, err := r.Seek(mid, os.SEEK_SET); err != nil {
		t.Fatalf("error seeking to midpoint with %+v: %v", s, err)
	}
	if _, err := r.Seek(skip, os.SEEK_CUR); err != nil {
		t.Fatalf("error seeking from %d with %+v: %v", mid, s, err)
	}
}

func (s seek) end(t *testing.T, r io.ReadSeeker) {
	if _, err := r.Seek(-s.offset, os.SEEK_END); err != nil {
		t.Fatalf("error seeking from end with %+v: %v", s, err)
	}
}

func TestClientSeek(t *testing.T) {
	sftp, cmd := testClient(t, READONLY)
	defer cmd.Wait()
	defer sftp.Close()

	fOS, err := ioutil.TempFile("", "seek-test")
	if err != nil {
		t.Fatal(err)
	}
	defer fOS.Close()

	fSFTP, err := sftp.Open(fOS.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer fSFTP.Close()

	writeN(t, fOS, seekBytes)

	if err := quick.CheckEqual(
		func(s seek) (string, int64) { s.set(t, fOS); return readHash(t, fOS) },
		func(s seek) (string, int64) { s.set(t, fSFTP); return readHash(t, fSFTP) },
		nil,
	); err != nil {
		t.Errorf("Seek: expected equal absolute seeks: %v", err)
	}

	if err := quick.CheckEqual(
		func(s seek) (string, int64) { s.current(t, fOS); return readHash(t, fOS) },
		func(s seek) (string, int64) { s.current(t, fSFTP); return readHash(t, fSFTP) },
		nil,
	); err != nil {
		t.Errorf("Seek: expected equal seeks from middle: %v", err)
	}

	if err := quick.CheckEqual(
		func(s seek) (string, int64) { s.end(t, fOS); return readHash(t, fOS) },
		func(s seek) (string, int64) { s.end(t, fSFTP); return readHash(t, fSFTP) },
		nil,
	); err != nil {
		t.Errorf("Seek: expected equal seeks from end: %v", err)
	}
}

func TestClientCreate(t *testing.T) {
	sftp, cmd := testClient(t, READWRITE)
	defer cmd.Wait()
	defer sftp.Close()

	f, err := ioutil.TempFile("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	defer os.Remove(f.Name())

	f2, err := sftp.Create(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
}

func TestClientAppend(t *testing.T) {
	sftp, cmd := testClient(t, READWRITE)
	defer cmd.Wait()
	defer sftp.Close()

	f, err := ioutil.TempFile("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	defer os.Remove(f.Name())

	f2, err := sftp.OpenFile(f.Name(), os.O_RDWR|os.O_APPEND)
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
}

func TestClientCreateFailed(t *testing.T) {
	sftp, cmd := testClient(t, READONLY)
	defer cmd.Wait()
	defer sftp.Close()

	f, err := ioutil.TempFile("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	defer os.Remove(f.Name())

	f2, err := sftp.Create(f.Name())
	if err1, ok := err.(*StatusError); !ok || err1.Code != ssh_FX_PERMISSION_DENIED {
		t.Fatalf("Create: want: %v, got %#v", ssh_FX_PERMISSION_DENIED, err)
	}
	if err == nil {
		f2.Close()
	}
}

func TestClientFileStat(t *testing.T) {
	sftp, cmd := testClient(t, READONLY)
	defer cmd.Wait()
	defer sftp.Close()

	f, err := ioutil.TempFile("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	want, err := os.Lstat(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	f2, err := sftp.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	got, err := f2.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if !sameFile(want, got) {
		t.Fatalf("Lstat(%q): want %#v, got %#v", f.Name(), want, got)
	}
}

func TestClientRemove(t *testing.T) {
	sftp, cmd := testClient(t, READWRITE)
	defer cmd.Wait()
	defer sftp.Close()

	f, err := ioutil.TempFile("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	if err := sftp.Remove(f.Name()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(f.Name()); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestClientRemoveDir(t *testing.T) {
	sftp, cmd := testClient(t, READWRITE)
	defer cmd.Wait()
	defer sftp.Close()

	dir, err := ioutil.TempDir("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	if err := sftp.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestClientRemoveFailed(t *testing.T) {
	sftp, cmd := testClient(t, READONLY)
	defer cmd.Wait()
	defer sftp.Close()

	f, err := ioutil.TempFile("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	if err := sftp.Remove(f.Name()); err == nil {
		t.Fatalf("Remove(%v): want: permission denied, got %v", f.Name(), err)
	}
	if _, err := os.Lstat(f.Name()); err != nil {
		t.Fatal(err)
	}
}

func TestClientRename(t *testing.T) {
	sftp, cmd := testClient(t, READWRITE)
	defer cmd.Wait()
	defer sftp.Close()

	f, err := ioutil.TempFile("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	f2 := f.Name() + ".new"
	if err := sftp.Rename(f.Name(), f2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(f.Name()); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if _, err := os.Lstat(f2); err != nil {
		t.Fatal(err)
	}
}

func TestClientReadLine(t *testing.T) {
	sftp, cmd := testClient(t, READWRITE)
	defer cmd.Wait()
	defer sftp.Close()

	f, err := ioutil.TempFile("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	f2 := f.Name() + ".sym"
	if err := os.Symlink(f.Name(), f2); err != nil {
		t.Fatal(err)
	}
	if _, err := sftp.ReadLink(f2); err != nil {
		t.Fatal(err)
	}
}

func sameFile(want, got os.FileInfo) bool {
	return want.Name() == got.Name() &&
		want.Size() == got.Size()
}

var clientReadTests = []struct {
	n int64
}{
	{0},
	{1},
	{1000},
	{1024},
	{1025},
	{2048},
	{4096},
	{1 << 12},
	{1 << 13},
	{1 << 14},
	{1 << 15},
	{1 << 16},
	{1 << 17},
	{1 << 18},
	{1 << 19},
	{1 << 20},
}

func TestClientRead(t *testing.T) {
	sftp, cmd := testClient(t, READONLY)
	defer cmd.Wait()
	defer sftp.Close()

	d, err := ioutil.TempDir("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(d)

	for _, tt := range clientReadTests {
		f, err := ioutil.TempFile(d, "read-test")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		hash := writeN(t, f, tt.n)
		f2, err := sftp.Open(f.Name())
		if err != nil {
			t.Fatal(err)
		}
		defer f2.Close()
		hash2, n := readHash(t, f2)
		if hash != hash2 || tt.n != n {
			t.Errorf("Read: hash: want: %q, got %q, read: want: %v, got %v", hash, hash2, tt.n, n)
		}
	}
}

// readHash reads r until EOF returning the number of bytes read
// and the hash of the contents.
func readHash(t *testing.T, r io.Reader) (string, int64) {
	h := sha1.New()
	tr := io.TeeReader(r, h)
	read, err := io.Copy(ioutil.Discard, tr)
	if err != nil {
		t.Fatal(err)
	}
	return string(h.Sum(nil)), read
}

// writeN writes n bytes of random data to w and returns the
// hash of that data.
func writeN(t *testing.T, w io.Writer, n int64) string {
	rand, err := os.Open("/dev/urandom")
	if err != nil {
		t.Fatal(err)
	}
	defer rand.Close()

	h := sha1.New()

	mw := io.MultiWriter(w, h)

	written, err := io.CopyN(mw, rand, n)
	if err != nil {
		t.Fatal(err)
	}
	if written != n {
		t.Fatalf("CopyN(%v): wrote: %v", n, written)
	}
	return string(h.Sum(nil))
}

var clientWriteTests = []struct {
	n     int
	total int64 // cumulative file size
}{
	{0, 0},
	{1, 1},
	{0, 1},
	{999, 1000},
	{24, 1024},
	{1023, 2047},
	{2048, 4095},
	{1 << 12, 8191},
	{1 << 13, 16383},
	{1 << 14, 32767},
	{1 << 15, 65535},
	{1 << 16, 131071},
	{1 << 17, 262143},
	{1 << 18, 524287},
	{1 << 19, 1048575},
	{1 << 20, 2097151},
	{1 << 21, 4194303},
}

func TestClientWrite(t *testing.T) {
	sftp, cmd := testClient(t, READWRITE)
	defer cmd.Wait()
	defer sftp.Close()

	d, err := ioutil.TempDir("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(d)

	f := path.Join(d, "writeTest")
	w, err := sftp.Create(f)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	for _, tt := range clientWriteTests {
		got, err := w.Write(make([]byte, tt.n))
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.n {
			t.Errorf("Write(%v): wrote: want: %v, got %v", tt.n, tt.n, got)
		}
		fi, err := os.Stat(f)
		if err != nil {
			t.Fatal(err)
		}
		if total := fi.Size(); total != tt.total {
			t.Errorf("Write(%v): size: want: %v, got %v", tt.n, tt.total, total)
		}
	}
}

// taken from github.com/kr/fs/walk_test.go

type PathTest struct {
	path, result string
}

type Node struct {
	name    string
	entries []*Node // nil if the entry is a file
	mark    int
}

var tree = &Node{
	"testdata",
	[]*Node{
		{"a", nil, 0},
		{"b", []*Node{}, 0},
		{"c", nil, 0},
		{
			"d",
			[]*Node{
				{"x", nil, 0},
				{"y", []*Node{}, 0},
				{
					"z",
					[]*Node{
						{"u", nil, 0},
						{"v", nil, 0},
					},
					0,
				},
			},
			0,
		},
	},
	0,
}

func walkTree(n *Node, path string, f func(path string, n *Node)) {
	f(path, n)
	for _, e := range n.entries {
		walkTree(e, filepath.Join(path, e.name), f)
	}
}

func makeTree(t *testing.T) {
	walkTree(tree, tree.name, func(path string, n *Node) {
		if n.entries == nil {
			fd, err := os.Create(path)
			if err != nil {
				t.Errorf("makeTree: %v", err)
				return
			}
			fd.Close()
		} else {
			os.Mkdir(path, 0770)
		}
	})
}

func markTree(n *Node) { walkTree(n, "", func(path string, n *Node) { n.mark++ }) }

func checkMarks(t *testing.T, report bool) {
	walkTree(tree, tree.name, func(path string, n *Node) {
		if n.mark != 1 && report {
			t.Errorf("node %s mark = %d; expected 1", path, n.mark)
		}
		n.mark = 0
	})
}

// Assumes that each node name is unique. Good enough for a test.
// If clear is true, any incoming error is cleared before return. The errors
// are always accumulated, though.
func mark(path string, info os.FileInfo, err error, errors *[]error, clear bool) error {
	if err != nil {
		*errors = append(*errors, err)
		if clear {
			return nil
		}
		return err
	}
	name := info.Name()
	walkTree(tree, tree.name, func(path string, n *Node) {
		if n.name == name {
			n.mark++
		}
	})
	return nil
}

func TestClientWalk(t *testing.T) {
	sftp, cmd := testClient(t, READONLY)
	defer cmd.Wait()
	defer sftp.Close()

	makeTree(t)
	errors := make([]error, 0, 10)
	clear := true
	markFn := func(walker *fs.Walker) (err error) {
		for walker.Step() {
			err = mark(walker.Path(), walker.Stat(), walker.Err(), &errors, clear)
			if err != nil {
				break
			}
		}
		return err
	}
	// Expect no errors.
	err := markFn(sftp.Walk(tree.name))
	if err != nil {
		t.Fatalf("no error expected, found: %s", err)
	}
	if len(errors) != 0 {
		t.Fatalf("unexpected errors: %s", errors)
	}
	checkMarks(t, true)
	errors = errors[0:0]

	// Test permission errors.  Only possible if we're not root
	// and only on some file systems (AFS, FAT).  To avoid errors during
	// all.bash on those file systems, skip during go test -short.
	if os.Getuid() > 0 && !testing.Short() {
		// introduce 2 errors: chmod top-level directories to 0
		os.Chmod(filepath.Join(tree.name, tree.entries[1].name), 0)
		os.Chmod(filepath.Join(tree.name, tree.entries[3].name), 0)

		// 3) capture errors, expect two.
		// mark respective subtrees manually
		markTree(tree.entries[1])
		markTree(tree.entries[3])
		// correct double-marking of directory itself
		tree.entries[1].mark--
		tree.entries[3].mark--
		err := markFn(sftp.Walk(tree.name))
		if err != nil {
			t.Fatalf("expected no error return from Walk, got %s", err)
		}
		if len(errors) != 2 {
			t.Errorf("expected 2 errors, got %d: %s", len(errors), errors)
		}
		// the inaccessible subtrees were marked manually
		checkMarks(t, true)
		errors = errors[0:0]

		// 4) capture errors, stop after first error.
		// mark respective subtrees manually
		markTree(tree.entries[1])
		markTree(tree.entries[3])
		// correct double-marking of directory itself
		tree.entries[1].mark--
		tree.entries[3].mark--
		clear = false // error will stop processing
		err = markFn(sftp.Walk(tree.name))
		if err == nil {
			t.Fatalf("expected error return from Walk")
		}
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d: %s", len(errors), errors)
		}
		// the inaccessible subtrees were marked manually
		checkMarks(t, false)
		errors = errors[0:0]

		// restore permissions
		os.Chmod(filepath.Join(tree.name, tree.entries[1].name), 0770)
		os.Chmod(filepath.Join(tree.name, tree.entries[3].name), 0770)
	}

	// cleanup
	if err := os.RemoveAll(tree.name); err != nil {
		t.Errorf("removeTree: %v", err)
	}
}

func benchmarkRead(b *testing.B, bufsize int) {
	size := 10*1024*1024 + 123 // ~10MiB

	// open sftp client
	sftp, cmd := testClient(b, READONLY)
	defer cmd.Wait()
	defer sftp.Close()

	buf := make([]byte, bufsize)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		offset := 0

		f2, err := sftp.Open("/dev/zero")
		if err != nil {
			b.Fatal(err)
		}
		defer f2.Close()

		for offset < size {
			n, err := io.ReadFull(f2, buf)
			offset += n
			if err == io.ErrUnexpectedEOF && offset != size {
				b.Fatalf("read too few bytes! want: %d, got: %d", size, n)
			}

			if err != nil {
				b.Fatal(err)
			}

			offset += n
		}
	}
}

func BenchmarkRead1k(b *testing.B) {
	benchmarkRead(b, 1*1024)
}

func BenchmarkRead16k(b *testing.B) {
	benchmarkRead(b, 16*1024)
}

func BenchmarkRead32k(b *testing.B) {
	benchmarkRead(b, 32*1024)
}

func BenchmarkRead128k(b *testing.B) {
	benchmarkRead(b, 128*1024)
}

func BenchmarkRead512k(b *testing.B) {
	benchmarkRead(b, 512*1024)
}

func BenchmarkRead1MiB(b *testing.B) {
	benchmarkRead(b, 1024*1024)
}

func BenchmarkRead4MiB(b *testing.B) {
	benchmarkRead(b, 4*1024*1024)
}

func benchmarkWrite(b *testing.B, bufsize int) {
	size := 10*1024*1024 + 123 // ~10MiB

	// open sftp client
	sftp, cmd := testClient(b, false)
	defer cmd.Wait()
	defer sftp.Close()

	data := make([]byte, size)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		offset := 0

		f, err := ioutil.TempFile("", "sftptest")
		if err != nil {
			b.Fatal(err)
		}
		defer os.Remove(f.Name())

		f2, err := sftp.Create(f.Name())
		if err != nil {
			b.Fatal(err)
		}
		defer f2.Close()

		for offset < size {
			n, err := f2.Write(data[offset:min(len(data), offset+bufsize)])
			if err != nil {
				b.Fatal(err)
			}

			if offset+n < size && n != bufsize {
				b.Fatalf("wrote too few bytes! want: %d, got: %d", size, n)
			}

			offset += n
		}

		f2.Close()

		fi, err := os.Stat(f.Name())
		if err != nil {
			b.Fatal(err)
		}

		if fi.Size() != int64(size) {
			b.Fatalf("wrong file size: want %d, got %d", size, fi.Size())
		}

		os.Remove(f.Name())
	}
}

func BenchmarkWrite1k(b *testing.B) {
	benchmarkWrite(b, 1*1024)
}

func BenchmarkWrite16k(b *testing.B) {
	benchmarkWrite(b, 16*1024)
}

func BenchmarkWrite32k(b *testing.B) {
	benchmarkWrite(b, 32*1024)
}

func BenchmarkWrite128k(b *testing.B) {
	benchmarkWrite(b, 128*1024)
}

func BenchmarkWrite512k(b *testing.B) {
	benchmarkWrite(b, 512*1024)
}

func BenchmarkWrite1MiB(b *testing.B) {
	benchmarkWrite(b, 1024*1024)
}

func BenchmarkWrite4MiB(b *testing.B) {
	benchmarkWrite(b, 4*1024*1024)
}
