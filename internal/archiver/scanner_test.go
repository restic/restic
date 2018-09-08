package archiver

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/restic/restic/internal/fs"
	restictest "github.com/restic/restic/internal/test"
)

func TestScanner(t *testing.T) {
	var tests = []struct {
		name  string
		src   TestDir
		want  map[string]ScanStats
		selFn SelectFunc
	}{
		{
			name: "include-all",
			src: TestDir{
				"other": TestFile{Content: "another file"},
				"work": TestDir{
					"foo":     TestFile{Content: "foo"},
					"foo.txt": TestFile{Content: "foo text file"},
					"subdir": TestDir{
						"other":   TestFile{Content: "other in subdir"},
						"bar.txt": TestFile{Content: "bar.txt in subdir"},
					},
				},
			},
			want: map[string]ScanStats{
				filepath.FromSlash("other"):               ScanStats{Files: 1, Bytes: 12},
				filepath.FromSlash("work/foo"):            ScanStats{Files: 2, Bytes: 15},
				filepath.FromSlash("work/foo.txt"):        ScanStats{Files: 3, Bytes: 28},
				filepath.FromSlash("work/subdir/bar.txt"): ScanStats{Files: 4, Bytes: 45},
				filepath.FromSlash("work/subdir/other"):   ScanStats{Files: 5, Bytes: 60},
				filepath.FromSlash("work/subdir"):         ScanStats{Files: 5, Dirs: 1, Bytes: 60},
				filepath.FromSlash("work"):                ScanStats{Files: 5, Dirs: 2, Bytes: 60},
				filepath.FromSlash("."):                   ScanStats{Files: 5, Dirs: 3, Bytes: 60},
				filepath.FromSlash(""):                    ScanStats{Files: 5, Dirs: 3, Bytes: 60},
			},
		},
		{
			name: "select-txt",
			src: TestDir{
				"other": TestFile{Content: "another file"},
				"work": TestDir{
					"foo":     TestFile{Content: "foo"},
					"foo.txt": TestFile{Content: "foo text file"},
					"subdir": TestDir{
						"other":   TestFile{Content: "other in subdir"},
						"bar.txt": TestFile{Content: "bar.txt in subdir"},
					},
				},
			},
			selFn: func(item string, fi os.FileInfo) bool {
				if fi.IsDir() {
					return true
				}

				if filepath.Ext(item) == ".txt" {
					return true
				}
				return false
			},
			want: map[string]ScanStats{
				filepath.FromSlash("work/foo.txt"):        ScanStats{Files: 1, Bytes: 13},
				filepath.FromSlash("work/subdir/bar.txt"): ScanStats{Files: 2, Bytes: 30},
				filepath.FromSlash("work/subdir"):         ScanStats{Files: 2, Dirs: 1, Bytes: 30},
				filepath.FromSlash("work"):                ScanStats{Files: 2, Dirs: 2, Bytes: 30},
				filepath.FromSlash("."):                   ScanStats{Files: 2, Dirs: 3, Bytes: 30},
				filepath.FromSlash(""):                    ScanStats{Files: 2, Dirs: 3, Bytes: 30},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tempdir, cleanup := restictest.TempDir(t)
			defer cleanup()

			TestCreateFiles(t, tempdir, test.src)

			back := fs.TestChdir(t, tempdir)
			defer back()

			cur, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}

			sc := NewScanner(fs.Track{FS: fs.Local{}})
			if test.selFn != nil {
				sc.Select = test.selFn
			}

			results := make(map[string]ScanStats)
			sc.Result = func(item string, s ScanStats) {
				var p string
				var err error

				if item != "" {
					p, err = filepath.Rel(cur, item)
					if err != nil {
						panic(err)
					}
				}

				results[p] = s
			}

			err = sc.Scan(ctx, []string{"."})
			if err != nil {
				t.Fatal(err)
			}

			if !cmp.Equal(test.want, results) {
				t.Error(cmp.Diff(test.want, results))
			}
		})
	}
}

