package restic

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"restic/backend"
	"restic/repository"
)

func loadIDSet(t testing.TB, filename string) backend.IDSet {
	f, err := os.Open(filename)
	if err != nil {
		t.Logf("unable to open golden file %v: %v", filename, err)
		return backend.IDSet{}
	}

	sc := bufio.NewScanner(f)

	ids := backend.NewIDSet()
	for sc.Scan() {
		id, err := backend.ParseID(sc.Text())
		if err != nil {
			t.Errorf("file %v contained invalid id: %v", filename, err)
		}

		ids.Insert(id)
	}

	if err = f.Close(); err != nil {
		t.Errorf("closing file %v failed with error %v", filename, err)
	}

	return ids
}

func saveIDSet(t testing.TB, filename string, s backend.IDSet) {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		t.Fatalf("unable to update golden file %v: %v", filename, err)
		return
	}

	var ids backend.IDs
	for id := range s {
		ids = append(ids, id)
	}

	sort.Sort(ids)
	for _, id := range ids {
		fmt.Fprintf(f, "%s\n", id)
	}

	if err = f.Close(); err != nil {
		t.Fatalf("close file %v returned error: %v", filename, err)
	}
}

var updateGoldenFiles = flag.Bool("update", false, "update golden files in testdata/")

const (
	testSnapshots = 3
	testDepth     = 2
)

var testTime = time.Unix(1469960361, 23)

func TestFindUsedBlobs(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	var snapshots []*Snapshot
	for i := 0; i < testSnapshots; i++ {
		sn := TestCreateSnapshot(t, repo, testTime.Add(time.Duration(i)*time.Second), testDepth)
		t.Logf("snapshot %v saved, tree %v", sn.ID().Str(), sn.Tree.Str())
		snapshots = append(snapshots, sn)
	}

	for i, sn := range snapshots {
		usedBlobs, err := FindUsedBlobs(repo, *sn.Tree)
		if err != nil {
			t.Errorf("FindUsedBlobs returned error: %v", err)
			continue
		}

		if len(usedBlobs) == 0 {
			t.Errorf("FindUsedBlobs returned an empty set")
			continue
		}

		goldenFilename := filepath.Join("testdata", fmt.Sprintf("used_blobs_snapshot%d", i))
		want := loadIDSet(t, goldenFilename)

		if !want.Equals(usedBlobs) {
			t.Errorf("snapshot %d: wrong list of blobs returned:\n  missing blobs: %v\n  extra blobs: %v",
				i, want.Sub(usedBlobs), usedBlobs.Sub(want))
		}

		if *updateGoldenFiles {
			saveIDSet(t, goldenFilename, usedBlobs)
		}
	}
}
