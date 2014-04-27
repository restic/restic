package storage_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/fd0/khepri/storage"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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

var _ = Describe("Storage", func() {
	var (
		tempdir string
		repo    *storage.DirRepository
		err     error
		id      storage.ID
	)

	BeforeEach(func() {
		tempdir, err = ioutil.TempDir("", "khepri-test-")
		if err != nil {
			panic(err)
		}
		repo, err = storage.NewDirRepository(tempdir)
		if err != nil {
			panic(err)
		}
	})

	AfterEach(func() {
		err = os.RemoveAll(tempdir)
		if err != nil {
			panic(err)
		}
		// fmt.Fprintf(os.Stderr, "leaving tempdir %s", tempdir)
		tempdir = ""
	})

	Describe("Repository", func() {
		Context("File Operations", func() {
			It("Should detect non-existing file", func() {
				for _, test := range TestStrings {
					id, err := storage.ParseID(test.id)
					Expect(err).NotTo(HaveOccurred())

					// try to get string out, should fail
					ret, err := repo.Test(id)
					Expect(ret).Should(Equal(false))
				}
			})

			It("Should Add File", func() {
				for _, test := range TestStrings {
					// store string in repository
					id, err = repo.Put(strings.NewReader(test.data))

					Expect(err).NotTo(HaveOccurred())
					Expect(id.String()).Should(Equal(test.id))

					// try to get it out again
					var buf bytes.Buffer
					rd, err := repo.Get(id)
					Expect(err).NotTo(HaveOccurred())
					Expect(rd).ShouldNot(BeNil())

					// compare content
					Expect(io.Copy(&buf, rd)).Should(Equal(int64(len(test.data))))
					Expect(buf.Bytes()).Should(Equal([]byte(test.data)))

					// store id under name
					err = repo.Link(test.data, id)
					Expect(err).NotTo(HaveOccurred())

					// resolve again
					Expect(repo.Resolve(test.data)).Should(Equal(id))

					// remove link
					Expect(repo.Unlink(test.data)).NotTo(HaveOccurred())

					// remove string
					Expect(repo.Remove(id))
				}
			})

			It("Should Add Buffer", func() {
				for _, test := range TestStrings {
					// store buf in repository
					id, err := repo.PutRaw([]byte(test.data))
					Expect(err).NotTo(HaveOccurred())
					Expect(id.String()).To(Equal(test.id))
				}
			})
		})
	})
})
