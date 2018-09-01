package archiver

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
	restictest "github.com/restic/restic/internal/test"
)

// MockT passes through all logging functions from T, but catches Fail(),
// Error/f() and Fatal/f(). It is used to test test helper functions.
type MockT struct {
	*testing.T
	HasFailed bool
}

// Fail marks the function as having failed but continues execution.
func (t *MockT) Fail() {
	t.T.Log("MockT Fail() called")
	t.HasFailed = true
}

// Fatal is equivalent to Log followed by FailNow.
func (t *MockT) Fatal(args ...interface{}) {
	t.T.Logf("MockT Fatal called with %v", args)
	t.HasFailed = true
}

// Fatalf is equivalent to Logf followed by FailNow.
func (t *MockT) Fatalf(msg string, args ...interface{}) {
	t.T.Logf("MockT Fatal called: "+msg, args...)
	t.HasFailed = true
}

// Error is equivalent to Log followed by Fail.
func (t *MockT) Error(args ...interface{}) {
	t.T.Logf("MockT Error called with %v", args)
	t.HasFailed = true
}

// Errorf is equivalent to Logf followed by Fail.
func (t *MockT) Errorf(msg string, args ...interface{}) {
	t.T.Logf("MockT Error called: "+msg, args...)
	t.HasFailed = true
}

