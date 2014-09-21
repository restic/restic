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
		id, err := repo.Create(khepri.TYPE_BLOB, []byte(test.data))
		ok(t, err)

		id2, err := khepri.ParseID(test.id)
		ok(t, err)

		equals(t, id2, id)
	}
}
