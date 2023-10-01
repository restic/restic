package restic_test

import (
	"context"
	"strings"
	"testing"

	"github.com/restic/restic/internal/restic"
)

var samples = restic.IDs{
	restic.TestParseID("20bdc1402a6fc9b633aaffffffffffffffffffffffffffffffffffffffffffff"),
	restic.TestParseID("20bdc1402a6fc9b633ccd578c4a92d0f4ef1a457fa2e16c596bc73fb409d6cc0"),
	restic.TestParseID("20bdc1402a6fc9b633ffffffffffffffffffffffffffffffffffffffffffffff"),
	restic.TestParseID("20ff988befa5fc40350f00d531a767606efefe242c837aaccb80673f286be53d"),
	restic.TestParseID("326cb59dfe802304f96ee9b5b9af93bdee73a30f53981e5ec579aedb6f1d0f07"),
	restic.TestParseID("86b60b9594d1d429c4aa98fa9562082cabf53b98c7dc083abe5dae31074dd15a"),
	restic.TestParseID("96c8dbe225079e624b5ce509f5bd817d1453cd0a85d30d536d01b64a8669aeae"),
	restic.TestParseID("fa31d65b87affcd167b119e9d3d2a27b8236ca4836cb077ed3e96fcbe209b792"),
}

func TestFind(t *testing.T) {
	list := samples

	m := &ListHelper{}
	m.ListFn = func(ctx context.Context, t restic.FileType, fn func(id restic.ID, size int64) error) error {
		for _, id := range list {
			err := fn(id, 0)
			if err != nil {
				return err
			}
		}
		return nil
	}

	f, err := restic.Find(context.TODO(), m, restic.SnapshotFile, "20bdc1402a6fc9b633aa")
	if err != nil {
		t.Error(err)
	}
	expectedMatch := restic.TestParseID("20bdc1402a6fc9b633aaffffffffffffffffffffffffffffffffffffffffffff")
	if f != expectedMatch {
		t.Errorf("Wrong match returned want %s, got %s", expectedMatch, f)
	}

	f, err = restic.Find(context.TODO(), m, restic.SnapshotFile, "NotAPrefix")
	if _, ok := err.(*restic.NoIDByPrefixError); !ok || !strings.Contains(err.Error(), "NotAPrefix") {
		t.Error("Expected no snapshots to be found.")
	}
	if !f.IsNull() {
		t.Errorf("Find should not return a match on error.")
	}

	// Try to match with a prefix longer than any ID.
	extraLengthID := samples[0].String() + "f"
	f, err = restic.Find(context.TODO(), m, restic.SnapshotFile, extraLengthID)
	if _, ok := err.(*restic.NoIDByPrefixError); !ok || !strings.Contains(err.Error(), extraLengthID) {
		t.Errorf("Wrong error %v for no snapshots matched", err)
	}
	if !f.IsNull() {
		t.Errorf("Find should not return a match on error.")
	}

	// Use a prefix that will match the prefix of multiple Ids in `samples`.
	f, err = restic.Find(context.TODO(), m, restic.SnapshotFile, "20bdc140")
	if _, ok := err.(*restic.MultipleIDMatchesError); !ok {
		t.Errorf("Wrong error %v for multiple snapshots", err)
	}
	if !f.IsNull() {
		t.Errorf("Find should not return a match on error.")
	}
}
