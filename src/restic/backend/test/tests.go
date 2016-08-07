package test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"reflect"
	"sort"
	"testing"

	"restic/backend"
	. "restic/test"
)

// CreateFn is a function that creates a temporary repository for the tests.
var CreateFn func() (backend.Backend, error)

// OpenFn is a function that opens a previously created temporary repository.
var OpenFn func() (backend.Backend, error)

// CleanupFn removes temporary files and directories created during the tests.
var CleanupFn func() error

var but backend.Backend // backendUnderTest
var butInitialized bool

func open(t testing.TB) backend.Backend {
	if OpenFn == nil {
		t.Fatal("OpenFn not set")
	}

	if CreateFn == nil {
		t.Fatalf("CreateFn not set")
	}

	if !butInitialized {
		be, err := CreateFn()
		if err != nil {
			t.Fatalf("Create returned unexpected error: %v", err)
		}

		but = be
		butInitialized = true
	}

	if but == nil {
		var err error
		but, err = OpenFn()
		if err != nil {
			t.Fatalf("Open returned unexpected error: %v", err)
		}
	}

	return but
}

func close(t testing.TB) {
	if but == nil {
		t.Fatalf("trying to close non-existing backend")
	}

	err := but.Close()
	if err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}

	but = nil
}

// TestCreate creates a backend.
func TestCreate(t testing.TB) {
	if CreateFn == nil {
		t.Fatalf("CreateFn not set!")
	}

	be, err := CreateFn()
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	butInitialized = true

	err = be.Close()
	if err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

// TestOpen opens a previously created backend.
func TestOpen(t testing.TB) {
	if OpenFn == nil {
		t.Fatalf("OpenFn not set!")
	}

	be, err := OpenFn()
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}

	err = be.Close()
	if err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

// TestCreateWithConfig tests that creating a backend in a location which already
// has a config file fails.
func TestCreateWithConfig(t testing.TB) {
	if CreateFn == nil {
		t.Fatalf("CreateFn not set")
	}

	b := open(t)
	defer close(t)

	// save a config
	store(t, b, backend.Config, []byte("test config"))

	// now create the backend again, this must fail
	_, err := CreateFn()
	if err == nil {
		t.Fatalf("expected error not found for creating a backend with an existing config file")
	}

	// remove config
	err = b.Remove(backend.Config, "")
	if err != nil {
		t.Fatalf("unexpected error removing config: %v", err)
	}
}

// TestLocation tests that a location string is returned.
func TestLocation(t testing.TB) {
	b := open(t)
	defer close(t)

	l := b.Location()
	if l == "" {
		t.Fatalf("invalid location string %q", l)
	}
}

// TestConfig saves and loads a config from the backend.
func TestConfig(t testing.TB) {
	b := open(t)
	defer close(t)

	var testString = "Config"

	// create config and read it back
	_, err := backend.LoadAll(b, backend.Handle{Type: backend.Config}, nil)
	if err == nil {
		t.Fatalf("did not get expected error for non-existing config")
	}

	err = b.Save(backend.Handle{Type: backend.Config}, []byte(testString))
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// try accessing the config with different names, should all return the
	// same config
	for _, name := range []string{"", "foo", "bar", "0000000000000000000000000000000000000000000000000000000000000000"} {
		h := backend.Handle{Type: backend.Config, Name: name}
		buf, err := backend.LoadAll(b, h, nil)
		if err != nil {
			t.Fatalf("unable to read config with name %q: %v", name, err)
		}

		if string(buf) != testString {
			t.Fatalf("wrong data returned, want %q, got %q", testString, string(buf))
		}
	}
}

// TestLoad tests the backend's Load function.
func TestLoad(t testing.TB) {
	b := open(t)
	defer close(t)

	_, err := b.Load(backend.Handle{}, nil, 0)
	if err == nil {
		t.Fatalf("Load() did not return an error for invalid handle")
	}

	_, err = b.Load(backend.Handle{Type: backend.Data, Name: "foobar"}, nil, 0)
	if err == nil {
		t.Fatalf("Load() did not return an error for non-existing blob")
	}

	length := rand.Intn(1<<24) + 2000

	data := Random(23, length)
	id := backend.Hash(data)

	handle := backend.Handle{Type: backend.Data, Name: id.String()}
	err = b.Save(handle, data)
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	for i := 0; i < 50; i++ {
		l := rand.Intn(length + 2000)
		o := rand.Intn(length + 2000)

		d := data
		if o < len(d) {
			d = d[o:]
		} else {
			o = len(d)
			d = d[:0]
		}

		if l > 0 && l < len(d) {
			d = d[:l]
		}

		buf := make([]byte, l)
		n, err := b.Load(handle, buf, int64(o))

		// if we requested data beyond the end of the file, require
		// ErrUnexpectedEOF error
		if l > len(d) {
			if err != io.ErrUnexpectedEOF {
				t.Errorf("Load(%d, %d) did not return io.ErrUnexpectedEOF", len(buf), int64(o))
			}
			err = nil
			buf = buf[:len(d)]
		}

		if err != nil {
			t.Errorf("Load(%d, %d): unexpected error: %v", len(buf), int64(o), err)
			continue
		}

		if n != len(buf) {
			t.Errorf("Load(%d, %d): wrong length returned, want %d, got %d",
				len(buf), int64(o), len(buf), n)
			continue
		}

		buf = buf[:n]
		if !bytes.Equal(buf, d) {
			t.Errorf("Load(%d, %d) returned wrong bytes", len(buf), int64(o))
			continue
		}
	}

	// test with negative offset
	for i := 0; i < 50; i++ {
		l := rand.Intn(length + 2000)
		o := rand.Intn(length + 2000)

		d := data
		if o < len(d) {
			d = d[len(d)-o:]
		} else {
			o = 0
		}

		if l > 0 && l < len(d) {
			d = d[:l]
		}

		buf := make([]byte, l)
		n, err := b.Load(handle, buf, -int64(o))

		// if we requested data beyond the end of the file, require
		// ErrUnexpectedEOF error
		if l > len(d) {
			if err != io.ErrUnexpectedEOF {
				t.Errorf("Load(%d, %d) did not return io.ErrUnexpectedEOF", len(buf), int64(o))
			}
			err = nil
			buf = buf[:len(d)]
		}

		if err != nil {
			t.Errorf("Load(%d, %d): unexpected error: %v", len(buf), int64(o), err)
			continue
		}

		if n != len(buf) {
			t.Errorf("Load(%d, %d): wrong length returned, want %d, got %d",
				len(buf), int64(o), len(buf), n)
			continue
		}

		buf = buf[:n]
		if !bytes.Equal(buf, d) {
			t.Errorf("Load(%d, %d) returned wrong bytes", len(buf), int64(o))
			continue
		}
	}

	// load with a too-large buffer, this should return io.ErrUnexpectedEOF
	buf := make([]byte, length+100)
	n, err := b.Load(handle, buf, 0)
	if n != length {
		t.Errorf("wrong length for larger buffer returned, want %d, got %d", length, n)
	}

	if err != io.ErrUnexpectedEOF {
		t.Errorf("wrong error returned for larger buffer: want io.ErrUnexpectedEOF, got %#v", err)
	}

	OK(t, b.Remove(backend.Data, id.String()))
}

// TestLoadNegativeOffset tests the backend's Load function with negative offsets.
func TestLoadNegativeOffset(t testing.TB) {
	b := open(t)
	defer close(t)

	length := rand.Intn(1<<24) + 2000

	data := Random(23, length)
	id := backend.Hash(data)

	handle := backend.Handle{Type: backend.Data, Name: id.String()}
	err := b.Save(handle, data)
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// test normal reads
	for i := 0; i < 50; i++ {
		l := rand.Intn(length + 2000)
		o := -rand.Intn(length + 2000)

		buf := make([]byte, l)
		n, err := b.Load(handle, buf, int64(o))
		t.Logf("data %v, load(%v, %v) -> %v %v",
			len(data), len(buf), o, n, err)

		// if we requested data beyond the end of the file, require
		// ErrUnexpectedEOF error
		if len(buf) > -o {
			if err != io.ErrUnexpectedEOF {
				t.Errorf("Load(%d, %d) did not return io.ErrUnexpectedEOF", len(buf), o)
			}
			err = nil
			buf = buf[:-o]
		}

		if err != nil {
			t.Errorf("Load(%d, %d) returned error: %v", len(buf), o, err)
		}

		if n != len(buf) {
			t.Errorf("Load(%d, %d) returned short read, only got %d bytes", len(buf), o, n)
		}

		p := len(data) + o
		if !bytes.Equal(buf, data[p:p+len(buf)]) {
			t.Errorf("Load(%d, %d) returned wrong bytes", len(buf), o)
			continue
		}

	}

	OK(t, b.Remove(backend.Data, id.String()))
}

// TestSave tests saving data in the backend.
func TestSave(t testing.TB) {
	b := open(t)
	defer close(t)
	var id backend.ID

	for i := 0; i < 10; i++ {
		length := rand.Intn(1<<23) + 200000
		data := Random(23, length)
		// use the first 32 byte as the ID
		copy(id[:], data)

		h := backend.Handle{
			Type: backend.Data,
			Name: fmt.Sprintf("%s-%d", id, i),
		}
		err := b.Save(h, data)
		OK(t, err)

		buf, err := backend.LoadAll(b, h, nil)
		OK(t, err)
		if len(buf) != len(data) {
			t.Fatalf("number of bytes does not match, want %v, got %v", len(data), len(buf))
		}

		if !bytes.Equal(buf, data) {
			t.Fatalf("data not equal")
		}

		fi, err := b.Stat(h)
		OK(t, err)

		if fi.Size != int64(len(data)) {
			t.Fatalf("Stat() returned different size, want %q, got %d", len(data), fi.Size)
		}

		err = b.Remove(h.Type, h.Name)
		if err != nil {
			t.Fatalf("error removing item: %v", err)
		}
	}
}

var filenameTests = []struct {
	name string
	data string
}{
	{"1dfc6bc0f06cb255889e9ea7860a5753e8eb9665c9a96627971171b444e3113e", "x"},
	{"foobar", "foobar"},
	{
		"1dfc6bc0f06cb255889e9ea7860a5753e8eb9665c9a96627971171b444e3113e4bf8f2d9144cc5420a80f04a4880ad6155fc58903a4fb6457c476c43541dcaa6-5",
		"foobar content of data blob",
	},
}

// TestSaveFilenames tests saving data with various file names in the backend.
func TestSaveFilenames(t testing.TB) {
	b := open(t)
	defer close(t)

	for i, test := range filenameTests {
		h := backend.Handle{Name: test.name, Type: backend.Data}
		err := b.Save(h, []byte(test.data))
		if err != nil {
			t.Errorf("test %d failed: Save() returned %v", i, err)
			continue
		}

		buf, err := backend.LoadAll(b, h, nil)
		if err != nil {
			t.Errorf("test %d failed: Load() returned %v", i, err)
			continue
		}

		if !bytes.Equal(buf, []byte(test.data)) {
			t.Errorf("test %d: returned wrong bytes", i)
		}

		err = b.Remove(h.Type, h.Name)
		if err != nil {
			t.Errorf("test %d failed: Remove() returned %v", i, err)
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

func store(t testing.TB, b backend.Backend, tpe backend.Type, data []byte) {
	id := backend.Hash(data)
	err := b.Save(backend.Handle{Name: id.String(), Type: tpe}, data)
	OK(t, err)
}

func read(t testing.TB, rd io.Reader, expectedData []byte) {
	buf, err := ioutil.ReadAll(rd)
	OK(t, err)
	if expectedData != nil {
		Equals(t, expectedData, buf)
	}
}

// TestBackend tests all functions of the backend.
func TestBackend(t testing.TB) {
	b := open(t)
	defer close(t)

	for _, tpe := range []backend.Type{
		backend.Data, backend.Key, backend.Lock,
		backend.Snapshot, backend.Index,
	} {
		// detect non-existing files
		for _, test := range testStrings {
			id, err := backend.ParseID(test.id)
			OK(t, err)

			// test if blob is already in repository
			ret, err := b.Test(tpe, id.String())
			OK(t, err)
			Assert(t, !ret, "blob was found to exist before creating")

			// try to stat a not existing blob
			h := backend.Handle{Type: tpe, Name: id.String()}
			_, err = b.Stat(h)
			Assert(t, err != nil, "blob data could be extracted before creation")

			// try to read not existing blob
			_, err = b.Load(h, nil, 0)
			Assert(t, err != nil, "blob reader could be obtained before creation")

			// try to get string out, should fail
			ret, err = b.Test(tpe, id.String())
			OK(t, err)
			Assert(t, !ret, "id %q was found (but should not have)", test.id)
		}

		// add files
		for _, test := range testStrings {
			store(t, b, tpe, []byte(test.data))

			// test Load()
			h := backend.Handle{Type: tpe, Name: test.id}
			buf, err := backend.LoadAll(b, h, nil)
			OK(t, err)
			Equals(t, test.data, string(buf))

			// try to read it out with an offset and a length
			start := 1
			end := len(test.data) - 2
			length := end - start

			buf2 := make([]byte, length)
			n, err := b.Load(h, buf2, int64(start))
			OK(t, err)
			Equals(t, length, n)
			Equals(t, test.data[start:end], string(buf2))
		}

		// test adding the first file again
		test := testStrings[0]

		// create blob
		err := b.Save(backend.Handle{Type: tpe, Name: test.id}, []byte(test.data))
		Assert(t, err != nil, "expected error, got %v", err)

		// remove and recreate
		err = b.Remove(tpe, test.id)
		OK(t, err)

		// test that the blob is gone
		ok, err := b.Test(tpe, test.id)
		OK(t, err)
		Assert(t, ok == false, "removed blob still present")

		// create blob
		err = b.Save(backend.Handle{Type: tpe, Name: test.id}, []byte(test.data))
		OK(t, err)

		// list items
		IDs := backend.IDs{}

		for _, test := range testStrings {
			id, err := backend.ParseID(test.id)
			OK(t, err)
			IDs = append(IDs, id)
		}

		list := backend.IDs{}

		for s := range b.List(tpe, nil) {
			list = append(list, ParseID(s))
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
		if TestCleanupTempDirs {
			for _, test := range testStrings {
				id, err := backend.ParseID(test.id)
				OK(t, err)

				found, err := b.Test(tpe, id.String())
				OK(t, err)

				OK(t, b.Remove(tpe, id.String()))

				found, err = b.Test(tpe, id.String())
				OK(t, err)
				Assert(t, !found, fmt.Sprintf("id %q not found after removal", id))
			}
		}
	}
}

// TestDelete tests the Delete function.
func TestDelete(t testing.TB) {
	b := open(t)
	defer close(t)

	be, ok := b.(backend.Deleter)
	if !ok {
		return
	}

	err := be.Delete()
	if err != nil {
		t.Fatalf("error deleting backend: %v", err)
	}
}

// TestCleanup runs the cleanup function after all tests are run.
func TestCleanup(t testing.TB) {
	if CleanupFn == nil {
		t.Log("CleanupFn function not set")
		return
	}

	if !TestCleanupTempDirs {
		t.Logf("not cleaning up backend")
		return
	}

	err := CleanupFn()
	if err != nil {
		t.Fatalf("Cleanup returned error: %v", err)
	}
}
