package backend_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"sort"
	"testing"

	"github.com/restic/restic/backend"
)

func testBackend(b backend.Backend, t *testing.T) {
	for _, tpe := range []backend.Type{backend.Data, backend.Key, backend.Lock, backend.Snapshot, backend.Tree} {
		// detect non-existing files
		for _, test := range TestStrings {
			id, err := backend.ParseID(test.id)
			ok(t, err)

			// test if blob is already in repository
			ret, err := b.Test(tpe, id.String())
			ok(t, err)
			assert(t, !ret, "blob was found to exist before creating")

			// try to open not existing blob
			_, err = b.Get(tpe, id.String())
			assert(t, err != nil, "blob data could be extracted before creation")

			// try to get string out, should fail
			ret, err = b.Test(tpe, id.String())
			ok(t, err)
			assert(t, !ret, "id %q was found (but should not have)", test.id)
		}

		// add files
		for _, test := range TestStrings {
			// store string in backend
			blob, err := b.Create()
			ok(t, err)

			_, err = blob.Write([]byte(test.data))
			ok(t, err)
			ok(t, blob.Finalize(tpe, test.id))

			// try to get it out again
			rd, err := b.Get(tpe, test.id)
			ok(t, err)
			assert(t, rd != nil, "Get() returned nil")

			buf, err := ioutil.ReadAll(rd)
			ok(t, err)
			equals(t, test.data, string(buf))

			// compare content
			equals(t, test.data, string(buf))
		}

		// test adding the first file again
		test := TestStrings[0]

		// create blob
		blob, err := b.Create()
		ok(t, err)

		_, err = blob.Write([]byte(test.data))
		ok(t, err)
		err = blob.Finalize(tpe, test.id)
		assert(t, err != nil, "expected error, got %v", err)

		// remove and recreate
		err = b.Remove(tpe, test.id)
		ok(t, err)

		// create blob
		blob, err = b.Create()
		ok(t, err)

		_, err = io.Copy(blob, bytes.NewReader([]byte(test.data)))
		ok(t, err)
		ok(t, blob.Finalize(tpe, test.id))

		// list items
		IDs := backend.IDs{}

		for _, test := range TestStrings {
			id, err := backend.ParseID(test.id)
			ok(t, err)
			IDs = append(IDs, id)
		}

		sort.Sort(IDs)

		i := 0
		for s := range b.List(tpe, nil) {
			equals(t, IDs[i].String(), s)
			i++
		}

		// remove content if requested
		if *testCleanup {
			for _, test := range TestStrings {
				id, err := backend.ParseID(test.id)
				ok(t, err)

				found, err := b.Test(tpe, id.String())
				ok(t, err)
				assert(t, found, fmt.Sprintf("id %q was not found before removal", id))

				ok(t, b.Remove(tpe, id.String()))

				found, err = b.Test(tpe, id.String())
				ok(t, err)
				assert(t, !found, fmt.Sprintf("id %q not found after removal", id))
			}
		}

	}
}
