package khepri_test

import (
	"testing"

	"github.com/fd0/khepri"
)

func TestObjects(t *testing.T) {
	repo, err := setupRepo()
	ok(t, err)

	defer func() {
		err = teardownRepo(repo)
		ok(t, err)
	}()

	for _, test := range TestStrings {
		obj, ch, err := repo.Create(khepri.TYPE_BLOB)
		ok(t, err)

		_, err = obj.Write([]byte(test.data))
		ok(t, err)

		err = obj.Close()
		ok(t, err)

		id, err := khepri.ParseID(test.id)
		ok(t, err)

		equals(t, id, <-ch)
	}
}
