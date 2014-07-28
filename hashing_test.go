package khepri_test

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"hash"

	"github.com/fd0/khepri"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Hashing", func() {
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

	Describe("Reader", func() {
		Context("Static Strings", func() {
			It("Should compute digest", func() {
				for _, t := range static_tests {
					r := khepri.NewHashingReader(bytes.NewBuffer([]byte(t.text)), t.hash)

					n, err := r.Read(make([]byte, len(t.text)+1))

					if n != len(t.text) {
						Fail("not enough bytes read")
					}

					if err != nil {
						panic(err)
					}

					digest := r.Hash()

					h := hex.EncodeToString(digest)
					Expect(h).Should(Equal(t.digest))
				}
			})
		})

		Context("Random Strings", func() {

		})
	})

	Describe("Writer", func() {
		Context("Static Strings", func() {
			It("Should compute digest", func() {
				for _, t := range static_tests {
					var buf bytes.Buffer
					w := khepri.NewHashingWriter(&buf, t.hash)

					n, err := w.Write([]byte(t.text))

					if n != len(t.text) {
						Fail("not enough bytes written")
					}

					if err != nil {
						panic(err)
					}

					digest := w.Hash()

					h := hex.EncodeToString(digest)
					Expect(h).Should(Equal(t.digest))
				}
			})
		})
	})
})
