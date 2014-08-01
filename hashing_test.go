package khepri_test

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"hash"
	"io/ioutil"
	"testing"

	"github.com/fd0/khepri"
)

var static_tests = []struct {
	hash   func() hash.Hash
	text   string
	digest string
}{
	{md5.New, "foobar\n", "14758f1afd44c09b7992073ccf00b43d"},
	// test data from http://www.nsrl.nist.gov/testdata/
	{sha1.New, "abc", "a9993e364706816aba3e25717850c26c9cd0d89d"},
	{sha1.New, "abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq", "84983e441c3bd26ebaae4aa1f95129e5e54670f1"},
}

func TestReader(t *testing.T) {
	for _, test := range static_tests {
		r := khepri.NewHashingReader(bytes.NewBuffer([]byte(test.text)), test.hash)
		buf, err := ioutil.ReadAll(r)
		ok(t, err)
		equals(t, test.text, string(buf))

		equals(t, hex.EncodeToString(r.Hash()), test.digest)
	}
}

func TestWriter(t *testing.T) {
	for _, test := range static_tests {
		var buf bytes.Buffer
		w := khepri.NewHashingWriter(&buf, test.hash)

		_, err := w.Write([]byte(test.text))
		ok(t, err)

		equals(t, hex.EncodeToString(w.Hash()), test.digest)
	}
}
