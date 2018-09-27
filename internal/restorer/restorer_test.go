package restorer

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

type Node interface{}

type Snapshot struct {
	Nodes  map[string]Node
	treeID restic.ID
}

type File struct {
	Data  string
	Links uint64
	Inode uint64
}

type Dir struct {
	Nodes map[string]Node
	Mode  os.FileMode
}

func saveFile(t testing.TB, repo restic.Repository, node File) restic.ID {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	id, err := repo.SaveBlob(ctx, restic.DataBlob, []byte(node.Data), restic.ID{})
	if err != nil {
		t.Fatal(err)
	}

	return id
}

func saveDir(t testing.TB, repo restic.Repository, nodes map[string]Node, inode uint64) restic.ID {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tree := &restic.Tree{}
	for name, n := range nodes {
		inode++
		switch node := n.(type) {
		case File:
			fi := n.(File).Inode
			if fi == 0 {
				fi = inode
			}
			lc := n.(File).Links
			if lc == 0 {
				lc = 1
			}
			fc := []restic.ID{}
			if len(n.(File).Data) > 0 {
				fc = append(fc, saveFile(t, repo, node))
			}
			tree.Insert(&restic.Node{
				Type:    "file",
				Mode:    0644,
				Name:    name,
				UID:     uint32(os.Getuid()),
				GID:     uint32(os.Getgid()),
				Content: fc,
				Size:    uint64(len(n.(File).Data)),
				Inode:   fi,
				Links:   lc,
			})
		case Dir:
			id := saveDir(t, repo, node.Nodes, inode)

			mode := node.Mode
			if mode == 0 {
				mode = 0755
			}

			tree.Insert(&restic.Node{
				Type:    "dir",
				Mode:    mode,
				Name:    name,
				UID:     uint32(os.Getuid()),
				GID:     uint32(os.Getgid()),
				Subtree: &id,
			})
		default:
			t.Fatalf("unknown node type %T", node)
		}
	}

	id, err := repo.SaveTree(ctx, tree)
	if err != nil {
		t.Fatal(err)
	}

	return id
}

func saveSnapshot(t testing.TB, repo restic.Repository, snapshot Snapshot) (*restic.Snapshot, restic.ID) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	treeID := saveDir(t, repo, snapshot.Nodes, 1000)

	err := repo.Flush(ctx)
	if err != nil {
		t.Fatal(err)
	}

	err = repo.SaveIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}

	sn, err := restic.NewSnapshot([]string{"test"}, nil, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	sn.Tree = &treeID
	id, err := repo.SaveJSONUnpacked(ctx, restic.SnapshotFile, sn)
	if err != nil {
		t.Fatal(err)
	}

	return sn, id
}

// toSlash converts the OS specific path dir to a slash-separated path.
func toSlash(dir string) string {
	data := strings.Split(dir, string(filepath.Separator))
	return strings.Join(data, "/")
}

