package restorer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	restoreui "github.com/restic/restic/internal/ui/restore"
	"golang.org/x/sync/errgroup"
)

type Node interface{}

type Snapshot struct {
	Nodes map[string]Node
}

type File struct {
	Data       string
	DataParts  []string
	Links      uint64
	Inode      uint64
	Mode       os.FileMode
	ModTime    time.Time
	attributes *FileAttributes
}

type Symlink struct {
	Target  string
	ModTime time.Time
}

type Dir struct {
	Nodes      map[string]Node
	Mode       os.FileMode
	ModTime    time.Time
	attributes *FileAttributes
}

type FileAttributes struct {
	ReadOnly  bool
	Hidden    bool
	System    bool
	Archive   bool
	Encrypted bool
}

func saveFile(t testing.TB, repo restic.BlobSaver, data string) restic.ID {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	id, _, _, err := repo.SaveBlob(ctx, restic.DataBlob, []byte(data), restic.ID{}, false)
	if err != nil {
		t.Fatal(err)
	}

	return id
}

func saveDir(t testing.TB, repo restic.BlobSaver, nodes map[string]Node, inode uint64, getGenericAttributes func(attr *FileAttributes, isDir bool) (genericAttributes map[restic.GenericAttributeType]json.RawMessage)) restic.ID {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tree := &restic.Tree{}
	for name, n := range nodes {
		inode++
		switch node := n.(type) {
		case File:
			fi := node.Inode
			if fi == 0 {
				fi = inode
			}
			lc := node.Links
			if lc == 0 {
				lc = 1
			}
			fc := []restic.ID{}
			size := 0
			if len(node.Data) > 0 {
				size = len(node.Data)
				fc = append(fc, saveFile(t, repo, node.Data))
			} else if len(node.DataParts) > 0 {
				for _, part := range node.DataParts {
					fc = append(fc, saveFile(t, repo, part))
					size += len(part)
				}
			}
			mode := node.Mode
			if mode == 0 {
				mode = 0644
			}
			err := tree.Insert(&restic.Node{
				Type:              "file",
				Mode:              mode,
				ModTime:           node.ModTime,
				Name:              name,
				UID:               uint32(os.Getuid()),
				GID:               uint32(os.Getgid()),
				Content:           fc,
				Size:              uint64(size),
				Inode:             fi,
				Links:             lc,
				GenericAttributes: getGenericAttributes(node.attributes, false),
			})
			rtest.OK(t, err)
		case Symlink:
			err := tree.Insert(&restic.Node{
				Type:       "symlink",
				Mode:       os.ModeSymlink | 0o777,
				ModTime:    node.ModTime,
				Name:       name,
				UID:        uint32(os.Getuid()),
				GID:        uint32(os.Getgid()),
				LinkTarget: node.Target,
				Inode:      inode,
				Links:      1,
			})
			rtest.OK(t, err)
		case Dir:
			id := saveDir(t, repo, node.Nodes, inode, getGenericAttributes)

			mode := node.Mode
			if mode == 0 {
				mode = 0755
			}

			err := tree.Insert(&restic.Node{
				Type:              "dir",
				Mode:              mode,
				ModTime:           node.ModTime,
				Name:              name,
				UID:               uint32(os.Getuid()),
				GID:               uint32(os.Getgid()),
				Subtree:           &id,
				GenericAttributes: getGenericAttributes(node.attributes, false),
			})
			rtest.OK(t, err)
		default:
			t.Fatalf("unknown node type %T", node)
		}
	}

	id, err := restic.SaveTree(ctx, repo, tree)
	if err != nil {
		t.Fatal(err)
	}

	return id
}

func saveSnapshot(t testing.TB, repo restic.Repository, snapshot Snapshot, getGenericAttributes func(attr *FileAttributes, isDir bool) (genericAttributes map[restic.GenericAttributeType]json.RawMessage)) (*restic.Snapshot, restic.ID) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg, wgCtx := errgroup.WithContext(ctx)
	repo.StartPackUploader(wgCtx, wg)
	treeID := saveDir(t, repo, snapshot.Nodes, 1000, getGenericAttributes)
	err := repo.Flush(ctx)
	if err != nil {
		t.Fatal(err)
	}

	sn, err := restic.NewSnapshot([]string{"test"}, nil, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	sn.Tree = &treeID
	id, err := restic.SaveSnapshot(ctx, repo, sn)
	if err != nil {
		t.Fatal(err)
	}

	return sn, id
}

var noopGetGenericAttributes = func(attr *FileAttributes, isDir bool) (genericAttributes map[restic.GenericAttributeType]json.RawMessage) {
	// No-op
	return nil
}

