package test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/minio/sha256-simd"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/test"

	"github.com/restic/restic/internal/backend"
)

func seedRand(t testing.TB) {
	seed := time.Now().UnixNano()
	rand.Seed(seed)
	t.Logf("rand initialized with seed %d", seed)
}

func beTest(ctx context.Context, be restic.Backend, h restic.Handle) (bool, error) {
	_, err := be.Stat(ctx, h)
	if err != nil && be.IsNotExist(err) {
		return false, nil
	}

	return err == nil, err
}

// TestCreateWithConfig tests that creating a backend in a location which already
// has a config file fails.
func (s *Suite) TestCreateWithConfig(t *testing.T) {
	b := s.open(t)
	defer s.close(t, b)

	// remove a config if present
	cfgHandle := restic.Handle{Type: restic.ConfigFile}
	cfgPresent, err := beTest(context.TODO(), b, cfgHandle)
	if err != nil {
		t.Fatalf("unable to test for config: %+v", err)
	}

	if cfgPresent {
		remove(t, b, cfgHandle)
	}

	// save a config
	store(t, b, restic.ConfigFile, []byte("test config"))

	// now create the backend again, this must fail
	_, err = s.Create(s.Config)
	if err == nil {
		t.Fatalf("expected error not found for creating a backend with an existing config file")
	}

	// remove config
	err = b.Remove(context.TODO(), restic.Handle{Type: restic.ConfigFile, Name: ""})
	if err != nil {
		t.Fatalf("unexpected error removing config: %+v", err)
	}
}

// TestLocation tests that a location string is returned.
func (s *Suite) TestLocation(t *testing.T) {
	b := s.open(t)
	defer s.close(t, b)

	l := b.Location()
	if l == "" {
		t.Fatalf("invalid location string %q", l)
	}
}

// TestConfig saves and loads a config from the backend.
func (s *Suite) TestConfig(t *testing.T) {
	b := s.open(t)
	defer s.close(t, b)

	var testString = "Config"

	// create config and read it back
	_, err := backend.LoadAll(context.TODO(), nil, b, restic.Handle{Type: restic.ConfigFile})
	if err == nil {
		t.Fatalf("did not get expected error for non-existing config")
	}
	test.Assert(t, b.IsNotExist(err), "IsNotExist() did not recognize error from LoadAll(): %v", err)

	err = b.Save(context.TODO(), restic.Handle{Type: restic.ConfigFile}, restic.NewByteReader([]byte(testString), b.Hasher()))
	if err != nil {
		t.Fatalf("Save() error: %+v", err)
	}

	// try accessing the config with different names, should all return the
	// same config
	for _, name := range []string{"", "foo", "bar", "0000000000000000000000000000000000000000000000000000000000000000"} {
		h := restic.Handle{Type: restic.ConfigFile, Name: name}
		buf, err := backend.LoadAll(context.TODO(), nil, b, h)
		if err != nil {
			t.Fatalf("unable to read config with name %q: %+v", name, err)
		}

		if string(buf) != testString {
			t.Fatalf("wrong data returned, want %q, got %q", testString, string(buf))
		}
	}

	// remove the config
	remove(t, b, restic.Handle{Type: restic.ConfigFile})
}