func TestScannerError(t *testing.T) {
	var tests = []struct {
		name    string
		unix    bool
		src     TestDir
		result  ScanStats
		selFn   SelectFunc
		errFn   func(t testing.TB, item string, fi os.FileInfo, err error) error
		resFn   func(t testing.TB, item string, s ScanStats)
		prepare func(t testing.TB)
	}{
		{
			name: "no-error",
			src: TestDir{
				"other": TestFile{Content: "another file"},
				"work": TestDir{
					"foo":     TestFile{Content: "foo"},
					"foo.txt": TestFile{Content: "foo text file"},
					"subdir": TestDir{
						"other":   TestFile{Content: "other in subdir"},
						"bar.txt": TestFile{Content: "bar.txt in subdir"},
					},
				},
			},
			result: ScanStats{Files: 5, Dirs: 3, Bytes: 60},
		},
		{
			name: "unreadable-dir",
			unix: true,
			src: TestDir{
				"other": TestFile{Content: "another file"},
				"work": TestDir{
					"foo":     TestFile{Content: "foo"},
					"foo.txt": TestFile{Content: "foo text file"},
					"subdir": TestDir{
						"other":   TestFile{Content: "other in subdir"},
						"bar.txt": TestFile{Content: "bar.txt in subdir"},
					},
				},
			},
			result: ScanStats{Files: 3, Dirs: 2, Bytes: 28},
			prepare: func(t testing.TB) {
				err := os.Chmod(filepath.Join("work", "subdir"), 0000)
				if err != nil {
					t.Fatal(err)
				}
			},
			errFn: func(t testing.TB, item string, fi os.FileInfo, err error) error {
				if item == filepath.FromSlash("work/subdir") {
					return nil
				}

				return err
			},
		},
		{
			name: "removed-item",
			src: TestDir{
				"bar":   TestFile{Content: "bar"},
				"baz":   TestFile{Content: "baz"},
				"foo":   TestFile{Content: "foo"},
				"other": TestFile{Content: "other"},
			},
			result: ScanStats{Files: 3, Dirs: 1, Bytes: 11},
			resFn: func(t testing.TB, item string, s ScanStats) {
				if item == "bar" {
					err := os.Remove("foo")
					if err != nil {
						t.Fatal(err)
					}
				}
			},
			errFn: func(t testing.TB, item string, fi os.FileInfo, err error) error {
				if item == "foo" {
					t.Logf("ignoring error for %v: %v", item, err)
					return nil
				}

				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.unix && runtime.GOOS == "windows" {
				t.Skipf("skip on windows")
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tempdir, cleanup := restictest.TempDir(t)
			defer cleanup()

			TestCreateFiles(t, tempdir, test.src)

			back := fs.TestChdir(t, tempdir)
			defer back()

			cur, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}

			if test.prepare != nil {
				test.prepare(t)
			}

			sc := NewScanner(fs.Track{FS: fs.Local{}})
			if test.selFn != nil {
				sc.Select = test.selFn
			}

			var stats ScanStats

			sc.Result = func(item string, s ScanStats) {
				if item == "" {
					stats = s
					return
				}

				if test.resFn != nil {
					p, relErr := filepath.Rel(cur, item)
					if relErr != nil {
						panic(relErr)
					}
					test.resFn(t, p, s)
				}
			}
			if test.errFn != nil {
				sc.Error = func(item string, fi os.FileInfo, err error) error {
					p, relErr := filepath.Rel(cur, item)
					if relErr != nil {
						panic(relErr)
					}

					return test.errFn(t, p, fi, err)
				}
			}

			err = sc.Scan(ctx, []string{"."})
			if err != nil {
				t.Fatal(err)
			}

			if stats != test.result {
				t.Errorf("wrong final result, want\n  %#v\ngot:\n  %#v", test.result, stats)
			}
		})
	}
}

func TestScannerCancel(t *testing.T) {
	src := TestDir{
		"bar":   TestFile{Content: "bar"},
		"baz":   TestFile{Content: "baz"},
		"foo":   TestFile{Content: "foo"},
		"other": TestFile{Content: "other"},
	}

	result := ScanStats{Files: 2, Dirs: 1, Bytes: 6}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tempdir, cleanup := restictest.TempDir(t)
	defer cleanup()

	TestCreateFiles(t, tempdir, src)

	back := fs.TestChdir(t, tempdir)
	defer back()

	cur, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	sc := NewScanner(fs.Track{FS: fs.Local{}})
	var lastStats ScanStats
	sc.Result = func(item string, s ScanStats) {
		lastStats = s

		if item == filepath.Join(cur, "baz") {
			t.Logf("found baz")
			cancel()
		}
	}

	err = sc.Scan(ctx, []string{"."})
	if err != nil {
		t.Errorf("unexpected error %v found", err)
	}

	if lastStats != result {
		t.Errorf("wrong final result, want\n  %#v\ngot:\n  %#v", result, lastStats)
	}
}
