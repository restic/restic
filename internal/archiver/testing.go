package archiver

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

// TestSnapshot creates a new snapshot of path.
func TestSnapshot(t testing.TB, repo restic.Repository, path string, parent *restic.ID) *restic.Snapshot {
	arch := New(repo, fs.Local{}, Options{})
	opts := SnapshotOptions{
		Time:     time.Now(),
		Hostname: "localhost",
		Tags:     []string{"test"},
	}
	if parent != nil {
		sn, err := restic.LoadSnapshot(context.TODO(), arch.Repo, *parent)
		if err != nil {
			t.Fatal(err)
		}
		opts.ParentSnapshot = sn
	}
	sn, _, err := arch.Snapshot(context.TODO(), []string{path}, opts)
	if err != nil {
		t.Fatal(err)
	}
	return sn
}

// TestDir describes a directory structure to create for a test.
type TestDir map[string]interface{}

func (d TestDir) String() string {
	return "<Dir>"
}

// TestFile describes a file created for a test.
type TestFile struct {
	Content string
}

func (f TestFile) String() string {
	return "<File>"
}

// TestSymlink describes a symlink created for a test.
type TestSymlink struct {
	Target string
}

func (s TestSymlink) String() string {
	return "<Symlink>"
}

// TestCreateFiles creates a directory structure described by dir at target,
// which must already exist. On Windows, symlinks aren't created.
func TestCreateFiles(t testing.TB, target string, dir TestDir) {
	t.Helper()
	for name, item := range dir {
		targetPath := filepath.Join(target, name)

		switch it := item.(type) {
		case TestFile:
			err := os.WriteFile(targetPath, []byte(it.Content), 0644)
			if err != nil {
				t.Fatal(err)
			}
		case TestSymlink:
			err := fs.Symlink(filepath.FromSlash(it.Target), targetPath)
			if err != nil {
				t.Fatal(err)
			}
		case TestDir:
			err := fs.Mkdir(targetPath, 0755)
			if err != nil {
				t.Fatal(err)
			}

			TestCreateFiles(t, targetPath, it)
		}
	}
}

// TestWalkFunc is used by TestWalkFiles to traverse the dir. When an error is
// returned, traversal stops and the surrounding test is marked as failed.
type TestWalkFunc func(path string, item interface{}) error

// TestWalkFiles runs fn for each file/directory in dir, the filename will be
// constructed with target as the prefix. Symlinks on Windows are ignored.
func TestWalkFiles(t testing.TB, target string, dir TestDir, fn TestWalkFunc) {
	t.Helper()
	for name, item := range dir {
		targetPath := filepath.Join(target, name)

		err := fn(targetPath, item)
		if err != nil {
			t.Fatalf("TestWalkFunc returned error for %v: %v", targetPath, err)
			return
		}

		if dir, ok := item.(TestDir); ok {
			TestWalkFiles(t, targetPath, dir, fn)
		}
	}
}

// fixpath removes UNC paths (starting with `\\?`) on windows. On Linux, it's a noop.
func fixpath(item string) string {
	if runtime.GOOS != "windows" {
		return item
	}
	if strings.HasPrefix(item, `\\?`) {
		return item[4:]
	}
	return item
}

