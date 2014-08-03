package khepri_test

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/fd0/khepri"
)

var testCleanup = flag.Bool("test.cleanup", true, "clean up after running tests (remove repository directory with all content)")

var TestStrings = []struct {
	id   string
	t    khepri.Type
	data string
}{
	{"c3ab8ff13720e8ad9047dd39466b3c8974e592c2fa383d4a3960714caef0c4f2", khepri.TYPE_BLOB, "foobar"},
	{"248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1", khepri.TYPE_BLOB, "abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq"},
	{"cc5d46bdb4991c6eae3eb739c9c8a7a46fe9654fab79c47b4fe48383b5b25e1c", khepri.TYPE_REF, "foo/bar"},
	{"4e54d2c721cbdb730f01b10b62dec622962b36966ec685880effa63d71c808f2", khepri.TYPE_BLOB, "foo/../../baz"},
}

func setupRepo() (*khepri.DirRepository, error) {
	tempdir, err := ioutil.TempDir("", "khepri-test-")
	if err != nil {
		return nil, err
	}

	repo, err := khepri.NewDirRepository(tempdir)
	if err != nil {
		return nil, err
	}

	return repo, nil
}

func teardownRepo(repo *khepri.DirRepository) error {
	if !*testCleanup {
		fmt.Fprintf(os.Stderr, "leaving repository at %s\n", repo.Path())
		return nil
	}

	err := os.RemoveAll(repo.Path())
	if err != nil {
		return err
	}

	return nil
}

func TestRepository(t *testing.T) {
	repo, err := setupRepo()
	ok(t, err)

	defer func() {
		err = teardownRepo(repo)
		ok(t, err)
	}()

	// detect non-existing files
	for _, test := range TestStrings {
		id, err := khepri.ParseID(test.id)
		ok(t, err)

		// try to get string out, should fail
		ret, err := repo.Test(test.t, id)
		ok(t, err)
		assert(t, !ret, fmt.Sprintf("id %q was found (but should not have)", test.id))
	}

	// add files
	for _, test := range TestStrings {
		// store string in repository
		id, err := repo.Put(test.t, strings.NewReader(test.data))
		ok(t, err)
		equals(t, test.id, id.String())

		// try to get it out again
		rd, err := repo.Get(test.t, id)
		ok(t, err)
		assert(t, rd != nil, "Get() returned nil reader")

		// compare content
		buf, err := ioutil.ReadAll(rd)
		equals(t, test.data, string(buf))
	}

	// add buffer
	for _, test := range TestStrings {
		// store buf in repository
		id, err := repo.PutRaw(test.t, []byte(test.data))
		ok(t, err)
		equals(t, test.id, id.String())
	}

	// list ids
	for _, tpe := range []khepri.Type{khepri.TYPE_BLOB, khepri.TYPE_REF} {
		IDs := khepri.IDs{}
		for _, test := range TestStrings {
			if test.t == tpe {
				id, err := khepri.ParseID(test.id)
				ok(t, err)
				IDs = append(IDs, id)
			}
		}

		ids, err := repo.ListIDs(tpe)
		ok(t, err)

		sort.Sort(ids)
		sort.Sort(IDs)
		equals(t, IDs, ids)
	}

	// remove content if requested
	if *testCleanup {
		for _, test := range TestStrings {
			id, err := khepri.ParseID(test.id)
			ok(t, err)

			found, err := repo.Test(test.t, id)
			ok(t, err)
			assert(t, found, fmt.Sprintf("id %q was not found before removal"))

			err = repo.Remove(test.t, id)
			ok(t, err)

			found, err = repo.Test(test.t, id)
			ok(t, err)
			assert(t, !found, fmt.Sprintf("id %q was not found before removal"))
		}
	}
}
