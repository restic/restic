package restorer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

// saveUnorderedTree saves nodes in the given order without enforcing the
// name ordering that TreeJSONBuilder requires. This simulates trees written
// by clients that bypass restic's ordering checks.
func saveUnorderedTree(t testing.TB, saver restic.BlobSaver, nodes []*data.Node) restic.ID {
	t.Helper()
	ctx := context.Background()

	buf, err := json.Marshal(struct {
		Nodes []*data.Node `json:"nodes"`
	}{Nodes: nodes})
	if err != nil {
		t.Fatal(err)
	}

	id, _, _, err := saver.SaveBlob(ctx, restic.TreeBlob, buf, restic.ID{}, false)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func saveSnapshotWithTree(t testing.TB, repo restic.Repository, treeID restic.ID) *data.Snapshot {
	t.Helper()
	ctx := context.Background()

	sn, err := data.NewSnapshot([]string{"test"}, nil, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	sn.Tree = &treeID
	_, err = data.SaveSnapshot(ctx, repo, sn)
	if err != nil {
		t.Fatal(err)
	}
	return sn
}

// TestRestoreDuplicateNodeNames covers https://github.com/restic/restic/issues/22180:
// a tree containing a symlink `a` followed by a directory `a` must not let
// restore write through the symlink to files outside the target.
func TestRestoreDuplicateNodeNames(t *testing.T) {
	repo := repository.TestRepository(t)
	ctx := t.Context()

	outside := rtest.TempDir(t)
	rtest.OK(t, os.WriteFile(filepath.Join(outside, "victim"), []byte("precious"), 0644))
	rtest.OK(t, os.WriteFile(filepath.Join(outside, "victim2"), []byte("precious"), 0644))

	uid, gid := uint32(os.Getuid()), uint32(os.Getgid())
	var treeID restic.ID
	rtest.OK(t, repo.WithBlobUploader(ctx, func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		contentID := saveFile(t, uploader, "pwned")
		subtreeID := saveUnorderedTree(t, uploader, []*data.Node{
			{
				Type: data.NodeTypeFile, Name: "victim2", Mode: 0644,
				UID: uid, GID: gid, Inode: 100, DeviceID: 1, Links: 2,
				Size: 5, Content: restic.IDs{contentID},
			},
			{
				Type: data.NodeTypeSymlink, Name: "victim", Mode: os.ModeSymlink | 0777,
				UID: uid, GID: gid, Inode: 101, Links: 1, LinkTarget: "/nonexistent-pwned",
			},
		})
		treeID = saveUnorderedTree(t, uploader, []*data.Node{
			{
				Type: data.NodeTypeFile, Name: "0src", Mode: 0644,
				UID: uid, GID: gid, Inode: 100, DeviceID: 1, Links: 2,
				Size: 5, Content: restic.IDs{contentID},
			},
			{
				Type: data.NodeTypeSymlink, Name: "a", Mode: os.ModeSymlink | 0777,
				UID: uid, GID: gid, Inode: 102, Links: 1, LinkTarget: outside,
			},
			{
				Type: data.NodeTypeDir, Name: "a", Mode: 0755,
				UID: uid, GID: gid, Inode: 103, Links: 1, Subtree: &subtreeID,
			},
		})
		return nil
	}))

	sn := saveSnapshotWithTree(t, repo, treeID)
	target := filepath.Join(rtest.TempDir(t), "target")

	res := NewRestorer(repo, sn, Options{})
	_, err := res.RestoreTo(ctx, target)
	if !errors.Is(err, data.ErrTreeNotOrdered) {
		t.Fatalf("RestoreTo returned error %v, want an error wrapping ErrTreeNotOrdered", err)
	}

	// Nothing outside the restore target must have been touched.
	for _, name := range []string{"victim", "victim2"} {
		path := filepath.Join(outside, name)
		fi, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("%q was replaced by a symlink", path)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "precious" {
			t.Fatalf("%q contains %q, want %q", path, content, "precious")
		}
	}
}

// TestRestoreUnorderedNodeNames checks that trees whose names are not in
// strictly increasing order are rejected even without duplicate names.
func TestRestoreUnorderedNodeNames(t *testing.T) {
	repo := repository.TestRepository(t)
	ctx := t.Context()

	uid, gid := uint32(os.Getuid()), uint32(os.Getgid())
	var treeID restic.ID
	rtest.OK(t, repo.WithBlobUploader(ctx, func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		contentID := saveFile(t, uploader, "data")
		treeID = saveUnorderedTree(t, uploader, []*data.Node{
			{
				Type: data.NodeTypeFile, Name: "b", Mode: 0644,
				UID: uid, GID: gid, Inode: 100, Links: 1,
				Size: 4, Content: restic.IDs{contentID},
			},
			{
				Type: data.NodeTypeFile, Name: "a", Mode: 0644,
				UID: uid, GID: gid, Inode: 101, Links: 1,
				Size: 4, Content: restic.IDs{contentID},
			},
		})
		return nil
	}))

	sn := saveSnapshotWithTree(t, repo, treeID)
	target := filepath.Join(rtest.TempDir(t), "target")

	res := NewRestorer(repo, sn, Options{})
	_, err := res.RestoreTo(ctx, target)
	if !errors.Is(err, data.ErrTreeNotOrdered) {
		t.Fatalf("RestoreTo returned error %v, want an error wrapping ErrTreeNotOrdered", err)
	}
}
