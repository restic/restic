package backend_test

import (
	"testing"

	"github.com/restic/restic/backend"
)

func TestID(t *testing.T) {
	for _, test := range TestStrings {
		id, err := backend.ParseID(test.id)
		ok(t, err)

		id2, err := backend.ParseID(test.id)
		ok(t, err)
		assert(t, id.Equal(id2), "ID.Equal() does not work as expected")

		ret, err := id.EqualString(test.id)
		ok(t, err)
		assert(t, ret, "ID.EqualString() returned wrong value")

		// test json marshalling
		buf, err := id.MarshalJSON()
		ok(t, err)
		equals(t, "\""+test.id+"\"", string(buf))

		var id3 backend.ID
		err = id3.UnmarshalJSON(buf)
		ok(t, err)
		equals(t, id, id3)
	}
}