// TestLoad tests the backend's Load function.
func (s *Suite) TestLoad(t *testing.T) {
	seedRand(t)

	b := s.open(t)
	defer s.close(t, b)

	noop := func(rd io.Reader) error {
		return nil
	}

	err := b.Load(context.TODO(), restic.Handle{}, 0, 0, noop)
	if err == nil {
		t.Fatalf("Load() did not return an error for invalid handle")
	}
	test.Assert(t, !b.IsNotExist(err), "IsNotExist() should not accept an invalid handle error: %v", err)

	err = testLoad(b, restic.Handle{Type: restic.PackFile, Name: "foobar"}, 0, 0)
	if err == nil {
		t.Fatalf("Load() did not return an error for non-existing blob")
	}
	test.Assert(t, b.IsNotExist(err), "IsNotExist() did not recognize non-existing blob: %v", err)

	length := rand.Intn(1<<24) + 2000

	data := test.Random(23, length)
	id := restic.Hash(data)

	handle := restic.Handle{Type: restic.PackFile, Name: id.String()}
	err = b.Save(context.TODO(), handle, restic.NewByteReader(data, b.Hasher()))
	if err != nil {
		t.Fatalf("Save() error: %+v", err)
	}

	t.Logf("saved %d bytes as %v", length, handle)

	err = b.Load(context.TODO(), handle, 100, -1, noop)
	if err == nil {
		t.Fatalf("Load() returned no error for negative offset!")
	}

	err = b.Load(context.TODO(), handle, 0, 0, func(rd io.Reader) error {
		_, err := io.Copy(io.Discard, rd)
		if err != nil {
			t.Fatal(err)
		}
		return errors.Errorf("deliberate error")
	})
	if err == nil {
		t.Fatalf("Load() did not propagate consumer error!")
	}
	if err.Error() != "deliberate error" {
		t.Fatalf("Load() did not correctly propagate consumer error!")
	}

	loadTests := 50
	if s.MinimalData {
		loadTests = 10
	}

	for i := 0; i < loadTests; i++ {
		l := rand.Intn(length + 2000)
		o := rand.Intn(length + 2000)

		d := data
		if o < len(d) {
			d = d[o:]
		} else {
			t.Logf("offset == length, skipping test")
			continue
		}

		getlen := l
		if l >= len(d) && rand.Float32() >= 0.5 {
			getlen = 0
		}

		if l > 0 && l < len(d) {
			d = d[:l]
		}

		var buf []byte
		err := b.Load(context.TODO(), handle, getlen, int64(o), func(rd io.Reader) (ierr error) {
			buf, ierr = io.ReadAll(rd)
			return ierr
		})
		if err != nil {
			t.Logf("Load, l %v, o %v, len(d) %v, getlen %v", l, o, len(d), getlen)
			t.Errorf("Load(%d, %d) returned unexpected error: %+v", l, o, err)
			continue
		}

		if l == 0 && len(buf) != len(d) {
			t.Logf("Load, l %v, o %v, len(d) %v, getlen %v", l, o, len(d), getlen)
			t.Errorf("Load(%d, %d) wrong number of bytes read: want %d, got %d", l, o, len(d), len(buf))
			continue
		}

		if l > 0 && l <= len(d) && len(buf) != l {
			t.Logf("Load, l %v, o %v, len(d) %v, getlen %v", l, o, len(d), getlen)
			t.Errorf("Load(%d, %d) wrong number of bytes read: want %d, got %d", l, o, l, len(buf))
			continue
		}

		if l > len(d) && len(buf) != len(d) {
			t.Logf("Load, l %v, o %v, len(d) %v, getlen %v", l, o, len(d), getlen)
			t.Errorf("Load(%d, %d) wrong number of bytes read for overlong read: want %d, got %d", l, o, l, len(buf))
			continue
		}

		if !bytes.Equal(buf, d) {
			t.Logf("Load, l %v, o %v, len(d) %v, getlen %v", l, o, len(d), getlen)
			t.Errorf("Load(%d, %d) returned wrong bytes", l, o)
			continue
		}
	}

	test.OK(t, b.Remove(context.TODO(), handle))
}

