package test

import (
	"bytes"
	crand "crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"reflect"
	"sort"
	"testing"

	"github.com/restic/restic/backend"
	. "github.com/restic/restic/test"
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

// Create creates a backend.
func Create(t testing.TB) {
	if CreateFn == nil {
		t.Fatalf("CreateFn not set!")
	}

	be, err := CreateFn()
	if err != nil {
		fmt.Printf("foo\n")
		t.Fatalf("Create returned error: %v", err)
	}

	butInitialized = true

	err = be.Close()
	if err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

// Open opens a previously created backend.
func Open(t testing.TB) {
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

// Location tests that a location string is returned.
func Location(t testing.TB) {
	b := open(t)
	defer close(t)

	l := b.Location()
	if l == "" {
		t.Fatalf("invalid location string %q", l)
	}
}

// Config saves and loads a config from the backend.
func Config(t testing.TB) {
	b := open(t)
	defer close(t)

	var testString = "Config"

	// create config and read it back
	_, err := b.GetReader(backend.Config, "", 0, 0)
	if err == nil {
		t.Fatalf("did not get expected error for non-existing config")
	}

	blob, err := b.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	_, err = blob.Write([]byte(testString))
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	err = blob.Finalize(backend.Config, "")
	if err != nil {
		t.Fatalf("Finalize() error: %v", err)
	}

	// try accessing the config with different names, should all return the
	// same config
	for _, name := range []string{"", "foo", "bar", "0000000000000000000000000000000000000000000000000000000000000000"} {
		rd, err := b.GetReader(backend.Config, name, 0, 0)
		if err != nil {
			t.Fatalf("unable to read config with name %q: %v", name, err)
		}

		buf, err := ioutil.ReadAll(rd)
		if err != nil {
			t.Fatalf("read config error: %v", err)
		}

		err = rd.Close()
		if err != nil {
			t.Fatalf("close error: %v", err)
		}

		if string(buf) != testString {
			t.Fatalf("wrong data returned, want %q, got %q", testString, string(buf))
		}
	}
}

// GetReader tests various ways the GetReader function can be called.
func GetReader(t testing.TB) {
	b := open(t)
	defer close(t)

	length := rand.Intn(1<<24) + 2000

	data := make([]byte, length)
	_, err := io.ReadFull(crand.Reader, data)
	OK(t, err)

	blob, err := b.Create()
	OK(t, err)

	id := backend.Hash(data)

	_, err = blob.Write([]byte(data))
	OK(t, err)
	OK(t, blob.Finalize(backend.Data, id.String()))

	for i := 0; i < 500; i++ {
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

		rd, err := b.GetReader(backend.Data, id.String(), uint(o), uint(l))
		OK(t, err)
		buf, err := ioutil.ReadAll(rd)
		OK(t, err)

		if !bytes.Equal(buf, d) {
			t.Fatalf("data not equal")
		}
	}

	OK(t, b.Remove(backend.Data, id.String()))
}

// Load tests the backend's Load function.
func Load(t testing.TB) {
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

	data := make([]byte, length)
	_, err = io.ReadFull(crand.Reader, data)
	if err != nil {
		t.Fatalf("reading random data failed: %v", err)
	}

	id := backend.Hash(data)

	blob, err := b.Create()
	OK(t, err)

	_, err = blob.Write([]byte(data))
	OK(t, err)
	OK(t, blob.Finalize(backend.Data, id.String()))

	for i := 0; i < 500; i++ {
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
		h := backend.Handle{Type: backend.Data, Name: id.String()}
		n, err := b.Load(h, buf, int64(o))

		// if we requested data beyond the end of the file, ignore
		// ErrUnexpectedEOF error
		if l > len(d) && err == io.ErrUnexpectedEOF {
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

	OK(t, b.Remove(backend.Data, id.String()))
}

// Write tests writing data to the backend.
func Write(t testing.TB) {
	b := open(t)
	defer close(t)

	length := rand.Intn(1<<23) + 2000

	data := make([]byte, length)
	_, err := io.ReadFull(crand.Reader, data)
	OK(t, err)
	id := backend.Hash(data)

	for i := 0; i < 10; i++ {
		blob, err := b.Create()
		OK(t, err)

		o := 0
		for o < len(data) {
			l := rand.Intn(len(data) - o)
			if len(data)-o < 20 {
				l = len(data) - o
			}

			n, err := blob.Write(data[o : o+l])
			OK(t, err)
			if n != l {
				t.Fatalf("wrong number of bytes written, want %v, got %v", l, n)
			}

			o += l
		}

		name := fmt.Sprintf("%s-%d", id, i)
		OK(t, blob.Finalize(backend.Data, name))

		rd, err := b.GetReader(backend.Data, name, 0, 0)
		OK(t, err)

		buf, err := ioutil.ReadAll(rd)
		OK(t, err)

		if len(buf) != len(data) {
			t.Fatalf("number of bytes does not match, want %v, got %v", len(data), len(buf))
		}

		if !bytes.Equal(buf, data) {
			t.Fatalf("data not equal")
		}

		err = b.Remove(backend.Data, name)
		if err != nil {
			t.Fatalf("error removing item: %v", err)
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

	blob, err := b.Create()
	OK(t, err)

	_, err = blob.Write([]byte(data))
	OK(t, err)
	OK(t, blob.Finalize(tpe, id.String()))
}

func read(t testing.TB, rd io.Reader, expectedData []byte) {
	buf, err := ioutil.ReadAll(rd)
	OK(t, err)
	if expectedData != nil {
		Equals(t, expectedData, buf)
	}
}

// Generic tests all functions of the backend.
func Generic(t testing.TB) {
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

			// try to open not existing blob
			_, err = b.GetReader(tpe, id.String(), 0, 0)
			Assert(t, err != nil, "blob data could be extracted before creation")

			// try to read not existing blob
			_, err = b.GetReader(tpe, id.String(), 0, 1)
			Assert(t, err != nil, "blob reader could be obtained before creation")

			// try to get string out, should fail
			ret, err = b.Test(tpe, id.String())
			OK(t, err)
			Assert(t, !ret, "id %q was found (but should not have)", test.id)
		}

		// add files
		for _, test := range testStrings {
			store(t, b, tpe, []byte(test.data))

			// test GetReader()
			rd, err := b.GetReader(tpe, test.id, 0, uint(len(test.data)))
			OK(t, err)
			Assert(t, rd != nil, "GetReader() returned nil")

			read(t, rd, []byte(test.data))
			OK(t, rd.Close())

			// try to read it out with an offset and a length
			start := 1
			end := len(test.data) - 2
			length := end - start
			rd, err = b.GetReader(tpe, test.id, uint(start), uint(length))
			OK(t, err)
			Assert(t, rd != nil, "GetReader() returned nil")

			read(t, rd, []byte(test.data[start:end]))
			OK(t, rd.Close())
		}

		// test adding the first file again
		test := testStrings[0]

		// create blob
		blob, err := b.Create()
		OK(t, err)

		_, err = blob.Write([]byte(test.data))
		OK(t, err)
		err = blob.Finalize(tpe, test.id)
		Assert(t, err != nil, "expected error, got %v", err)

		// remove and recreate
		err = b.Remove(tpe, test.id)
		OK(t, err)

		// test that the blob is gone
		ok, err := b.Test(tpe, test.id)
		OK(t, err)
		Assert(t, ok == false, "removed blob still present")

		// create blob
		blob, err = b.Create()
		OK(t, err)

		_, err = io.Copy(blob, bytes.NewReader([]byte(test.data)))
		OK(t, err)
		OK(t, blob.Finalize(tpe, test.id))

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
		if TestCleanup {
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

// Delete tests the Delete function.
func Delete(t testing.TB) {
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

// Cleanup runs the cleanup function after all tests are run.
func Cleanup(t testing.TB) {
	if CleanupFn == nil {
		t.Log("CleanupFn function not set")
		return
	}

	if !TestCleanup {
		t.Logf("not cleaning up backend")
		return
	}

	err := CleanupFn()
	if err != nil {
		t.Fatalf("Cleanup returned error: %v", err)
	}
}
