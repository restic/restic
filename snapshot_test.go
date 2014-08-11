package khepri_test

import (
	"testing"
	"time"

	"github.com/fd0/khepri"
)

func TestSnapshot(t *testing.T) {
	repo, err := setupRepo()
	ok(t, err)

	defer func() {
		err = teardownRepo(repo)
		ok(t, err)
	}()

	sn := khepri.NewSnapshot("/home/foobar")
	sn.Content, err = khepri.ParseID("c3ab8ff13720e8ad9047dd39466b3c8974e592c2fa383d4a3960714caef0c4f2")
	ok(t, err)
	sn.Time, err = time.Parse(time.RFC3339Nano, "2014-08-03T17:49:05.378595539+02:00")
	ok(t, err)

	_, err = sn.Save(repo)
	ok(t, err)
}
