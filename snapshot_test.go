package restic_test

import (
	"testing"
	"time"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
)

func testSnapshot(t *testing.T, be backend.Server) {
	var err error
	sn := restic.NewSnapshot("/home/foobar")
	sn.Content, err = backend.ParseID("c3ab8ff13720e8ad9047dd39466b3c8974e592c2fa383d4a3960714caef0c4f2")
	ok(t, err)
	sn.Time, err = time.Parse(time.RFC3339Nano, "2014-08-03T17:49:05.378595539+02:00")
	ok(t, err)

	// _, err = sn.Save(be)
	// ok(t, err)
}

func TestSnapshot(t *testing.T) {
	repo := setupBackend(t)
	defer teardownBackend(t, repo)

	testSnapshot(t, repo)
}