func createFilesAt(t testing.TB, targetdir string, files map[string]interface{}) {
	for name, item := range files {
		target := filepath.Join(targetdir, filepath.FromSlash(name))
		err := fs.MkdirAll(filepath.Dir(target), 0700)
		if err != nil {
			t.Fatal(err)
		}

		switch it := item.(type) {
		case TestFile:
			err := ioutil.WriteFile(target, []byte(it.Content), 0600)
			if err != nil {
				t.Fatal(err)
			}
		case TestSymlink:
			// ignore symlinks on windows
			if runtime.GOOS == "windows" {
				continue
			}
			err := fs.Symlink(filepath.FromSlash(it.Target), target)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestTestCreateFiles(t *testing.T) {
	var tests = []struct {
		dir   TestDir
		files map[string]interface{}
	}{
		{
			dir: TestDir{
				"foo": TestFile{Content: "foo"},
				"subdir": TestDir{
					"subfile": TestFile{Content: "bar"},
				},
				"sub": TestDir{
					"subsub": TestDir{
						"link": TestSymlink{Target: "x/y/z"},
					},
				},
			},
			files: map[string]interface{}{
				"foo":             TestFile{Content: "foo"},
				"subdir":          TestDir{},
				"subdir/subfile":  TestFile{Content: "bar"},
				"sub/subsub/link": TestSymlink{Target: "x/y/z"},
			},
		},
	}

	for i, test := range tests {
		tempdir, cleanup := restictest.TempDir(t)
		defer cleanup()

		t.Run("", func(t *testing.T) {
			tempdir := filepath.Join(tempdir, fmt.Sprintf("test-%d", i))
			err := fs.MkdirAll(tempdir, 0700)
			if err != nil {
				t.Fatal(err)
			}

			TestCreateFiles(t, tempdir, test.dir)

			for name, item := range test.files {
				// don't check symlinks on windows
				if runtime.GOOS == "windows" {
					if _, ok := item.(TestSymlink); ok {
						continue
					}
					continue
				}

				targetPath := filepath.Join(tempdir, filepath.FromSlash(name))
				fi, err := fs.Lstat(targetPath)
				if err != nil {
					t.Error(err)
					continue
				}

				switch node := item.(type) {
				case TestFile:
					if !fs.IsRegularFile(fi) {
						t.Errorf("is not regular file: %v", name)
						continue
					}

					content, err := ioutil.ReadFile(targetPath)
					if err != nil {
						t.Error(err)
						continue
					}

					if string(content) != node.Content {
						t.Errorf("wrong content for %v: want %q, got %q", name, node.Content, content)
					}
				case TestSymlink:
					if fi.Mode()&os.ModeType != os.ModeSymlink {
						t.Errorf("is not symlink: %v, %o != %o", name, fi.Mode(), os.ModeSymlink)
						continue
					}

					target, err := fs.Readlink(targetPath)
					if err != nil {
						t.Error(err)
						continue
					}

					if target != node.Target {
						t.Errorf("wrong target for %v: want %q, got %q", name, node.Target, target)
					}
				case TestDir:
					if !fi.IsDir() {
						t.Errorf("is not directory: %v", name)
					}
				}
			}
		})
	}
}

func TestTestWalkFiles(t *testing.T) {
	var tests = []struct {
		dir  TestDir
		want map[string]string
	}{
		{
			dir: TestDir{
				"foo": TestFile{Content: "foo"},
				"subdir": TestDir{
					"subfile": TestFile{Content: "bar"},
				},
				"x": TestDir{
					"y": TestDir{
						"link": TestSymlink{Target: filepath.FromSlash("../../foo")},
					},
				},
			},
			want: map[string]string{
				"foo":                                "<File>",
				"subdir":                             "<Dir>",
				filepath.FromSlash("subdir/subfile"): "<File>",
				"x":                                  "<Dir>",
				filepath.FromSlash("x/y"):            "<Dir>",
				filepath.FromSlash("x/y/link"):       "<Symlink>",
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			tempdir, cleanup := restictest.TempDir(t)
			defer cleanup()

			got := make(map[string]string)

			TestCreateFiles(t, tempdir, test.dir)
			TestWalkFiles(t, tempdir, test.dir, func(path string, item interface{}) error {
				p, err := filepath.Rel(tempdir, path)
				if err != nil {
					return err
				}

				got[p] = fmt.Sprintf("%v", item)
				return nil
			})

			if !cmp.Equal(test.want, got) {
				t.Error(cmp.Diff(test.want, got))
			}
		})
	}
}

func TestTestEnsureFiles(t *testing.T) {
	var tests = []struct {
		expectFailure bool
		files         map[string]interface{}
		want          TestDir
		unixOnly      bool
	}{
		{
			files: map[string]interface{}{
				"foo":            TestFile{Content: "foo"},
				"subdir/subfile": TestFile{Content: "bar"},
				"x/y/link":       TestSymlink{Target: "../../foo"},
			},
			want: TestDir{
				"foo": TestFile{Content: "foo"},
				"subdir": TestDir{
					"subfile": TestFile{Content: "bar"},
				},
				"x": TestDir{
					"y": TestDir{
						"link": TestSymlink{Target: "../../foo"},
					},
				},
			},
		},
		{
			expectFailure: true,
			files: map[string]interface{}{
				"foo": TestFile{Content: "foo"},
			},
			want: TestDir{
				"foo": TestFile{Content: "foo"},
				"subdir": TestDir{
					"subfile": TestFile{Content: "bar"},
				},
			},
		},
		{
			expectFailure: true,
			files: map[string]interface{}{
				"foo":            TestFile{Content: "foo"},
				"subdir/subfile": TestFile{Content: "bar"},
			},
			want: TestDir{
				"foo": TestFile{Content: "foo"},
			},
		},
		{
			expectFailure: true,
			files: map[string]interface{}{
				"foo": TestFile{Content: "xxx"},
			},
			want: TestDir{
				"foo": TestFile{Content: "foo"},
			},
		},
		{
			expectFailure: true,
			files: map[string]interface{}{
				"foo": TestSymlink{Target: "/xxx"},
			},
			want: TestDir{
				"foo": TestFile{Content: "foo"},
			},
		},
		{
			expectFailure: true,
			unixOnly:      true,
			files: map[string]interface{}{
				"foo": TestFile{Content: "foo"},
			},
			want: TestDir{
				"foo": TestSymlink{Target: "/xxx"},
			},
		},
		{
			expectFailure: true,
			unixOnly:      true,
			files: map[string]interface{}{
				"foo": TestSymlink{Target: "xxx"},
			},
			want: TestDir{
				"foo": TestSymlink{Target: "/yyy"},
			},
		},
		{
			expectFailure: true,
			files: map[string]interface{}{
				"foo": TestDir{
					"foo": TestFile{Content: "foo"},
				},
			},
			want: TestDir{
				"foo": TestFile{Content: "foo"},
			},
		},
		{
			expectFailure: true,
			files: map[string]interface{}{
				"foo": TestFile{Content: "foo"},
			},
			want: TestDir{
				"foo": TestDir{
					"foo": TestFile{Content: "foo"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			if test.unixOnly && runtime.GOOS == "windows" {
				t.Skip("skip on Windows")
				return
			}

			tempdir, cleanup := restictest.TempDir(t)
			defer cleanup()

			createFilesAt(t, tempdir, test.files)

			subtestT := testing.TB(t)
			if test.expectFailure {
				subtestT = &MockT{T: t}
			}

			TestEnsureFiles(subtestT, tempdir, test.want)

			if test.expectFailure && !subtestT.(*MockT).HasFailed {
				t.Fatal("expected failure of TestEnsureFiles not found")
			}
		})
	}
}

func TestTestEnsureSnapshot(t *testing.T) {
	var tests = []struct {
		expectFailure bool
		files         map[string]interface{}
		want          TestDir
		unixOnly      bool
	}{
		{
			files: map[string]interface{}{
				"foo":                                TestFile{Content: "foo"},
				filepath.FromSlash("subdir/subfile"): TestFile{Content: "bar"},
				filepath.FromSlash("x/y/link"):       TestSymlink{Target: filepath.FromSlash("../../foo")},
			},
			want: TestDir{
				"target": TestDir{
					"foo": TestFile{Content: "foo"},
					"subdir": TestDir{
						"subfile": TestFile{Content: "bar"},
					},
					"x": TestDir{
						"y": TestDir{
							"link": TestSymlink{Target: filepath.FromSlash("../../foo")},
						},
					},
				},
			},
		},
		{
			expectFailure: true,
			files: map[string]interface{}{
				"foo": TestFile{Content: "foo"},
			},
			want: TestDir{
				"target": TestDir{
					"bar": TestFile{Content: "foo"},
				},
			},
		},
		{
			expectFailure: true,
			files: map[string]interface{}{
				"foo": TestFile{Content: "foo"},
				"bar": TestFile{Content: "bar"},
			},
			want: TestDir{
				"target": TestDir{
					"foo": TestFile{Content: "foo"},
				},
			},
		},
		{
			expectFailure: true,
			files: map[string]interface{}{
				"foo": TestFile{Content: "foo"},
			},
			want: TestDir{
				"target": TestDir{
					"foo": TestFile{Content: "foo"},
					"bar": TestFile{Content: "bar"},
				},
			},
		},
		{
			expectFailure: true,
			files: map[string]interface{}{
				"foo": TestFile{Content: "foo"},
			},
			want: TestDir{
				"target": TestDir{
					"foo": TestDir{
						"foo": TestFile{Content: "foo"},
					},
				},
			},
		},
		{
			expectFailure: true,
			files: map[string]interface{}{
				"foo": TestSymlink{Target: filepath.FromSlash("x/y/z")},
			},
			want: TestDir{
				"target": TestDir{
					"foo": TestFile{Content: "foo"},
				},
			},
		},
		{
			expectFailure: true,
			unixOnly:      true,
			files: map[string]interface{}{
				"foo": TestSymlink{Target: filepath.FromSlash("x/y/z")},
			},
			want: TestDir{
				"target": TestDir{
					"foo": TestSymlink{Target: filepath.FromSlash("x/y/z2")},
				},
			},
		},
		{
			expectFailure: true,
			files: map[string]interface{}{
				"foo": TestFile{Content: "foo"},
			},
			want: TestDir{
				"target": TestDir{
					"foo": TestFile{Content: "xxx"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			if test.unixOnly && runtime.GOOS == "windows" {
				t.Skip("skip on Windows")
				return
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tempdir, cleanup := restictest.TempDir(t)
			defer cleanup()

			targetDir := filepath.Join(tempdir, "target")
			err := fs.Mkdir(targetDir, 0700)
			if err != nil {
				t.Fatal(err)
			}

			createFilesAt(t, targetDir, test.files)

			back := fs.TestChdir(t, tempdir)
			defer back()

			repo, cleanup := repository.TestRepository(t)
			defer cleanup()

			arch := New(repo, fs.Local{}, Options{})
			opts := SnapshotOptions{
				Time:     time.Now(),
				Hostname: "localhost",
				Tags:     []string{"test"},
			}
			_, id, err := arch.Snapshot(ctx, []string{"."}, opts)
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("snapshot saved as %v", id.Str())

			subtestT := testing.TB(t)
			if test.expectFailure {
				subtestT = &MockT{T: t}
			}

			TestEnsureSnapshot(subtestT, repo, id, test.want)

			if test.expectFailure && !subtestT.(*MockT).HasFailed {
				t.Fatal("expected failure of TestEnsureSnapshot not found")
			}
		})
	}
}