// TestEnsureFiles tests if the directory structure at target is the same as
// described in dir.
func TestEnsureFiles(t testing.TB, target string, dir TestDir) {
	t.Helper()
	pathsChecked := make(map[string]struct{})

	// first, test that all items are there
	TestWalkFiles(t, target, dir, func(path string, item interface{}) error {
		fi, err := fs.Lstat(path)
		if err != nil {
			return err
		}

		switch node := item.(type) {
		case TestDir:
			if !fi.IsDir() {
				t.Errorf("is not a directory: %v", path)
			}
			return nil
		case TestFile:
			if !fs.IsRegularFile(fi) {
				t.Errorf("is not a regular file: %v", path)
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			if string(content) != node.Content {
				t.Errorf("wrong content for %v, want %q, got %q", path, node.Content, content)
			}
		case TestSymlink:
			if fi.Mode()&os.ModeType != os.ModeSymlink {
				t.Errorf("is not a symlink: %v", path)
				return nil
			}

			target, err := fs.Readlink(path)
			if err != nil {
				return err
			}

			if target != node.Target {
				t.Errorf("wrong target for %v, want %v, got %v", path, node.Target, target)
			}
		}

		pathsChecked[path] = struct{}{}

		for parent := filepath.Dir(path); parent != target; parent = filepath.Dir(parent) {
			pathsChecked[parent] = struct{}{}
		}

		return nil
	})

	// then, traverse the directory again, looking for additional files
	err := fs.Walk(target, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		path = fixpath(path)

		if path == target {
			return nil
		}

		_, ok := pathsChecked[path]
		if !ok {
			t.Errorf("additional item found: %v %v", path, fi.Mode())
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestEnsureFileContent checks if the file in the repo is the same as file.
func TestEnsureFileContent(ctx context.Context, t testing.TB, repo restic.Repository, filename string, node *restic.Node, file TestFile) {
	if int(node.Size) != len(file.Content) {
		t.Fatalf("%v: wrong node size: want %d, got %d", filename, node.Size, len(file.Content))
		return
	}

	content := make([]byte, crypto.CiphertextLength(len(file.Content)))
	pos := 0
	for _, id := range node.Content {
		part, err := repo.LoadBlob(ctx, restic.DataBlob, id, content[pos:])
		if err != nil {
			t.Fatalf("error loading blob %v: %v", id.Str(), err)
			return
		}

		copy(content[pos:pos+len(part)], part)
		pos += len(part)
	}

	content = content[:pos]

	if string(content) != file.Content {
		t.Fatalf("%v: wrong content returned, want %q, got %q", filename, file.Content, content)
	}
}

// TestEnsureTree checks that the tree ID in the repo matches dir. On Windows,
// Symlinks are ignored.
func TestEnsureTree(ctx context.Context, t testing.TB, prefix string, repo restic.Repository, treeID restic.ID, dir TestDir) {
	t.Helper()

	tree, err := restic.LoadTree(ctx, repo, treeID)
	if err != nil {
		t.Fatal(err)
		return
	}

	var nodeNames []string
	for _, node := range tree.Nodes {
		nodeNames = append(nodeNames, node.Name)
	}
	debug.Log("%v (%v) %v", prefix, treeID.Str(), nodeNames)

	checked := make(map[string]struct{})
	for _, node := range tree.Nodes {
		nodePrefix := path.Join(prefix, node.Name)

		entry, ok := dir[node.Name]
		if !ok {
			t.Errorf("unexpected tree node %q found, want: %#v", node.Name, dir)
			return
		}

		checked[node.Name] = struct{}{}

		switch e := entry.(type) {
		case TestDir:
			if node.Type != "dir" {
				t.Errorf("tree node %v has wrong type %q, want %q", nodePrefix, node.Type, "dir")
				return
			}

			if node.Subtree == nil {
				t.Errorf("tree node %v has nil subtree", nodePrefix)
				return
			}

			TestEnsureTree(ctx, t, path.Join(prefix, node.Name), repo, *node.Subtree, e)
		case TestFile:
			if node.Type != "file" {
				t.Errorf("tree node %v has wrong type %q, want %q", nodePrefix, node.Type, "file")
			}
			TestEnsureFileContent(ctx, t, repo, nodePrefix, node, e)
		case TestSymlink:
			if node.Type != "symlink" {
				t.Errorf("tree node %v has wrong type %q, want %q", nodePrefix, node.Type, "file")
			}

			if e.Target != node.LinkTarget {
				t.Errorf("symlink %v has wrong target, want %q, got %q", nodePrefix, e.Target, node.LinkTarget)
			}
		}
	}

	for name := range dir {
		_, ok := checked[name]
		if !ok {
			t.Errorf("tree %v: expected node %q not found, has: %v", prefix, name, nodeNames)
		}
	}
}

// TestEnsureSnapshot tests if the snapshot in the repo has exactly the same
// structure as dir. On Windows, Symlinks are ignored.
func TestEnsureSnapshot(t testing.TB, repo restic.Repository, snapshotID restic.ID, dir TestDir) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sn, err := restic.LoadSnapshot(ctx, repo, snapshotID)
	if err != nil {
		t.Fatal(err)
		return
	}

	if sn.Tree == nil {
		t.Fatal("snapshot has nil tree ID")
		return
	}

	TestEnsureTree(ctx, t, "/", repo, *sn.Tree, dir)
}
