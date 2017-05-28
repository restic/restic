package test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"reflect"
	"restic"
	"restic/errors"
	"sort"
	"strings"
	"testing"
	"time"

	"restic/test"

	"restic/backend"
)

func seedRand(t testing.TB) {
	seed := time.Now().UnixNano()
	rand.Seed(seed)
	t.Logf("rand initialized with seed %d", seed)
}

// TestCreateWithConfig tests that creating a backend in a location which already
// has a config file fails.
func (s *Suite) TestCreateWithConfig(t *testing.T) {
	b := s.open(t)
	defer s.close(t, b)

	// remove a config if present
	cfgHandle := restic.Handle{Type: restic.ConfigFile}
	cfgPresent, err := b.Test(cfgHandle)
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
	err = b.Remove(restic.Handle{Type: restic.ConfigFile, Name: ""})
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
	_, err := backend.LoadAll(b, restic.Handle{Type: restic.ConfigFile})
	if err == nil {
		t.Fatalf("did not get expected error for non-existing config")
	}

	err = b.Save(restic.Handle{Type: restic.ConfigFile}, strings.NewReader(testString))
	if err != nil {
		t.Fatalf("Save() error: %+v", err)
	}

	// try accessing the config with different names, should all return the
	// same config
	for _, name := range []string{"", "foo", "bar", "0000000000000000000000000000000000000000000000000000000000000000"} {
		h := restic.Handle{Type: restic.ConfigFile, Name: name}
		buf, err := backend.LoadAll(b, h)
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

	_, err := b.Load(restic.Handle{}, 0, 0)
	if err == nil {
		t.Fatalf("Load() did not return an error for invalid handle")
	}

	_, err = b.Load(restic.Handle{Type: restic.DataFile, Name: "foobar"}, 0, 0)
	if err == nil {
		t.Fatalf("Load() did not return an error for non-existing blob")
	}

	length := rand.Intn(1<<24) + 2000

	data := test.Random(23, length)
	id := restic.Hash(data)

	handle := restic.Handle{Type: restic.DataFile, Name: id.String()}
	err = b.Save(handle, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Save() error: %+v", err)
	}

	t.Logf("saved %d bytes as %v", length, handle)

	rd, err := b.Load(handle, 100, -1)
	if err == nil {
		t.Fatalf("Load() returned no error for negative offset!")
	}

	if rd != nil {
		t.Fatalf("Load() returned a non-nil reader for negative offset!")
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

		rd, err := b.Load(handle, getlen, int64(o))
		if err != nil {
			t.Logf("Load, l %v, o %v, len(d) %v, getlen %v", l, o, len(d), getlen)
			t.Errorf("Load(%d, %d) returned unexpected error: %+v", l, o, err)
			continue
		}

		buf, err := ioutil.ReadAll(rd)
		if err != nil {
			t.Logf("Load, l %v, o %v, len(d) %v, getlen %v", l, o, len(d), getlen)
			t.Errorf("Load(%d, %d) ReadAll() returned unexpected error: %+v", l, o, err)
			if err = rd.Close(); err != nil {
				t.Errorf("Load(%d, %d) rd.Close() returned error: %+v", l, o, err)
			}
			continue
		}

		if l == 0 && len(buf) != len(d) {
			t.Logf("Load, l %v, o %v, len(d) %v, getlen %v", l, o, len(d), getlen)
			t.Errorf("Load(%d, %d) wrong number of bytes read: want %d, got %d", l, o, len(d), len(buf))
			if err = rd.Close(); err != nil {
				t.Errorf("Load(%d, %d) rd.Close() returned error: %+v", l, o, err)
			}
			continue
		}

		if l > 0 && l <= len(d) && len(buf) != l {
			t.Logf("Load, l %v, o %v, len(d) %v, getlen %v", l, o, len(d), getlen)
			t.Errorf("Load(%d, %d) wrong number of bytes read: want %d, got %d", l, o, l, len(buf))
			if err = rd.Close(); err != nil {
				t.Errorf("Load(%d, %d) rd.Close() returned error: %+v", l, o, err)
			}
			continue
		}

		if l > len(d) && len(buf) != len(d) {
			t.Logf("Load, l %v, o %v, len(d) %v, getlen %v", l, o, len(d), getlen)
			t.Errorf("Load(%d, %d) wrong number of bytes read for overlong read: want %d, got %d", l, o, l, len(buf))
			if err = rd.Close(); err != nil {
				t.Errorf("Load(%d, %d) rd.Close() returned error: %+v", l, o, err)
			}
			continue
		}

		if !bytes.Equal(buf, d) {
			t.Logf("Load, l %v, o %v, len(d) %v, getlen %v", l, o, len(d), getlen)
			t.Errorf("Load(%d, %d) returned wrong bytes", l, o)
			if err = rd.Close(); err != nil {
				t.Errorf("Load(%d, %d) rd.Close() returned error: %+v", l, o, err)
			}
			continue
		}

		err = rd.Close()
		if err != nil {
			t.Logf("Load, l %v, o %v, len(d) %v, getlen %v", l, o, len(d), getlen)
			t.Errorf("Load(%d, %d) rd.Close() returned unexpected error: %+v", l, o, err)
			continue
		}
	}

	test.OK(t, b.Remove(handle))
}

type errorCloser struct {
	io.Reader
	size int64
	t    testing.TB
}

func (ec errorCloser) Close() error {
	ec.t.Error("forbidden method close was called")
	return errors.New("forbidden method close was called")
}

func (ec errorCloser) Size() int64 {
	return ec.size
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
		// use the first 32 byte as the ID
		copy(id[:], data)

		h := restic.Handle{
			Type: restic.DataFile,
			Name: fmt.Sprintf("%s-%d", id, i),
		}
		err := b.Save(h, bytes.NewReader(data))
		test.OK(t, err)

		buf, err := backend.LoadAll(b, h)
		test.OK(t, err)
		if len(buf) != len(data) {
			t.Fatalf("number of bytes does not match, want %v, got %v", len(data), len(buf))
		}

		if !bytes.Equal(buf, data) {
			t.Fatalf("data not equal")
		}

		fi, err := b.Stat(h)
		test.OK(t, err)

		if fi.Size != int64(len(data)) {
			t.Fatalf("Stat() returned different size, want %q, got %d", len(data), fi.Size)
		}

		err = b.Remove(h)
		if err != nil {
			t.Fatalf("error removing item: %+v", err)
		}
	}

	// test saving from a tempfile
	tmpfile, err := ioutil.TempFile("", "restic-backend-save-test-")
	if err != nil {
		t.Fatal(err)
	}

	length := rand.Intn(1<<23) + 200000
	data := test.Random(23, length)
	copy(id[:], data)

	if _, err = tmpfile.Write(data); err != nil {
		t.Fatal(err)
	}

	if _, err = tmpfile.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	h := restic.Handle{Type: restic.DataFile, Name: id.String()}

	// wrap the tempfile in an errorCloser, so we can detect if the backend
	// closes the reader
	err = b.Save(h, errorCloser{t: t, size: int64(length), Reader: tmpfile})
	if err != nil {
		t.Fatal(err)
	}

	err = b.Remove(h)
	if err != nil {
		t.Fatalf("error removing item: %+v", err)
	}

	// try again directly with the temp file
	if _, err = tmpfile.Seek(588, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	err = b.Save(h, tmpfile)
	if err != nil {
		t.Fatal(err)
	}

	if err = tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	err = b.Remove(h)
	if err != nil {
		t.Fatalf("error removing item: %+v", err)
	}

	if err = os.Remove(tmpfile.Name()); err != nil {
		t.Fatal(err)
	}
}

var filenameTests = []struct {
	name string
	data string
}{
	{"1dfc6bc0f06cb255889e9ea7860a5753e8eb9665c9a96627971171b444e3113e", "x"},
	{"f00b4r", "foobar"},
	{
		"1dfc6bc0f06cb255889e9ea7860a5753e8eb9665c9a96627971171b444e3113e4bf8f2d9144cc5420a80f04a4880ad6155fc58903a4fb6457c476c43541dcaa6-5",
		"foobar content of data blob",
	},
}

// TestSaveFilenames tests saving data with various file names in the backend.
func (s *Suite) TestSaveFilenames(t *testing.T) {
	b := s.open(t)
	defer s.close(t, b)

	for i, test := range filenameTests {
		h := restic.Handle{Name: test.name, Type: restic.DataFile}
		err := b.Save(h, strings.NewReader(test.data))
		if err != nil {
			t.Errorf("test %d failed: Save() returned %+v", i, err)
			continue
		}

		buf, err := backend.LoadAll(b, h)
		if err != nil {
			t.Errorf("test %d failed: Load() returned %+v", i, err)
			continue
		}

		if !bytes.Equal(buf, []byte(test.data)) {
			t.Errorf("test %d: returned wrong bytes", i)
		}

		err = b.Remove(h)
		if err != nil {
			t.Errorf("test %d failed: Remove() returned %+v", i, err)
			continue
		}
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
	err := b.Save(h, bytes.NewReader(data))
	test.OK(t, err)
	return h
}

// TestBackend tests all functions of the backend.
func (s *Suite) TestBackend(t *testing.T) {
	b := s.open(t)
	defer s.close(t, b)

	for _, tpe := range []restic.FileType{
		restic.DataFile, restic.KeyFile, restic.LockFile,
		restic.SnapshotFile, restic.IndexFile,
	} {
		// detect non-existing files
		for _, ts := range testStrings {
			id, err := restic.ParseID(ts.id)
			test.OK(t, err)

			// test if blob is already in repository
			h := restic.Handle{Type: tpe, Name: id.String()}
			ret, err := b.Test(h)
			test.OK(t, err)
			test.Assert(t, !ret, "blob was found to exist before creating")

			// try to stat a not existing blob
			_, err = b.Stat(h)
			test.Assert(t, err != nil, "blob data could be extracted before creation")

			// try to read not existing blob
			_, err = b.Load(h, 0, 0)
			test.Assert(t, err != nil, "blob reader could be obtained before creation")

			// try to get string out, should fail
			ret, err = b.Test(h)
			test.OK(t, err)
			test.Assert(t, !ret, "id %q was found (but should not have)", ts.id)
		}

		// add files
		for _, ts := range testStrings {
			store(t, b, tpe, []byte(ts.data))

			// test Load()
			h := restic.Handle{Type: tpe, Name: ts.id}
			buf, err := backend.LoadAll(b, h)
			test.OK(t, err)
			test.Equals(t, ts.data, string(buf))

			// try to read it out with an offset and a length
			start := 1
			end := len(ts.data) - 2
			length := end - start

			buf2 := make([]byte, length)
			rd, err := b.Load(h, len(buf2), int64(start))
			test.OK(t, err)
			n, err := io.ReadFull(rd, buf2)
			test.OK(t, err)
			test.Equals(t, len(buf2), n)

			remaining, err := io.Copy(ioutil.Discard, rd)
			test.OK(t, err)
			test.Equals(t, int64(0), remaining)

			test.OK(t, rd.Close())

			test.Equals(t, ts.data[start:end], string(buf2))
		}

		// test adding the first file again
		ts := testStrings[0]

		// create blob
		h := restic.Handle{Type: tpe, Name: ts.id}
		err := b.Save(h, strings.NewReader(ts.data))
		test.Assert(t, err != nil, "expected error for %v, got %v", h, err)

		// remove and recreate
		err = b.Remove(h)
		test.OK(t, err)

		// test that the blob is gone
		ok, err := b.Test(h)
		test.OK(t, err)
		test.Assert(t, !ok, "removed blob still present")

		// create blob
		err = b.Save(h, strings.NewReader(ts.data))
		test.OK(t, err)

		// list items
		IDs := restic.IDs{}

		for _, ts := range testStrings {
			id, err := restic.ParseID(ts.id)
			test.OK(t, err)
			IDs = append(IDs, id)
		}

		list := restic.IDs{}

		for s := range b.List(tpe, nil) {
			list = append(list, restic.TestParseID(s))
		}

		if len(IDs) != len(list) {
			t.Fatalf("wrong number of IDs returned: want %d, got %d", len(IDs), len(list))
		}

		sort.Sort(IDs)
		sort.Sort(list)

		if !reflect.DeepEqual(IDs, list) {
			t.Fatalf("lists aren't equal, want:\n  %v\n  got:\n%v\n", IDs, list)
		}

		// remove content if requested
		if test.TestCleanupTempDirs {
			for _, ts := range testStrings {
				id, err := restic.ParseID(ts.id)
				test.OK(t, err)

				h := restic.Handle{Type: tpe, Name: id.String()}

				found, err := b.Test(h)
				test.OK(t, err)
				test.Assert(t, found, fmt.Sprintf("id %q not found", id))

				test.OK(t, b.Remove(h))

				found, err = b.Test(h)
				test.OK(t, err)
				test.Assert(t, !found, fmt.Sprintf("id %q not found after removal", id))
			}
		}
	}
}

// TestDelete tests the Delete function.
func (s *Suite) TestDelete(t *testing.T) {
	if !test.TestCleanupTempDirs {
		t.Skipf("not removing backend, TestCleanupTempDirs is false")
	}

	b := s.open(t)
	defer s.close(t, b)

	be, ok := b.(restic.Deleter)
	if !ok {
		return
	}

	err := be.Delete()
	if err != nil {
		t.Fatalf("error deleting backend: %+v", err)
	}
}