func TestRestorer(t *testing.T) {
	var tests = []struct {
		Snapshot
		Files      map[string]string
		ErrorsMust map[string]map[string]struct{}
		ErrorsMay  map[string]map[string]struct{}
		Select     func(item string, isDir bool) (selectForRestore bool, childMayBeSelected bool)
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
			Select: func(item string, isDir bool) (selectedForRestore bool, childMayBeSelected bool) {
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
			repo := repository.TestRepository(t)
			sn, id := saveSnapshot(t, repo, test.Snapshot, noopGetGenericAttributes)
			t.Logf("snapshot saved as %v", id.Str())

			res := NewRestorer(repo, sn, Options{})

			tempdir := rtest.TempDir(t)
			// make sure we're creating a new subdir of the tempdir
			tempdir = filepath.Join(tempdir, "target")

			res.SelectFilter = func(item string, isDir bool) (selectedForRestore bool, childMayBeSelected bool) {
				t.Logf("restore %v", item)
				if test.Select != nil {
					return test.Select(item, isDir)
				}

				return true, true
			}

			errors := make(map[string]map[string]struct{})
			res.Error = func(location string, err error) error {
				location = filepath.ToSlash(location)
				t.Logf("restore returned error for %q: %v", location, err)
				if errors[location] == nil {
					errors[location] = make(map[string]struct{})
				}
				errors[location][err.Error()] = struct{}{}
				return nil
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := res.RestoreTo(ctx, tempdir)
			if err != nil {
				t.Fatal(err)
			}

			if len(test.ErrorsMust)+len(test.ErrorsMay) == 0 {
				_, err = res.VerifyFiles(ctx, tempdir)
				rtest.OK(t, err)
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
				data, err := os.ReadFile(filepath.Join(tempdir, filepath.FromSlash(filename)))
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
			repo := repository.TestRepository(t)

			sn, id := saveSnapshot(t, repo, test.Snapshot, noopGetGenericAttributes)
			t.Logf("snapshot saved as %v", id.Str())

			res := NewRestorer(repo, sn, Options{})

			tempdir := rtest.TempDir(t)
			cleanup := rtest.Chdir(t, tempdir)
			defer cleanup()

			errors := make(map[string]string)
			res.Error = func(location string, err error) error {
				t.Logf("restore returned error for %q: %v", location, err)
				errors[location] = err.Error()
				return nil
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := res.RestoreTo(ctx, "restore")
			if err != nil {
				t.Fatal(err)
			}
			nverified, err := res.VerifyFiles(ctx, "restore")
			rtest.OK(t, err)
			rtest.Equals(t, len(test.Files), nverified)

			for filename, err := range errors {
				t.Errorf("unexpected error for %v found: %v", filename, err)
			}

			for filename, content := range test.Files {
				data, err := os.ReadFile(filepath.Join(tempdir, "restore", filepath.FromSlash(filename)))
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
	funcName string   // name of the function
	location string   // location passed to the function
	files    []string // file list passed to the function
}

func checkVisitOrder(list []TreeVisit) TraverseTreeCheck {
	var pos int

	return func(t testing.TB) treeVisitor {
		check := func(funcName string) func(*restic.Node, string, string, []string) error {
			return func(node *restic.Node, target, location string, expectedFilenames []string) error {
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

				if !reflect.DeepEqual(expectedFilenames, v.files) {
					t.Errorf("step %v: want files %v, got %v", pos, list[pos].files, expectedFilenames)
				}

				pos++
				return nil
			}
		}
		checkNoFilename := func(funcName string) func(*restic.Node, string, string) error {
			f := check(funcName)
			return func(node *restic.Node, target, location string) error {
				return f(node, target, location, nil)
			}
		}

		return treeVisitor{
			enterDir:  checkNoFilename("enterDir"),
			visitNode: checkNoFilename("visitNode"),
			leaveDir:  check("leaveDir"),
		}
	}
}

func TestRestorerTraverseTree(t *testing.T) {
	var tests = []struct {
		Snapshot
		Select  func(item string, isDir bool) (selectForRestore bool, childMayBeSelected bool)
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
			Select: func(item string, isDir bool) (selectForRestore bool, childMayBeSelected bool) {
				return true, true
			},
			Visitor: checkVisitOrder([]TreeVisit{
				{"enterDir", "/", nil},
				{"enterDir", "/dir", nil},
				{"visitNode", "/dir/otherfile", nil},
				{"enterDir", "/dir/subdir", nil},
				{"visitNode", "/dir/subdir/file", nil},
				{"leaveDir", "/dir/subdir", []string{"file"}},
				{"leaveDir", "/dir", []string{"otherfile", "subdir"}},
				{"visitNode", "/foo", nil},
				{"leaveDir", "/", []string{"dir", "foo"}},
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
			Select: func(item string, isDir bool) (selectForRestore bool, childMayBeSelected bool) {
				if item == "/foo" {
					return true, false
				}
				return false, false
			},
			Visitor: checkVisitOrder([]TreeVisit{
				{"enterDir", "/", nil},
				{"visitNode", "/foo", nil},
				{"leaveDir", "/", []string{"dir", "foo"}},
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
			Select: func(item string, isDir bool) (selectForRestore bool, childMayBeSelected bool) {
				if item == "/aaa" {
					return true, false
				}
				return false, false
			},
			Visitor: checkVisitOrder([]TreeVisit{
				{"enterDir", "/", nil},
				{"visitNode", "/aaa", nil},
				{"leaveDir", "/", []string{"aaa", "dir"}},
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
			Select: func(item string, isDir bool) (selectForRestore bool, childMayBeSelected bool) {
				if strings.HasPrefix(item, "/dir") {
					return true, true
				}
				return false, false
			},
			Visitor: checkVisitOrder([]TreeVisit{
				{"enterDir", "/", nil},
				{"enterDir", "/dir", nil},
				{"visitNode", "/dir/otherfile", nil},
				{"enterDir", "/dir/subdir", nil},
				{"visitNode", "/dir/subdir/file", nil},
				{"leaveDir", "/dir/subdir", []string{"file"}},
				{"leaveDir", "/dir", []string{"otherfile", "subdir"}},
				{"leaveDir", "/", []string{"dir", "foo"}},
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
			Select: func(item string, isDir bool) (selectForRestore bool, childMayBeSelected bool) {
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
				{"enterDir", "/", nil},
				{"visitNode", "/dir/otherfile", nil},
				{"leaveDir", "/dir", []string{"otherfile", "subdir"}},
				{"leaveDir", "/", []string{"dir", "foo"}},
			}),
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			repo := repository.TestRepository(t)
			sn, _ := saveSnapshot(t, repo, test.Snapshot, noopGetGenericAttributes)

			// set Delete option to enable tracking filenames in a directory
			res := NewRestorer(repo, sn, Options{Delete: true})

			res.SelectFilter = test.Select

			tempdir := rtest.TempDir(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// make sure we're creating a new subdir of the tempdir
			target := filepath.Join(tempdir, "target")

			err := res.traverseTree(ctx, target, *sn.Tree, test.Visitor(t))
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func normalizeFileMode(mode os.FileMode) os.FileMode {
	if runtime.GOOS == "windows" {
		if mode.IsDir() {
			return 0555 | os.ModeDir
		}
		return os.FileMode(0444)
	}
	return mode
}

func checkConsistentInfo(t testing.TB, file string, fi os.FileInfo, modtime time.Time, mode os.FileMode) {
	if fi.Mode() != mode {
		t.Errorf("checking %q, Mode() returned wrong value, want 0%o, got 0%o", file, mode, fi.Mode())
	}

	if !fi.ModTime().Equal(modtime) {
		t.Errorf("checking %s, ModTime() returned wrong value, want %v, got %v", file, modtime, fi.ModTime())
	}
}

// test inspired from test case https://github.com/restic/restic/issues/1212
func TestRestorerConsistentTimestampsAndPermissions(t *testing.T) {
	timeForTest := time.Date(2019, time.January, 9, 1, 46, 40, 0, time.UTC)

	repo := repository.TestRepository(t)

	sn, _ := saveSnapshot(t, repo, Snapshot{
		Nodes: map[string]Node{
			"dir": Dir{
				Mode:    normalizeFileMode(0750 | os.ModeDir),
				ModTime: timeForTest,
				Nodes: map[string]Node{
					"file1": File{
						Mode:    normalizeFileMode(os.FileMode(0700)),
						ModTime: timeForTest,
						Data:    "content: file\n",
					},
					"anotherfile": File{
						Data: "content: file\n",
					},
					"subdir": Dir{
						Mode:    normalizeFileMode(0700 | os.ModeDir),
						ModTime: timeForTest,
						Nodes: map[string]Node{
							"file2": File{
								Mode:    normalizeFileMode(os.FileMode(0666)),
								ModTime: timeForTest,
								Links:   2,
								Inode:   1,
							},
						},
					},
				},
			},
		},
	}, noopGetGenericAttributes)

	res := NewRestorer(repo, sn, Options{})

	res.SelectFilter = func(item string, isDir bool) (selectedForRestore bool, childMayBeSelected bool) {
		switch filepath.ToSlash(item) {
		case "/dir":
			childMayBeSelected = true
		case "/dir/file1":
			selectedForRestore = true
			childMayBeSelected = false
		case "/dir/subdir":
			selectedForRestore = true
			childMayBeSelected = true
		case "/dir/subdir/file2":
			selectedForRestore = true
			childMayBeSelected = false
		}
		return selectedForRestore, childMayBeSelected
	}

	tempdir := rtest.TempDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := res.RestoreTo(ctx, tempdir)
	rtest.OK(t, err)

	var testPatterns = []struct {
		path    string
		modtime time.Time
		mode    os.FileMode
	}{
		{"dir", timeForTest, normalizeFileMode(0750 | os.ModeDir)},
		{filepath.Join("dir", "file1"), timeForTest, normalizeFileMode(os.FileMode(0700))},
		{filepath.Join("dir", "subdir"), timeForTest, normalizeFileMode(0700 | os.ModeDir)},
		{filepath.Join("dir", "subdir", "file2"), timeForTest, normalizeFileMode(os.FileMode(0666))},
	}

	for _, test := range testPatterns {
		f, err := os.Stat(filepath.Join(tempdir, test.path))
		rtest.OK(t, err)
		checkConsistentInfo(t, test.path, f, test.modtime, test.mode)
	}
}

// VerifyFiles must not report cancellation of its context through res.Error.
func TestVerifyCancel(t *testing.T) {
	snapshot := Snapshot{
		Nodes: map[string]Node{
			"foo": File{Data: "content: foo\n"},
		},
	}

	repo := repository.TestRepository(t)
	sn, _ := saveSnapshot(t, repo, snapshot, noopGetGenericAttributes)

	res := NewRestorer(repo, sn, Options{})

	tempdir := rtest.TempDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rtest.OK(t, res.RestoreTo(ctx, tempdir))
	err := os.WriteFile(filepath.Join(tempdir, "foo"), []byte("bar"), 0644)
	rtest.OK(t, err)

	var errs []error
	res.Error = func(filename string, err error) error {
		errs = append(errs, err)
		return err
	}

	nverified, err := res.VerifyFiles(ctx, tempdir)
	rtest.Equals(t, 0, nverified)
	rtest.Assert(t, err != nil, "nil error from VerifyFiles")
	rtest.Equals(t, 1, len(errs))
	rtest.Assert(t, strings.Contains(errs[0].Error(), "Invalid file size for"), "wrong error %q", errs[0].Error())
}

func TestRestorerSparseFiles(t *testing.T) {
	repo := repository.TestRepository(t)

	var zeros [1<<20 + 13]byte

	target := &fs.Reader{
		Mode:       0600,
		Name:       "/zeros",
		ReadCloser: io.NopCloser(bytes.NewReader(zeros[:])),
	}
	sc := archiver.NewScanner(target)
	err := sc.Scan(context.TODO(), []string{"/zeros"})
	rtest.OK(t, err)

	arch := archiver.New(repo, target, archiver.Options{})
	sn, _, _, err := arch.Snapshot(context.Background(), []string{"/zeros"},
		archiver.SnapshotOptions{})
	rtest.OK(t, err)

	res := NewRestorer(repo, sn, Options{Sparse: true})

	tempdir := rtest.TempDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = res.RestoreTo(ctx, tempdir)
	rtest.OK(t, err)

	filename := filepath.Join(tempdir, "zeros")
	content, err := os.ReadFile(filename)
	rtest.OK(t, err)

	rtest.Equals(t, len(zeros[:]), len(content))
	rtest.Equals(t, zeros[:], content)

	blocks := getBlockCount(t, filename)
	if blocks < 0 {
		return
	}

	// st.Blocks is the size in 512-byte blocks.
	denseBlocks := math.Ceil(float64(len(zeros)) / 512)
	sparsity := 1 - float64(blocks)/denseBlocks

	// This should report 100% sparse. We don't assert that,
	// as the behavior of sparse writes depends on the underlying
	// file system as well as the OS.
	t.Logf("wrote %d zeros as %d blocks, %.1f%% sparse",
		len(zeros), blocks, 100*sparsity)
}

func saveSnapshotsAndOverwrite(t *testing.T, baseSnapshot Snapshot, overwriteSnapshot Snapshot, baseOptions, overwriteOptions Options) string {
	repo := repository.TestRepository(t)
	tempdir := filepath.Join(rtest.TempDir(t), "target")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// base snapshot
	sn, id := saveSnapshot(t, repo, baseSnapshot, noopGetGenericAttributes)
	t.Logf("base snapshot saved as %v", id.Str())

	res := NewRestorer(repo, sn, baseOptions)
	rtest.OK(t, res.RestoreTo(ctx, tempdir))

	// overwrite snapshot
	sn, id = saveSnapshot(t, repo, overwriteSnapshot, noopGetGenericAttributes)
	t.Logf("overwrite snapshot saved as %v", id.Str())
	res = NewRestorer(repo, sn, overwriteOptions)
	rtest.OK(t, res.RestoreTo(ctx, tempdir))

	_, err := res.VerifyFiles(ctx, tempdir)
	rtest.OK(t, err)

	return tempdir
}

func TestRestorerSparseOverwrite(t *testing.T) {
	baseSnapshot := Snapshot{
		Nodes: map[string]Node{
			"foo": File{Data: "content: new\n"},
		},
	}
	var zero [14]byte
	sparseSnapshot := Snapshot{
		Nodes: map[string]Node{
			"foo": File{Data: string(zero[:])},
		},
	}

	opts := Options{Sparse: true, Overwrite: OverwriteAlways}
	saveSnapshotsAndOverwrite(t, baseSnapshot, sparseSnapshot, opts, opts)
}

type printerMock struct {
	s restoreui.State
}

func (p *printerMock) Update(_ restoreui.State, _ time.Duration) {
}
func (p *printerMock) Error(item string, err error) error {
	return nil
}
func (p *printerMock) CompleteItem(action restoreui.ItemAction, item string, size uint64) {
}
func (p *printerMock) Finish(s restoreui.State, _ time.Duration) {
	p.s = s
}

func TestRestorerOverwriteBehavior(t *testing.T) {
	baseTime := time.Now()
	baseSnapshot := Snapshot{
		Nodes: map[string]Node{
			"foo": File{Data: "content: foo\n", ModTime: baseTime},
			"dirtest": Dir{
				Nodes: map[string]Node{
					"file": File{Data: "content: file\n", ModTime: baseTime},
					"foo":  File{Data: "content: foobar", ModTime: baseTime},
				},
				ModTime: baseTime,
			},
		},
	}
	overwriteSnapshot := Snapshot{
		Nodes: map[string]Node{
			"foo": File{Data: "content: new\n", ModTime: baseTime.Add(time.Second)},
			"dirtest": Dir{
				Nodes: map[string]Node{
					"file": File{Data: "content: file2\n", ModTime: baseTime.Add(-time.Second)},
					"foo":  File{Data: "content: foo", ModTime: baseTime},
				},
			},
		},
	}

	var tests = []struct {
		Overwrite OverwriteBehavior
		Files     map[string]string
		Progress  restoreui.State
	}{
		{
			Overwrite: OverwriteAlways,
			Files: map[string]string{
				"foo":          "content: new\n",
				"dirtest/file": "content: file2\n",
				"dirtest/foo":  "content: foo",
			},
			Progress: restoreui.State{
				FilesFinished:   4,
				FilesTotal:      4,
				FilesSkipped:    0,
				AllBytesWritten: 40,
				AllBytesTotal:   40,
				AllBytesSkipped: 0,
			},
		},
		{
			Overwrite: OverwriteIfChanged,
			Files: map[string]string{
				"foo":          "content: new\n",
				"dirtest/file": "content: file2\n",
				"dirtest/foo":  "content: foo",
			},
			Progress: restoreui.State{
				FilesFinished:   4,
				FilesTotal:      4,
				FilesSkipped:    0,
				AllBytesWritten: 40,
				AllBytesTotal:   40,
				AllBytesSkipped: 0,
			},
		},
		{
			Overwrite: OverwriteIfNewer,
			Files: map[string]string{
				"foo":          "content: new\n",
				"dirtest/file": "content: file\n",
				"dirtest/foo":  "content: foobar",
			},
			Progress: restoreui.State{
				FilesFinished:   2,
				FilesTotal:      2,
				FilesSkipped:    2,
				AllBytesWritten: 13,
				AllBytesTotal:   13,
				AllBytesSkipped: 27,
			},
		},
		{
			Overwrite: OverwriteNever,
			Files: map[string]string{
				"foo":          "content: foo\n",
				"dirtest/file": "content: file\n",
				"dirtest/foo":  "content: foobar",
			},
			Progress: restoreui.State{
				FilesFinished:   1,
				FilesTotal:      1,
				FilesSkipped:    3,
				AllBytesWritten: 0,
				AllBytesTotal:   0,
				AllBytesSkipped: 40,
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			mock := &printerMock{}
			progress := restoreui.NewProgress(mock, 0)
			tempdir := saveSnapshotsAndOverwrite(t, baseSnapshot, overwriteSnapshot, Options{}, Options{Overwrite: test.Overwrite, Progress: progress})

			for filename, content := range test.Files {
				data, err := os.ReadFile(filepath.Join(tempdir, filepath.FromSlash(filename)))
				if err != nil {
					t.Errorf("unable to read file %v: %v", filename, err)
					continue
				}

				if !bytes.Equal(data, []byte(content)) {
					t.Errorf("file %v has wrong content: want %q, got %q", filename, content, data)
				}
			}

			progress.Finish()
			rtest.Equals(t, test.Progress, mock.s)
		})
	}
}

func TestRestorerOverwritePartial(t *testing.T) {
	parts := make([]string, 100)
	size := 0
	for i := 0; i < len(parts); i++ {
		parts[i] = fmt.Sprint(i)
		size += len(parts[i])
		if i < 8 {
			// small file
			size += len(parts[i])
		}
	}

	// the data of both snapshots is stored in different pack files
	// thus both small an foo in the overwriteSnapshot contain blobs from
	// two different pack files. This tests basic handling of blobs from
	// different pack files.
	baseTime := time.Now()
	baseSnapshot := Snapshot{
		Nodes: map[string]Node{
			"foo":   File{DataParts: parts[0:5], ModTime: baseTime},
			"small": File{DataParts: parts[0:5], ModTime: baseTime},
		},
	}
	overwriteSnapshot := Snapshot{
		Nodes: map[string]Node{
			"foo":   File{DataParts: parts, ModTime: baseTime},
			"small": File{DataParts: parts[0:8], ModTime: baseTime},
		},
	}

	mock := &printerMock{}
	progress := restoreui.NewProgress(mock, 0)
	saveSnapshotsAndOverwrite(t, baseSnapshot, overwriteSnapshot, Options{}, Options{Overwrite: OverwriteAlways, Progress: progress})
	progress.Finish()
	rtest.Equals(t, restoreui.State{
		FilesFinished:   2,
		FilesTotal:      2,
		FilesSkipped:    0,
		AllBytesWritten: uint64(size),
		AllBytesTotal:   uint64(size),
		AllBytesSkipped: 0,
	}, mock.s)
}

func TestRestorerOverwriteSpecial(t *testing.T) {
	baseTime := time.Now()
	baseSnapshot := Snapshot{
		Nodes: map[string]Node{
			"dirtest":  Dir{ModTime: baseTime},
			"link":     Symlink{Target: "foo", ModTime: baseTime},
			"file":     File{Data: "content: file\n", Inode: 42, Links: 2, ModTime: baseTime},
			"hardlink": File{Data: "content: file\n", Inode: 42, Links: 2, ModTime: baseTime},
			"newdir":   File{Data: "content: dir\n", ModTime: baseTime},
		},
	}
	overwriteSnapshot := Snapshot{
		Nodes: map[string]Node{
			"dirtest":  Symlink{Target: "foo", ModTime: baseTime},
			"link":     File{Data: "content: link\n", Inode: 42, Links: 2, ModTime: baseTime.Add(time.Second)},
			"file":     Symlink{Target: "foo2", ModTime: baseTime},
			"hardlink": File{Data: "content: link\n", Inode: 42, Links: 2, ModTime: baseTime.Add(time.Second)},
			"newdir":   Dir{ModTime: baseTime},
		},
	}

	files := map[string]string{
		"link":     "content: link\n",
		"hardlink": "content: link\n",
	}
	links := map[string]string{
		"dirtest": "foo",
		"file":    "foo2",
	}

	opts := Options{Overwrite: OverwriteAlways}
	tempdir := saveSnapshotsAndOverwrite(t, baseSnapshot, overwriteSnapshot, opts, opts)

	for filename, content := range files {
		data, err := os.ReadFile(filepath.Join(tempdir, filepath.FromSlash(filename)))
		if err != nil {
			t.Errorf("unable to read file %v: %v", filename, err)
			continue
		}

		if !bytes.Equal(data, []byte(content)) {
			t.Errorf("file %v has wrong content: want %q, got %q", filename, content, data)
		}
	}
	for filename, target := range links {
		link, err := fs.Readlink(filepath.Join(tempdir, filepath.FromSlash(filename)))
		rtest.OK(t, err)
		rtest.Equals(t, link, target, "wrong symlink target")
	}
}

func TestRestoreModified(t *testing.T) {
	// overwrite files between snapshots and also change their filesize
	snapshots := []Snapshot{
		{
			Nodes: map[string]Node{
				"foo": File{Data: "content: foo\n", ModTime: time.Now()},
				"bar": File{Data: "content: a\n", ModTime: time.Now()},
			},
		},
		{
			Nodes: map[string]Node{
				"foo": File{Data: "content: a\n", ModTime: time.Now()},
				"bar": File{Data: "content: bar\n", ModTime: time.Now()},
			},
		},
	}

	repo := repository.TestRepository(t)
	tempdir := filepath.Join(rtest.TempDir(t), "target")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, snapshot := range snapshots {
		sn, id := saveSnapshot(t, repo, snapshot, noopGetGenericAttributes)
		t.Logf("snapshot saved as %v", id.Str())

		res := NewRestorer(repo, sn, Options{Overwrite: OverwriteIfChanged})
		rtest.OK(t, res.RestoreTo(ctx, tempdir))
		n, err := res.VerifyFiles(ctx, tempdir)
		rtest.OK(t, err)
		rtest.Equals(t, 2, n, "unexpected number of verified files")
	}
}

func TestRestoreIfChanged(t *testing.T) {
	origData := "content: foo\n"
	modData := "content: bar\n"
	rtest.Equals(t, len(modData), len(origData), "broken testcase")
	snapshot := Snapshot{
		Nodes: map[string]Node{
			"foo": File{Data: origData, ModTime: time.Now()},
		},
	}

	repo := repository.TestRepository(t)
	tempdir := filepath.Join(rtest.TempDir(t), "target")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sn, id := saveSnapshot(t, repo, snapshot, noopGetGenericAttributes)
	t.Logf("snapshot saved as %v", id.Str())

	res := NewRestorer(repo, sn, Options{})
	rtest.OK(t, res.RestoreTo(ctx, tempdir))

	// modify file but maintain size and timestamp
	path := filepath.Join(tempdir, "foo")
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	rtest.OK(t, err)
	fi, err := f.Stat()
	rtest.OK(t, err)
	_, err = f.Write([]byte(modData))
	rtest.OK(t, err)
	rtest.OK(t, f.Close())
	var utimes = [...]syscall.Timespec{
		syscall.NsecToTimespec(fi.ModTime().UnixNano()),
		syscall.NsecToTimespec(fi.ModTime().UnixNano()),
	}
	rtest.OK(t, syscall.UtimesNano(path, utimes[:]))

	for _, overwrite := range []OverwriteBehavior{OverwriteIfChanged, OverwriteAlways} {
		res = NewRestorer(repo, sn, Options{Overwrite: overwrite})
		rtest.OK(t, res.RestoreTo(ctx, tempdir))
		data, err := os.ReadFile(path)
		rtest.OK(t, err)
		if overwrite == OverwriteAlways {
			// restore should notice the changed file content
			rtest.Equals(t, origData, string(data), "expected original file content")
		} else {
			// restore should not have noticed the changed file content
			rtest.Equals(t, modData, string(data), "expected modified file content")
		}
	}
}

func TestRestoreDryRun(t *testing.T) {
	snapshot := Snapshot{
		Nodes: map[string]Node{
			"foo":  File{Data: "content: foo\n", Links: 2, Inode: 42},
			"foo2": File{Data: "content: foo\n", Links: 2, Inode: 42},
			"dirtest": Dir{
				Nodes: map[string]Node{
					"file": File{Data: "content: file\n"},
				},
			},
			"link": Symlink{Target: "foo"},
		},
	}

	repo := repository.TestRepository(t)
	tempdir := filepath.Join(rtest.TempDir(t), "target")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sn, id := saveSnapshot(t, repo, snapshot, noopGetGenericAttributes)
	t.Logf("snapshot saved as %v", id.Str())

	res := NewRestorer(repo, sn, Options{DryRun: true})
	rtest.OK(t, res.RestoreTo(ctx, tempdir))

	_, err := os.Stat(tempdir)
	rtest.Assert(t, errors.Is(err, os.ErrNotExist), "expected no file to be created, got %v", err)
}

func TestRestoreDryRunDelete(t *testing.T) {
	snapshot := Snapshot{
		Nodes: map[string]Node{
			"foo": File{Data: "content: foo\n"},
		},
	}

	repo := repository.TestRepository(t)
	tempdir := filepath.Join(rtest.TempDir(t), "target")
	tempfile := filepath.Join(tempdir, "existing")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rtest.OK(t, os.Mkdir(tempdir, 0o755))
	f, err := os.Create(tempfile)
	rtest.OK(t, err)
	rtest.OK(t, f.Close())

	sn, _ := saveSnapshot(t, repo, snapshot, noopGetGenericAttributes)
	res := NewRestorer(repo, sn, Options{DryRun: true, Delete: true})
	rtest.OK(t, res.RestoreTo(ctx, tempdir))

	_, err = os.Stat(tempfile)
	rtest.Assert(t, err == nil, "expected file to still exist, got error %v", err)
}

func TestRestoreOverwriteDirectory(t *testing.T) {
	saveSnapshotsAndOverwrite(t,
		Snapshot{
			Nodes: map[string]Node{
				"dir": Dir{
					Mode: normalizeFileMode(0755 | os.ModeDir),
					Nodes: map[string]Node{
						"anotherfile": File{Data: "content: file\n"},
					},
				},
			},
		},
		Snapshot{
			Nodes: map[string]Node{
				"dir": File{Data: "content: file\n"},
			},
		},
		Options{},
		Options{Delete: true},
	)
}

func TestRestoreDelete(t *testing.T) {
	repo := repository.TestRepository(t)
	tempdir := rtest.TempDir(t)

	sn, _ := saveSnapshot(t, repo, Snapshot{
		Nodes: map[string]Node{
			"dir": Dir{
				Mode: normalizeFileMode(0755 | os.ModeDir),
				Nodes: map[string]Node{
					"file1":       File{Data: "content: file\n"},
					"anotherfile": File{Data: "content: file\n"},
				},
			},
			"dir2": Dir{
				Mode: normalizeFileMode(0755 | os.ModeDir),
				Nodes: map[string]Node{
					"anotherfile": File{Data: "content: file\n"},
				},
			},
			"anotherfile": File{Data: "content: file\n"},
		},
	}, noopGetGenericAttributes)

	// should delete files that no longer exist in the snapshot
	deleteSn, _ := saveSnapshot(t, repo, Snapshot{
		Nodes: map[string]Node{
			"dir": Dir{
				Mode: normalizeFileMode(0755 | os.ModeDir),
				Nodes: map[string]Node{
					"file1": File{Data: "content: file\n"},
				},
			},
		},
	}, noopGetGenericAttributes)

	tests := []struct {
		selectFilter func(item string, isDir bool) (selectedForRestore bool, childMayBeSelected bool)
		fileState    map[string]bool
	}{
		{
			selectFilter: nil,
			fileState: map[string]bool{
				"dir":                                true,
				filepath.Join("dir", "anotherfile"):  false,
				filepath.Join("dir", "file1"):        true,
				"dir2":                               false,
				filepath.Join("dir2", "anotherfile"): false,
				"anotherfile":                        false,
			},
		},
		{
			selectFilter: func(item string, isDir bool) (selectedForRestore bool, childMayBeSelected bool) {
				return false, false
			},
			fileState: map[string]bool{
				"dir":                                true,
				filepath.Join("dir", "anotherfile"):  true,
				filepath.Join("dir", "file1"):        true,
				"dir2":                               true,
				filepath.Join("dir2", "anotherfile"): true,
				"anotherfile":                        true,
			},
		},
		{
			selectFilter: func(item string, isDir bool) (selectedForRestore bool, childMayBeSelected bool) {
				switch item {
				case filepath.FromSlash("/dir"):
					selectedForRestore = true
				case filepath.FromSlash("/dir2"):
					selectedForRestore = true
				}
				return
			},
			fileState: map[string]bool{
				"dir":                                true,
				filepath.Join("dir", "anotherfile"):  true,
				filepath.Join("dir", "file1"):        true,
				"dir2":                               false,
				filepath.Join("dir2", "anotherfile"): false,
				"anotherfile":                        true,
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			res := NewRestorer(repo, sn, Options{})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := res.RestoreTo(ctx, tempdir)
			rtest.OK(t, err)

			res = NewRestorer(repo, deleteSn, Options{Delete: true})
			if test.selectFilter != nil {
				res.SelectFilter = test.selectFilter
			}
			err = res.RestoreTo(ctx, tempdir)
			rtest.OK(t, err)

			for fn, shouldExist := range test.fileState {
				_, err := os.Stat(filepath.Join(tempdir, fn))
				if shouldExist {
					rtest.OK(t, err)
				} else {
					rtest.Assert(t, errors.Is(err, os.ErrNotExist), "file %v: unexpected error got %v, expected ErrNotExist", fn, err)
				}
			}
		})
	}
}

func TestRestoreToFile(t *testing.T) {
	snapshot := Snapshot{
		Nodes: map[string]Node{
			"foo": File{Data: "content: foo\n"},
		},
	}

	repo := repository.TestRepository(t)
	tempdir := filepath.Join(rtest.TempDir(t), "target")

	// create a file in the place of the target directory
	rtest.OK(t, os.WriteFile(tempdir, []byte{}, 0o700))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sn, _ := saveSnapshot(t, repo, snapshot, noopGetGenericAttributes)
	res := NewRestorer(repo, sn, Options{})
	err := res.RestoreTo(ctx, tempdir)
	rtest.Assert(t, strings.Contains(err.Error(), "cannot create target directory"), "unexpected error %v", err)
}

func TestRestorerLongPath(t *testing.T) {
	tmp := t.TempDir()

	longPath := tmp
	for i := 0; i < 20; i++ {
		longPath = filepath.Join(longPath, "aaaaaaaaaaaaaaaaaaaa")
	}

	rtest.OK(t, os.MkdirAll(longPath, 0o700))
	f, err := fs.OpenFile(filepath.Join(longPath, "file"), fs.O_CREATE|fs.O_RDWR, 0o600)
	rtest.OK(t, err)
	_, err = f.WriteString("Hello, World!")
	rtest.OK(t, err)
	rtest.OK(t, f.Close())

	repo := repository.TestRepository(t)

	local := &fs.Local{}
	sc := archiver.NewScanner(local)
	rtest.OK(t, sc.Scan(context.TODO(), []string{tmp}))
	arch := archiver.New(repo, local, archiver.Options{})
	sn, _, _, err := arch.Snapshot(context.Background(), []string{tmp}, archiver.SnapshotOptions{})
	rtest.OK(t, err)

	res := NewRestorer(repo, sn, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rtest.OK(t, res.RestoreTo(ctx, tmp))
	_, err = res.VerifyFiles(ctx, tmp)
	rtest.OK(t, err)
}