// TestList makes sure that the backend implements List() pagination correctly.
func (s *Suite) TestList(t *testing.T) {
	seedRand(t)

	numTestFiles := rand.Intn(20) + 20

	b := s.open(t)
	defer s.close(t, b)

	// Check that the backend is empty to start with
	var found []string
	err := b.List(context.TODO(), restic.PackFile, func(fi restic.FileInfo) error {
		found = append(found, fi.Name)
		return nil
	})
	if err != nil {
		t.Fatalf("List returned error %v", err)
	}
	if found != nil {
		t.Fatalf("backend not empty at start of test - contains: %v", found)
	}

	list1 := make(map[restic.ID]int64)

	for i := 0; i < numTestFiles; i++ {
		data := test.Random(rand.Int(), rand.Intn(100)+55)
		id := restic.Hash(data)
		h := restic.Handle{Type: restic.PackFile, Name: id.String()}
		err := b.Save(context.TODO(), h, restic.NewByteReader(data, b.Hasher()))
		if err != nil {
			t.Fatal(err)
		}
		list1[id] = int64(len(data))
	}

	t.Logf("wrote %v files", len(list1))

	var tests = []struct {
		maxItems int
	}{
		{11}, {23}, {numTestFiles}, {numTestFiles + 10}, {numTestFiles + 1123},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("max-%v", test.maxItems), func(t *testing.T) {
			list2 := make(map[restic.ID]int64)

			type setter interface {
				SetListMaxItems(int)
			}

			if s, ok := b.(setter); ok {
				t.Logf("setting max list items to %d", test.maxItems)
				s.SetListMaxItems(test.maxItems)
			}

			err := b.List(context.TODO(), restic.PackFile, func(fi restic.FileInfo) error {
				id, err := restic.ParseID(fi.Name)
				if err != nil {
					t.Fatal(err)
				}
				list2[id] = fi.Size
				return nil
			})

			if err != nil {
				t.Fatalf("List returned error %v", err)
			}

			t.Logf("loaded %v IDs from backend", len(list2))

			for id, size := range list1 {
				size2, ok := list2[id]
				if !ok {
					t.Errorf("id %v not returned by List()", id.Str())
				}

				if size != size2 {
					t.Errorf("wrong size for id %v returned: want %v, got %v", id.Str(), size, size2)
				}
			}

			for id := range list2 {
				_, ok := list1[id]
				if !ok {
					t.Errorf("extra id %v returned by List()", id.Str())
				}
			}
		})
	}

	t.Logf("remove %d files", numTestFiles)
	handles := make([]restic.Handle, 0, len(list1))
	for id := range list1 {
		handles = append(handles, restic.Handle{Type: restic.PackFile, Name: id.String()})
	}

	err = s.delayedRemove(t, b, handles...)
	if err != nil {
		t.Fatal(err)
	}
}

// TestListCancel tests that the context is respected and the error is returned by List.
func (s *Suite) TestListCancel(t *testing.T) {
	seedRand(t)

	numTestFiles := 5

	b := s.open(t)
	defer s.close(t, b)

	testFiles := make([]restic.Handle, 0, numTestFiles)

	for i := 0; i < numTestFiles; i++ {
		data := []byte(fmt.Sprintf("random test blob %v", i))
		id := restic.Hash(data)
		h := restic.Handle{Type: restic.PackFile, Name: id.String()}
		err := b.Save(context.TODO(), h, restic.NewByteReader(data, b.Hasher()))
		if err != nil {
			t.Fatal(err)
		}
		testFiles = append(testFiles, h)
	}

	t.Run("Cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.TODO())
		cancel()

		// pass in a cancelled context
		err := b.List(ctx, restic.PackFile, func(fi restic.FileInfo) error {
			t.Errorf("got FileInfo %v for cancelled context", fi)
			return nil
		})

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected error not found, want %v, got %v", context.Canceled, err)
		}
	})

	t.Run("First", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.TODO())
		defer cancel()

		i := 0
		err := b.List(ctx, restic.PackFile, func(fi restic.FileInfo) error {
			i++
			// cancel the context on the first file
			if i == 1 {
				cancel()
			}
			return nil
		})

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected error not found, want %v, got %v", context.Canceled, err)
		}

		if i != 1 {
			t.Fatalf("wrong number of files returned by List, want %v, got %v", 1, i)
		}
	})

	t.Run("Last", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.TODO())
		defer cancel()

		i := 0
		err := b.List(ctx, restic.PackFile, func(fi restic.FileInfo) error {
			// cancel the context at the last file
			i++
			if i == numTestFiles {
				cancel()
			}
			return nil
		})

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected error not found, want %v, got %v", context.Canceled, err)
		}

		if i != numTestFiles {
			t.Fatalf("wrong number of files returned by List, want %v, got %v", numTestFiles, i)
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		// rather large timeout, let's try to get at least one item
		timeout := time.Second

		ctxTimeout, cancel := context.WithTimeout(context.TODO(), timeout)
		defer cancel()

		i := 0
		// pass in a context with a timeout
		err := b.List(ctxTimeout, restic.PackFile, func(fi restic.FileInfo) error {
			i++

			// wait until the context is cancelled
			<-ctxTimeout.Done()
			return nil
		})

		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected error not found, want %#v, got %#v", context.DeadlineExceeded, err)
		}

		if i > 2 {
			t.Fatalf("wrong number of files returned by List, want <= 2, got %v", i)
		}
	})

	err := s.delayedRemove(t, b, testFiles...)
	if err != nil {
		t.Fatal(err)
	}
}

type errorCloser struct {
	io.ReadSeeker
	l int64
	t testing.TB
	h []byte
}

