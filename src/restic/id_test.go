package restic

import (
	"reflect"
	"testing"
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
		id, err := ParseID(test.id)
		if err != nil {
			t.Error(err)
		}

		id2, err := ParseID(test.id)
		if err != nil {
			t.Error(err)
		}
		if !id.Equal(id2) {
			t.Errorf("ID.Equal() does not work as expected")
		}

		ret, err := id.EqualString(test.id)
		if err != nil {
			t.Error(err)
		}
		if !ret {
			t.Error("ID.EqualString() returned wrong value")
		}

		// test json marshalling
		buf, err := id.MarshalJSON()
		if err != nil {
			t.Error(err)
		}
		want := `"` + test.id + `"`
		if string(buf) != want {
			t.Errorf("string comparison failed, wanted %q, got %q", want, string(buf))
		}

		var id3 ID
		err = id3.UnmarshalJSON(buf)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(id, id3) {
			t.Error("ids are not equal")
		}
	}
}