func TestRestorer(t *testing.T) {
	var tests = []struct {
		Snapshot
		Files      map[string]string
		ErrorsMust map[string]map[string]struct{}
		ErrorsMay  map[string]map[string]struct{}
		Select     func(item string, dstpath string, node *restic.Node) (selectForRestore bool, childMayBeSelected bool)
	}{
		// valid test cases
		{
			Snapshot: Snapshot{
				Nodes: map[string]Node{
					"foo": File{Data: "content: foo\n"},
					"dirtest": Dir{
						Nodes: map[string]Node{
							"file": File{Data: "content: file\n"},
						},
					},
				},
			},
			Files: map[string]string{
				"foo":          "content: foo\n",
				"dirtest/file": "content: file\n",
			},
		},
		{
			Snapshot: Snapshot{
				Nodes: map[string]Node{
					"top": File{Data: "toplevel file"},
					"dir": Dir{
						Nodes: map[string]Node{
							"file": File{Data: "file in dir"},
							"subdir": Dir{
								Nodes: map[string]Node{
									"file": File{Data: "file in subdir"},
								},
							},
						},
					},
				},
			},
			Files: map[string]string{
				"top":             "toplevel file",
				"dir/file":        "file in dir",
				"dir/subdir/file": "file in subdir",
			},
		},
		{
			Snapshot: Snapshot{
				Nodes: map[string]Node{
					"dir": Dir{
						Mode: 0444,
					},
					"file": File{Data: "top-level file"},
				},
			},
			Files: map[string]string{
				"file": "top-level file",
			},
		},
		{
			Snapshot: Snapshot{
				Nodes: map[string]Node{
					"dir": Dir{
						Mode: 0555,
						Nodes: map[string]Node{
							"file": File{Data: "file in dir"},
						},
					},
				},
			},
			Files: map[string]string{
				"dir/file": "file in dir",
			},
		},
		{
			Snapshot: Snapshot{
				Nodes: map[string]Node{
					"topfile": File{Data: "top-level file"},
				},
			},
			Files: map[string]string{
				"topfile": "top-level file",
			},
		},
		{
			Snapshot: Snapshot{
				Nodes: map[string]Node{
					"dir": Dir{
						Nodes: map[string]Node{
							"file": File{Data: "content: file\n"},
						},
					},
				},
			},
			Files: map[string]string{
				"dir/file": "content: file\n",
			},
			Select: func(item, dstpath string, node *restic.Node) (selectedForRestore bool, childMayBeSelected bool) {
				switch item {
				case filepath.FromSlash("/dir"):
					childMayBeSelected = true
				case filepath.FromSlash("/dir/file"):
					selectedForRestore = true
					childMayBeSelected = true
				}

				return selectedForRestore, childMayBeSelected
			},
		},

		// test cases with invalid/constructed names
		{
			Snapshot: Snapshot{
				Nodes: map[string]Node{
					`..\test`:                      File{Data: "foo\n"},
					`..\..\foo\..\bar\..\xx\test2`: File{Data: "test2\n"},
				},
			},
			ErrorsMay: map[string]map[string]struct{}{
				`/`: {
					`invalid child node name ..\test`:                      struct{}{},
					`invalid child node name ..\..\foo\..\bar\..\xx\test2`: struct{}{},
				},
			},
		},
		{
			Snapshot: Snapshot{
				Nodes: map[string]Node{
					`../test`:                      File{Data: "foo\n"},
					`../../foo/../bar/../xx/test2`: File{Data: "test2\n"},
				},
			},
			ErrorsMay: map[string]map[string]struct{}{
				`/`: {
					`invalid child node name ../test`:                      struct{}{},
					`invalid child node name ../../foo/../bar/../xx/test2`: struct{}{},
				},
			},
		},
		{
			Snapshot: Snapshot{
				Nodes: map[string]Node{
					"top": File{Data: "toplevel file"},
					"x": Dir{
						Nodes: map[string]Node{
							"file1": File{Data: "file1"},
							"..": Dir{
								Nodes: map[string]Node{
									"file2": File{Data: "file2"},
									"..": Dir{
										Nodes: map[string]Node{
											"file2": File{Data: "file2"},
										},
									},
								},
							},
						},
					},
				},
			},
			Files: map[string]string{
				"top": "toplevel file",
			},
			ErrorsMust: map[string]map[string]struct{}{
				`/x`: {
					`invalid child node name ..`: struct{}{},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			repo, cleanup := repository.TestRepository(t)
			defer cleanup()
			_, id := saveSnapshot(t, repo, test.Snapshot)
			t.Logf("snapshot saved as %v", id.Str())

			res, err := NewRestorer(repo, id)
			if err != nil {
				t.Fatal(err)
			}

			tempdir, cleanup := rtest.TempDir(t)
			defer cleanup()

			// make sure we're creating a new subdir of the tempdir
			tempdir = filepath.Join(tempdir, "target")

			res.SelectFilter = func(item, dstpath string, node *restic.Node) (selectedForRestore bool, childMayBeSelected bool) {
				t.Logf("restore %v to %v", item, dstpath)
				if !fs.HasPathPrefix(tempdir, dstpath) {
					t.Errorf("would restore %v to %v, which is not within the target dir %v",
						item, dstpath, tempdir)
					return false, false
				}

				if test.Select != nil {
					return test.Select(item, dstpath, node)
				}

				return true, true
			}

			errors := make(map[string]map[string]struct{})
			res.Error = func(location string, err error) error {
				location = toSlash(location)
				t.Logf("restore returned error for %q: %v", location, err)
				if errors[location] == nil {
					errors[location] = make(map[string]struct{})
				}
				errors[location][err.Error()] = struct{}{}
				return nil
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err = res.RestoreTo(ctx, tempdir)
			if err != nil {
				t.Fatal(err)
			}

			for location, expectedErrors := range test.ErrorsMust {
				actualErrors, ok := errors[location]
				if !ok {
					t.Errorf("expected error(s) for %v, found none", location)
					continue
				}

				rtest.Equals(t, expectedErrors, actualErrors)

				delete(errors, location)
			}

			for location, expectedErrors := range test.ErrorsMay {
				actualErrors, ok := errors[location]
				if !ok {
					continue
				}

				rtest.Equals(t, expectedErrors, actualErrors)

				delete(errors, location)
			}

			for filename, err := range errors {
				t.Errorf("unexpected error for %v found: %v", filename, err)
			}

			for filename, content := range test.Files {
				data, err := ioutil.ReadFile(filepath.Join(tempdir, filepath.FromSlash(filename)))
				if err != nil {
					t.Errorf("unable to read file %v: %v", filename, err)
					continue
				}

				if !bytes.Equal(data, []byte(content)) {
					t.Errorf("file %v has wrong content: want %q, got %q", filename, content, data)
				}
			}
		})
	}
}

func TestRestorerRelative(t *testing.T) {
	var tests = []struct {
		Snapshot
		Files map[string]string
	}{
		{
			Snapshot: Snapshot{
				Nodes: map[string]Node{
					"foo": File{Data: "content: foo\n"},
					"dirtest": Dir{
						Nodes: map[string]Node{
							"file": File{Data: "content: file\n"},
						},
					},
				},
			},
			Files: map[string]string{
				"foo":          "content: foo\n",
				"dirtest/file": "content: file\n",
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			repo, cleanup := repository.TestRepository(t)
			defer cleanup()

			_, id := saveSnapshot(t, repo, test.Snapshot)
			t.Logf("snapshot saved as %v", id.Str())

			res, err := NewRestorer(repo, id)
			if err != nil {
				t.Fatal(err)
			}

			tempdir, cleanup := rtest.TempDir(t)
			defer cleanup()

			cleanup = fs.TestChdir(t, tempdir)
			defer cleanup()

			errors := make(map[string]string)
			res.Error = func(location string, err error) error {
				t.Logf("restore returned error for %q: %v", location, err)
				errors[location] = err.Error()
				return nil
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err = res.RestoreTo(ctx, "restore")
			if err != nil {
				t.Fatal(err)
			}

			for filename, err := range errors {
				t.Errorf("unexpected error for %v found: %v", filename, err)
			}

			for filename, content := range test.Files {
				data, err := ioutil.ReadFile(filepath.Join(tempdir, "restore", filepath.FromSlash(filename)))
				if err != nil {
					t.Errorf("unable to read file %v: %v", filename, err)
					continue
				}

				if !bytes.Equal(data, []byte(content)) {
					t.Errorf("file %v has wrong content: want %q, got %q", filename, content, data)
				}
			}
		})
	}
}

type TraverseTreeCheck func(testing.TB) treeVisitor

type TreeVisit struct {
	funcName string // name of the function
	location string // location passed to the function
}

func checkVisitOrder(list []TreeVisit) TraverseTreeCheck {
	var pos int

	return func(t testing.TB) treeVisitor {
		check := func(funcName string) func(*restic.Node, string, string) error {
			return func(node *restic.Node, target, location string) error {
				if pos >= len(list) {
					t.Errorf("step %v, %v(%v): expected no more than %d function calls", pos, funcName, location, len(list))
					pos++
					return nil
				}

				v := list[pos]

				if v.funcName != funcName {
					t.Errorf("step %v, location %v: want function %v, but %v was called",
						pos, location, v.funcName, funcName)
				}

				if location != filepath.FromSlash(v.location) {
					t.Errorf("step %v: want location %v, got %v", pos, list[pos].location, location)
				}

				pos++
				return nil
			}
		}

		return treeVisitor{
			enterDir:  check("enterDir"),
			visitNode: check("visitNode"),
			leaveDir:  check("leaveDir"),
		}
	}
}

func TestRestorerTraverseTree(t *testing.T) {
	var tests = []struct {
		Snapshot
		Select  func(item string, dstpath string, node *restic.Node) (selectForRestore bool, childMayBeSelected bool)
		Visitor TraverseTreeCheck
	}{
		{
			// select everything
			Snapshot: Snapshot{
				Nodes: map[string]Node{
					"dir": Dir{Nodes: map[string]Node{
						"otherfile": File{Data: "x"},
						"subdir": Dir{Nodes: map[string]Node{
							"file": File{Data: "content: file\n"},
						}},
					}},
					"foo": File{Data: "content: foo\n"},
				},
			},
			Select: func(item string, dstpath string, node *restic.Node) (selectForRestore bool, childMayBeSelected bool) {
				return true, true
			},
			Visitor: checkVisitOrder([]TreeVisit{
				{"enterDir", "/dir"},
				{"visitNode", "/dir/otherfile"},
				{"enterDir", "/dir/subdir"},
				{"visitNode", "/dir/subdir/file"},
				{"leaveDir", "/dir/subdir"},
				{"leaveDir", "/dir"},
				{"visitNode", "/foo"},
			}),
		},

		// select only the top-level file
		{
			Snapshot: Snapshot{
				Nodes: map[string]Node{
					"dir": Dir{Nodes: map[string]Node{
						"otherfile": File{Data: "x"},
						"subdir": Dir{Nodes: map[string]Node{
							"file": File{Data: "content: file\n"},
						}},
					}},
					"foo": File{Data: "content: foo\n"},
				},
			},
			Select: func(item string, dstpath string, node *restic.Node) (selectForRestore bool, childMayBeSelected bool) {
				if item == "/foo" {
					return true, false
				}
				return false, false
			},
			Visitor: checkVisitOrder([]TreeVisit{
				{"visitNode", "/foo"},
			}),
		},
		{
			Snapshot: Snapshot{
				Nodes: map[string]Node{
					"aaa": File{Data: "content: foo\n"},
					"dir": Dir{Nodes: map[string]Node{
						"otherfile": File{Data: "x"},
						"subdir": Dir{Nodes: map[string]Node{
							"file": File{Data: "content: file\n"},
						}},
					}},
				},
			},
			Select: func(item string, dstpath string, node *restic.Node) (selectForRestore bool, childMayBeSelected bool) {
				if item == "/aaa" {
					return true, false
				}
				return false, false
			},
			Visitor: checkVisitOrder([]TreeVisit{
				{"visitNode", "/aaa"},
			}),
		},

		// select dir/
		{
			Snapshot: Snapshot{
				Nodes: map[string]Node{
					"dir": Dir{Nodes: map[string]Node{
						"otherfile": File{Data: "x"},
						"subdir": Dir{Nodes: map[string]Node{
							"file": File{Data: "content: file\n"},
						}},
					}},
					"foo": File{Data: "content: foo\n"},
				},
			},
			Select: func(item string, dstpath string, node *restic.Node) (selectForRestore bool, childMayBeSelected bool) {
				if strings.HasPrefix(item, "/dir") {
					return true, true
				}
				return false, false
			},
			Visitor: checkVisitOrder([]TreeVisit{
				{"enterDir", "/dir"},
				{"visitNode", "/dir/otherfile"},
				{"enterDir", "/dir/subdir"},
				{"visitNode", "/dir/subdir/file"},
				{"leaveDir", "/dir/subdir"},
				{"leaveDir", "/dir"},
			}),
		},

		// select only dir/otherfile
		{
			Snapshot: Snapshot{
				Nodes: map[string]Node{
					"dir": Dir{Nodes: map[string]Node{
						"otherfile": File{Data: "x"},
						"subdir": Dir{Nodes: map[string]Node{
							"file": File{Data: "content: file\n"},
						}},
					}},
					"foo": File{Data: "content: foo\n"},
				},
			},
			Select: func(item string, dstpath string, node *restic.Node) (selectForRestore bool, childMayBeSelected bool) {
				switch item {
				case "/dir":
					return false, true
				case "/dir/otherfile":
					return true, false
				default:
					return false, false
				}
			},
			Visitor: checkVisitOrder([]TreeVisit{
				{"visitNode", "/dir/otherfile"},
			}),
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			repo, cleanup := repository.TestRepository(t)
			defer cleanup()
			sn, id := saveSnapshot(t, repo, test.Snapshot)

			res, err := NewRestorer(repo, id)
			if err != nil {
				t.Fatal(err)
			}

			res.SelectFilter = test.Select

			tempdir, cleanup := rtest.TempDir(t)
			defer cleanup()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// make sure we're creating a new subdir of the tempdir
			target := filepath.Join(tempdir, "target")

			err = res.traverseTree(ctx, target, string(filepath.Separator), *sn.Tree, test.Visitor(t))
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
