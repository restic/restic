package khepri_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"sort"
	"strings"

	"github.com/fd0/khepri"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var TestStrings = []struct {
	id   string
	t    khepri.Type
	data string
}{
	{"c3ab8ff13720e8ad9047dd39466b3c8974e592c2fa383d4a3960714caef0c4f2", khepri.TypeBlob, "foobar"},
	{"248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1", khepri.TypeBlob, "abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq"},
	{"cc5d46bdb4991c6eae3eb739c9c8a7a46fe9654fab79c47b4fe48383b5b25e1c", khepri.TypeRef, "foo/bar"},
	{"4e54d2c721cbdb730f01b10b62dec622962b36966ec685880effa63d71c808f2", khepri.TypeBlob, "foo/../../baz"},
}

var _ = Describe("Storage", func() {
	var (
		tempdir string
		repo    *khepri.DirRepository
		err     error
		id      khepri.ID
	)

	var _ = BeforeSuite(func() {
		tempdir, err = ioutil.TempDir("", "khepri-test-")
		if err != nil {
			panic(err)
		}
		repo, err = khepri.NewDirRepository(tempdir)
		if err != nil {
			panic(err)
		}
	})

	AfterSuite(func() {
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
					id, err := khepri.ParseID(test.id)
					Expect(err).NotTo(HaveOccurred())

					// try to get string out, should fail
					ret, err := repo.Test(test.t, id)
					Expect(ret).Should(Equal(false))
				}
			})

			It("Should Add File", func() {
				for _, test := range TestStrings {
					// store string in repository
					id, err = repo.Put(test.t, strings.NewReader(test.data))

					Expect(err).NotTo(HaveOccurred())
					Expect(id.String()).Should(Equal(test.id))

					// try to get it out again
					var buf bytes.Buffer
					rd, err := repo.Get(test.t, id)
					Expect(err).NotTo(HaveOccurred())
					Expect(rd).ShouldNot(BeNil())

					// compare content
					Expect(io.Copy(&buf, rd)).Should(Equal(int64(len(test.data))))
					Expect(buf.Bytes()).Should(Equal([]byte(test.data)))
				}
			})

			It("Should Add Buffer", func() {
				for _, test := range TestStrings {
					// store buf in repository
					id, err := repo.PutRaw(test.t, []byte(test.data))
					Expect(err).NotTo(HaveOccurred())
					Expect(id.String()).To(Equal(test.id))
				}
			})

			It("Should List IDs", func() {
				for _, t := range []khepri.Type{khepri.TypeBlob, khepri.TypeRef} {
					IDs := khepri.IDs{}
					for _, test := range TestStrings {
						if test.t == t {
							id, err := khepri.ParseID(test.id)
							Expect(err).NotTo(HaveOccurred())
							IDs = append(IDs, id)
						}
					}

					ids, err := repo.ListIDs(t)

					sort.Sort(ids)
					sort.Sort(IDs)
					Expect(err).NotTo(HaveOccurred())
					Expect(ids).Should(Equal(IDs))
				}
			})

			It("Should Remove Content", func() {
				for _, test := range TestStrings {
					id, err := khepri.ParseID(test.id)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(repo.Test(test.t, id)).To(Equal(true))
					Expect(repo.Remove(test.t, id))
					Expect(repo.Test(test.t, id)).To(Equal(false))
				}
			})
		})
	})

})