func (ec errorCloser) Close() error {
	ec.t.Error("forbidden method close was called")
	return errors.New("forbidden method close was called")
}

func (ec errorCloser) Length() int64 {
	return ec.l
}

func (ec errorCloser) Hash() []byte {
	return ec.h
}

func (ec errorCloser) Rewind() error {
	_, err := ec.ReadSeeker.Seek(0, io.SeekStart)
	return err
}

// TestSave tests saving data in the backend.
func (s *Suite) TestSave(t *testing.T) {
	seedRand(t)

	b := s.open(t)
	defer s.close(t, b)
	var id restic.ID

	saveTests := 10
	if s.MinimalData {
		saveTests = 2
	}

	for i := 0; i < saveTests; i++ {
		length := rand.Intn(1<<23) + 200000
		data := test.Random(23, length)
		id = sha256.Sum256(data)

		h := restic.Handle{
			Type: restic.PackFile,
			Name: id.String(),
		}
		err := b.Save(context.TODO(), h, restic.NewByteReader(data, b.Hasher()))
		test.OK(t, err)

		buf, err := backend.LoadAll(context.TODO(), nil, b, h)
		test.OK(t, err)
		if len(buf) != len(data) {
			t.Fatalf("number of bytes does not match, want %v, got %v", len(data), len(buf))
		}

		if !bytes.Equal(buf, data) {
			t.Fatalf("data not equal")
		}

		fi, err := b.Stat(context.TODO(), h)
		test.OK(t, err)

		if fi.Name != h.Name {
			t.Errorf("Stat() returned wrong name, want %q, got %q", h.Name, fi.Name)
		}

		if fi.Size != int64(len(data)) {
			t.Errorf("Stat() returned different size, want %q, got %d", len(data), fi.Size)
		}

		err = b.Remove(context.TODO(), h)
		if err != nil {
			t.Fatalf("error removing item: %+v", err)
		}
	}

	// test saving from a tempfile
	tmpfile, err := os.CreateTemp("", "restic-backend-save-test-")
	if err != nil {
		t.Fatal(err)
	}

	length := rand.Intn(1<<23) + 200000
	data := test.Random(23, length)
	id = sha256.Sum256(data)

	if _, err = tmpfile.Write(data); err != nil {
		t.Fatal(err)
	}

	if _, err = tmpfile.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	h := restic.Handle{Type: restic.PackFile, Name: id.String()}

	// wrap the tempfile in an errorCloser, so we can detect if the backend
	// closes the reader
	var beHash []byte
	if b.Hasher() != nil {
		beHasher := b.Hasher()
		// must never fail according to interface
		_, err := beHasher.Write(data)
		if err != nil {
			panic(err)
		}
		beHash = beHasher.Sum(nil)
	}
	err = b.Save(context.TODO(), h, errorCloser{
		t:          t,
		l:          int64(length),
		ReadSeeker: tmpfile,
		h:          beHash,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = s.delayedRemove(t, b, h)
	if err != nil {
		t.Fatalf("error removing item: %+v", err)
	}

	if err = tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	if err = os.Remove(tmpfile.Name()); err != nil {
		t.Fatal(err)
	}
}

type incompleteByteReader struct {
	restic.ByteReader
}

func (r *incompleteByteReader) Length() int64 {
	return r.ByteReader.Length() + 42
}

// TestSaveError tests saving data in the backend.
func (s *Suite) TestSaveError(t *testing.T) {
	seedRand(t)

	b := s.open(t)
	defer func() {
		// rclone will report an error when closing the backend. We have to ignore it
		// otherwise this test will always fail
		_ = b.Close()
	}()

	length := rand.Intn(1<<23) + 200000
	data := test.Random(24, length)
	var id restic.ID
	copy(id[:], data)

	// test that incomplete uploads fail
	h := restic.Handle{Type: restic.PackFile, Name: id.String()}
	err := b.Save(context.TODO(), h, &incompleteByteReader{ByteReader: *restic.NewByteReader(data, b.Hasher())})
	// try to delete possible leftovers
	_ = s.delayedRemove(t, b, h)
	if err == nil {
		t.Fatal("incomplete upload did not fail")
	}
}

type wrongByteReader struct {
	restic.ByteReader
}

func (b *wrongByteReader) Hash() []byte {
	h := b.ByteReader.Hash()
	modHash := make([]byte, len(h))
	copy(modHash, h)
	// flip a bit in the hash
	modHash[0] ^= 0x01
	return modHash
}

// TestSaveWrongHash tests that uploads with a wrong hash fail
func (s *Suite) TestSaveWrongHash(t *testing.T) {
	seedRand(t)

	b := s.open(t)
	defer s.close(t, b)
	// nothing to do if the backend doesn't support external hashes
	if b.Hasher() == nil {
		return
	}

	length := rand.Intn(1<<23) + 200000
	data := test.Random(25, length)
	var id restic.ID
	copy(id[:], data)

	// test that upload with hash mismatch fails
	h := restic.Handle{Type: restic.PackFile, Name: id.String()}
	err := b.Save(context.TODO(), h, &wrongByteReader{ByteReader: *restic.NewByteReader(data, b.Hasher())})
	exists, err2 := beTest(context.TODO(), b, h)
	if err2 != nil {
		t.Fatal(err2)
	}
	_ = s.delayedRemove(t, b, h)
	if err == nil {
		t.Fatal("upload with wrong hash did not fail")
	}
	t.Logf("%v", err)
	if exists {
		t.Fatal("Backend returned an error but stored the file anyways")
	}
}

var testStrings = []struct {
	id   string
	data string
}{
	{"c3ab8ff13720e8ad9047dd39466b3c8974e592c2fa383d4a3960714caef0c4f2", "foobar"},
	{"248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1", "abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq"},
	{"cc5d46bdb4991c6eae3eb739c9c8a7a46fe9654fab79c47b4fe48383b5b25e1c", "foo/bar"},
	{"4e54d2c721cbdb730f01b10b62dec622962b36966ec685880effa63d71c808f2", "foo/../../baz"},
}

func store(t testing.TB, b restic.Backend, tpe restic.FileType, data []byte) restic.Handle {
	id := restic.Hash(data)
	h := restic.Handle{Name: id.String(), Type: tpe}
	err := b.Save(context.TODO(), h, restic.NewByteReader([]byte(data), b.Hasher()))
	test.OK(t, err)
	return h
}

// testLoad loads a blob (but discards its contents).
func testLoad(b restic.Backend, h restic.Handle, length int, offset int64) error {
	return b.Load(context.TODO(), h, 0, 0, func(rd io.Reader) (ierr error) {
		_, ierr = io.Copy(io.Discard, rd)
		return ierr
	})
}

func (s *Suite) delayedRemove(t testing.TB, be restic.Backend, handles ...restic.Handle) error {
	// Some backend (swift, I'm looking at you) may implement delayed
	// removal of data. Let's wait a bit if this happens.

	for _, h := range handles {
		err := be.Remove(context.TODO(), h)
		if s.ErrorHandler != nil {
			err = s.ErrorHandler(t, be, err)
		}
		if err != nil {
			return err
		}
	}

	for _, h := range handles {
		start := time.Now()
		attempt := 0
		var found bool
		var err error
		for time.Since(start) <= s.WaitForDelayedRemoval {
			found, err = beTest(context.TODO(), be, h)
			if s.ErrorHandler != nil {
				err = s.ErrorHandler(t, be, err)
			}
			if err != nil {
				return err
			}

			if !found {
				break
			}

			time.Sleep(2 * time.Second)
			attempt++
		}

		if found {
			t.Fatalf("removed blob %v still present after %v (%d attempts)", h, time.Since(start), attempt)
		}
	}

	return nil
}

func delayedList(t testing.TB, b restic.Backend, tpe restic.FileType, max int, maxwait time.Duration) restic.IDs {
	list := restic.NewIDSet()
	start := time.Now()
	for i := 0; i < max; i++ {
		err := b.List(context.TODO(), tpe, func(fi restic.FileInfo) error {
			id := restic.TestParseID(fi.Name)
			list.Insert(id)
			return nil
		})

		if err != nil {
			t.Fatal(err)
		}

		if len(list) < max && time.Since(start) < maxwait {
			time.Sleep(500 * time.Millisecond)
		}
	}

	return list.List()
}

// TestBackend tests all functions of the backend.
func (s *Suite) TestBackend(t *testing.T) {
	b := s.open(t)
	defer s.close(t, b)

	test.Assert(t, !b.IsNotExist(nil), "IsNotExist() recognized nil error")

	for _, tpe := range []restic.FileType{
		restic.PackFile, restic.KeyFile, restic.LockFile,
		restic.SnapshotFile, restic.IndexFile,
	} {
		// detect non-existing files
		for _, ts := range testStrings {
			id, err := restic.ParseID(ts.id)
			test.OK(t, err)

			// test if blob is already in repository
			h := restic.Handle{Type: tpe, Name: id.String()}
			ret, err := beTest(context.TODO(), b, h)
			test.OK(t, err)
			test.Assert(t, !ret, "blob was found to exist before creating")

			// try to stat a not existing blob
			_, err = b.Stat(context.TODO(), h)
			test.Assert(t, err != nil, "blob data could be extracted before creation")
			test.Assert(t, b.IsNotExist(err), "IsNotExist() did not recognize Stat() error: %v", err)

			// try to read not existing blob
			err = testLoad(b, h, 0, 0)
			test.Assert(t, err != nil, "blob could be read before creation")
			test.Assert(t, b.IsNotExist(err), "IsNotExist() did not recognize Load() error: %v", err)

			// try to get string out, should fail
			ret, err = beTest(context.TODO(), b, h)
			test.OK(t, err)
			test.Assert(t, !ret, "id %q was found (but should not have)", ts.id)
		}

		// add files
		for _, ts := range testStrings {
			store(t, b, tpe, []byte(ts.data))

			// test Load()
			h := restic.Handle{Type: tpe, Name: ts.id}
			buf, err := backend.LoadAll(context.TODO(), nil, b, h)
			test.OK(t, err)
			test.Equals(t, ts.data, string(buf))

			// try to read it out with an offset and a length
			start := 1
			end := len(ts.data) - 2
			length := end - start

			buf2 := make([]byte, length)
			var n int
			err = b.Load(context.TODO(), h, len(buf2), int64(start), func(rd io.Reader) (ierr error) {
				n, ierr = io.ReadFull(rd, buf2)
				return ierr
			})
			test.OK(t, err)
			test.OK(t, err)
			test.Equals(t, len(buf2), n)
			test.Equals(t, ts.data[start:end], string(buf2))
		}

		// test adding the first file again
		ts := testStrings[0]
		h := restic.Handle{Type: tpe, Name: ts.id}

		// remove and recreate
		err := s.delayedRemove(t, b, h)
		test.OK(t, err)

		// test that the blob is gone
		ok, err := beTest(context.TODO(), b, h)
		test.OK(t, err)
		test.Assert(t, !ok, "removed blob still present")

		// create blob
		err = b.Save(context.TODO(), h, restic.NewByteReader([]byte(ts.data), b.Hasher()))
		test.OK(t, err)

		// list items
		IDs := restic.IDs{}

		for _, ts := range testStrings {
			id, err := restic.ParseID(ts.id)
			test.OK(t, err)
			IDs = append(IDs, id)
		}

		list := delayedList(t, b, tpe, len(IDs), s.WaitForDelayedRemoval)
		if len(IDs) != len(list) {
			t.Fatalf("wrong number of IDs returned: want %d, got %d", len(IDs), len(list))
		}

		sort.Sort(IDs)
		sort.Sort(list)

		if !reflect.DeepEqual(IDs, list) {
			t.Fatalf("lists aren't equal, want:\n  %v\n  got:\n%v\n", IDs, list)
		}

		var handles []restic.Handle
		for _, ts := range testStrings {
			id, err := restic.ParseID(ts.id)
			test.OK(t, err)

			h := restic.Handle{Type: tpe, Name: id.String()}

			found, err := beTest(context.TODO(), b, h)
			test.OK(t, err)
			test.Assert(t, found, fmt.Sprintf("id %v/%q not found", tpe, id))

			handles = append(handles, h)
		}

		test.OK(t, s.delayedRemove(t, b, handles...))
	}
}

// TestZZZDelete tests the Delete function. The name ensures that this test is executed last.
func (s *Suite) TestZZZDelete(t *testing.T) {
	if !test.TestCleanupTempDirs {
		t.Skipf("not removing backend, TestCleanupTempDirs is false")
	}

	b := s.open(t)
	defer s.close(t, b)

	err := b.Delete(context.TODO())
	if err != nil {
		t.Fatalf("error deleting backend: %+v", err)
	}
}
