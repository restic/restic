package backend_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"sort"
	"testing"

	"github.com/restic/restic/backend"
	. "github.com/restic/restic/test"
)

func testBackend(b backend.Backend, t *testing.T) {
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

			// try to get string out, should fail
			ret, err = b.Test(tpe, id.String())
			OK(t, err)
			Assert(t, !ret, "id %q was found (but should not have)", test.id)
		}

		// add files
		for _, test := range TestStrings {
			// store string in backend
			blob, err := b.Create()
			OK(t, err)

			_, err = blob.Write([]byte(test.data))
			OK(t, err)
			OK(t, blob.Finalize(tpe, test.id))

			// try to get it out again
			rd, err := b.Get(tpe, test.id)
			OK(t, err)
			Assert(t, rd != nil, "Get() returned nil")

			buf, err := ioutil.ReadAll(rd)
			OK(t, err)
			Equals(t, test.data, string(buf))

			// compare content
			Equals(t, test.data, string(buf))
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
				Assert(t, found, fmt.Sprintf("id %q was not found before removal", id))

				OK(t, b.Remove(tpe, id.String()))

				found, err = b.Test(tpe, id.String())
				OK(t, err)
				Assert(t, !found, fmt.Sprintf("id %q not found after removal", id))
			}
		}

	}
}
