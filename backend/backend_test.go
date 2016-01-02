package backend_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"sort"
	"testing"

	crand "crypto/rand"

	"github.com/restic/restic/backend"
	. "github.com/restic/restic/test"
)

func testBackendConfig(b backend.Backend, t *testing.T) {
	// create config and read it back
	_, err := b.Get(backend.Config, "")
	Assert(t, err != nil, "did not get expected error for non-existing config")

	blob, err := b.Create()
	OK(t, err)

	_, err = blob.Write([]byte("Config"))
	OK(t, err)
	OK(t, blob.Finalize(backend.Config, ""))

	// try accessing the config with different names, should all return the
	// same config
	for _, name := range []string{"", "foo", "bar", "0000000000000000000000000000000000000000000000000000000000000000"} {
		rd, err := b.Get(backend.Config, name)
		Assert(t, err == nil, "unable to read config")

		buf, err := ioutil.ReadAll(rd)
		OK(t, err)
		OK(t, rd.Close())
		Assert(t, string(buf) == "Config", "wrong data returned for config")
	}
}

func testGetReader(b backend.Backend, t testing.TB) {
	length := rand.Intn(1<<23) + 2000

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

func testBackend(b backend.Backend, t *testing.T) {
	testBackendConfig(b, t)

	for _, tpe := range []backend.Type{
		backend.Data, backend.Key, backend.Lock,
		backend.Snapshot, backend.Index,
	} {
		// detect non-existing files
		for _, test := range TestStrings {
			id, err := backend.ParseID(test.id)
			OK(t, err)

			// test if blob is already in repository
			ret, err := b.Test(tpe, id.String())
			OK(t, err)
			Assert(t, !ret, "blob was found to exist before creating")

			// try to open not existing blob
			_, err = b.Get(tpe, id.String())
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
		for _, test := range TestStrings {
			store(t, b, tpe, []byte(test.data))

			// test Get()
			rd, err := b.Get(tpe, test.id)
			OK(t, err)
			Assert(t, rd != nil, "Get() returned nil")

			read(t, rd, []byte(test.data))
			OK(t, rd.Close())

			// test GetReader()
			rd, err = b.GetReader(tpe, test.id, 0, uint(len(test.data)))
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
		test := TestStrings[0]

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

		for _, test := range TestStrings {
			id, err := backend.ParseID(test.id)
			OK(t, err)
			IDs = append(IDs, id)
		}

		sort.Sort(IDs)

		i := 0
		for s := range b.List(tpe, nil) {
			Equals(t, IDs[i].String(), s)
			i++
		}

		// remove content if requested
		if TestCleanup {
			for _, test := range TestStrings {
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

	testGetReader(b, t)
}
