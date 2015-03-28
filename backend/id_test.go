package backend_test

import (
	"testing"

	"github.com/restic/restic/backend"
)

var TestStrings = []struct {
	id   string
	data string
}{
	{"c3ab8ff13720e8ad9047dd39466b3c8974e592c2fa383d4a3960714caef0c4f2", "foobar"},
	{"248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1", "abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq"},
	{"cc5d46bdb4991c6eae3eb739c9c8a7a46fe9654fab79c47b4fe48383b5b25e1c", "foo/bar"},
	{"4e54d2c721cbdb730f01b10b62dec622962b36966ec685880effa63d71c808f2", "foo/../../baz"},
}

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
