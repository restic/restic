package archiver

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/restic/restic/internal/fs"
	rtest "github.com/restic/restic/internal/test"
)

// debug.Log requires Tree.String.
var _ fmt.Stringer = tree{}

func TestPathComponents(t *testing.T) {
	var tests = []struct {
		p       string
		c       []string
		virtual bool
		rel     bool
		win     bool
	}{
		{
			p: "/foo/bar/baz",
			c: []string{"foo", "bar", "baz"},
		},
		{
			p:   "/foo/bar/baz",
			c:   []string{"foo", "bar", "baz"},
			rel: true,
		},
		{
			p: "foo/bar/baz",
			c: []string{"foo", "bar", "baz"},
		},
		{
			p:   "foo/bar/baz",
			c:   []string{"foo", "bar", "baz"},
			rel: true,
		},
		{
			p: "../foo/bar/baz",
			c: []string{"foo", "bar", "baz"},
		},
		{
			p:   "../foo/bar/baz",
			c:   []string{"..", "foo", "bar", "baz"},
			rel: true,
		},
		{
			p:       "c:/foo/bar/baz",
			c:       []string{"c", "foo", "bar", "baz"},
			virtual: true,
			rel:     true,
			win:     true,
		},
		{
			p:       "c:/foo/../bar/baz",
			c:       []string{"c", "bar", "baz"},
			virtual: true,
			win:     true,
		},
		{
			p:       `c:\foo\..\bar\baz`,
			c:       []string{"c", "bar", "baz"},
			virtual: true,
			win:     true,
		},
		{
			p:       "c:/foo/../bar/baz",
			c:       []string{"c", "bar", "baz"},
			virtual: true,
			rel:     true,
			win:     true,
		},
		{
			p:       `c:\foo\..\bar\baz`,
			c:       []string{"c", "bar", "baz"},
			virtual: true,
			rel:     true,
			win:     true,
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			if test.win && runtime.GOOS != "windows" {
				t.Skip("skip test on unix")
			}

			c, v := pathComponents(fs.Local{}, filepath.FromSlash(test.p), test.rel)
			if !cmp.Equal(test.c, c) {
				t.Error(test.c, c)
			}

			if v != test.virtual {
				t.Errorf("unexpected virtual prefix count returned, want %v, got %v", test.virtual, v)
			}
		})
	}
}

func TestRootDirectory(t *testing.T) {
	var tests = []struct {
		target string
		root   string
		unix   bool
		win    bool
	}{
		{target: ".", root: "."},
		{target: "foo/bar/baz", root: "."},
		{target: "../foo/bar/baz", root: ".."},
		{target: "..", root: ".."},
		{target: "../../..", root: "../../.."},
		{target: "/home/foo", root: "/", unix: true},
		{target: "c:/home/foo", root: "c:/", win: true},
		{target: `c:\home\foo`, root: `c:\`, win: true},
		{target: "//host/share/foo", root: "//host/share/", win: true},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			if test.unix && runtime.GOOS == "windows" {
				t.Skip("skip test on windows")
			}
			if test.win && runtime.GOOS != "windows" {
				t.Skip("skip test on unix")
			}

			root := rootDirectory(fs.Local{}, filepath.FromSlash(test.target))
			want := filepath.FromSlash(test.root)
			if root != want {
				t.Fatalf("wrong root directory, want %v, got %v", want, root)
			}
		})
	}
}

func TestTree(t *testing.T) {
	var tests = []struct {
		targets   []string
		src       TestDir
		want      tree
		unix      bool
		win       bool
		mustError bool
	}{
		{
			targets: []string{"foo"},
			want: tree{Nodes: map[string]tree{
				"foo": {Path: "foo", Root: "."},
			}},
		},
		{
			targets: []string{"foo", "bar", "baz"},
			want: tree{Nodes: map[string]tree{
				"foo": {Path: "foo", Root: "."},
				"bar": {Path: "bar", Root: "."},
				"baz": {Path: "baz", Root: "."},
			}},
		},
		{
			targets: []string{"foo/user1", "foo/user2", "foo/other"},
			want: tree{Nodes: map[string]tree{
				"foo": {Root: ".", FileInfoPath: "foo", Nodes: map[string]tree{
					"user1": {Path: filepath.FromSlash("foo/user1")},
					"user2": {Path: filepath.FromSlash("foo/user2")},
					"other": {Path: filepath.FromSlash("foo/other")},
				}},
			}},
		},
		{
			targets: []string{"foo/work/user1", "foo/work/user2"},
			want: tree{Nodes: map[string]tree{
				"foo": {Root: ".", FileInfoPath: "foo", Nodes: map[string]tree{
					"work": {FileInfoPath: filepath.FromSlash("foo/work"), Nodes: map[string]tree{
						"user1": {Path: filepath.FromSlash("foo/work/user1")},
						"user2": {Path: filepath.FromSlash("foo/work/user2")},
					}},
				}},
			}},
		},
		{
			targets: []string{"foo/user1", "bar/user1", "foo/other"},
			want: tree{Nodes: map[string]tree{
				"foo": {Root: ".", FileInfoPath: "foo", Nodes: map[string]tree{
					"user1": {Path: filepath.FromSlash("foo/user1")},
					"other": {Path: filepath.FromSlash("foo/other")},
				}},
				"bar": {Root: ".", FileInfoPath: "bar", Nodes: map[string]tree{
					"user1": {Path: filepath.FromSlash("bar/user1")},
				}},
			}},
		},
		{
			targets: []string{"../work"},
			want: tree{Nodes: map[string]tree{
				"work": {Root: "..", Path: filepath.FromSlash("../work")},
			}},
		},
		{
			targets: []string{"../work/other"},
			want: tree{Nodes: map[string]tree{
				"work": {Root: "..", FileInfoPath: filepath.FromSlash("../work"), Nodes: map[string]tree{
					"other": {Path: filepath.FromSlash("../work/other")},
				}},
			}},
		},
		{
			targets: []string{"foo/user1", "../work/other", "foo/user2"},
			want: tree{Nodes: map[string]tree{
				"foo": {Root: ".", FileInfoPath: "foo", Nodes: map[string]tree{
					"user1": {Path: filepath.FromSlash("foo/user1")},
					"user2": {Path: filepath.FromSlash("foo/user2")},
				}},
				"work": {Root: "..", FileInfoPath: filepath.FromSlash("../work"), Nodes: map[string]tree{
					"other": {Path: filepath.FromSlash("../work/other")},
				}},
			}},
		},
		{
			targets: []string{"foo/user1", "../foo/other", "foo/user2"},
			want: tree{Nodes: map[string]tree{
				"foo": {Root: ".", FileInfoPath: "foo", Nodes: map[string]tree{
					"user1": {Path: filepath.FromSlash("foo/user1")},
					"user2": {Path: filepath.FromSlash("foo/user2")},
				}},
				"foo-1": {Root: "..", FileInfoPath: filepath.FromSlash("../foo"), Nodes: map[string]tree{
					"other": {Path: filepath.FromSlash("../foo/other")},
				}},
			}},
		},
		{
			src: TestDir{
				"foo": TestDir{
					"file": TestFile{Content: "file content"},
					"work": TestFile{Content: "work file content"},
				},
			},
			targets: []string{"foo", "foo/work"},
			want: tree{Nodes: map[string]tree{
				"foo": {
					Root:         ".",
					FileInfoPath: "foo",
					Nodes: map[string]tree{
						"file": {Path: filepath.FromSlash("foo/file")},
						"work": {Path: filepath.FromSlash("foo/work")},
					},
				},
			}},
		},
		{
			src: TestDir{
				"foo": TestDir{
					"file": TestFile{Content: "file content"},
					"work": TestDir{
						"other": TestFile{Content: "other file content"},
					},
				},
			},
			targets: []string{"foo/work", "foo"},
			want: tree{Nodes: map[string]tree{
				"foo": {
					Root:         ".",
					FileInfoPath: "foo",
					Nodes: map[string]tree{
						"file": {Path: filepath.FromSlash("foo/file")},
						"work": {Path: filepath.FromSlash("foo/work")},
					},
				},
			}},
		},
		{
			src: TestDir{
				"foo": TestDir{
					"work": TestDir{
						"user1": TestFile{Content: "file content"},
						"user2": TestFile{Content: "other file content"},
					},
				},
			},
			targets: []string{"foo/work", "foo/work/user2"},
			want: tree{Nodes: map[string]tree{
				"foo": {Root: ".", FileInfoPath: "foo", Nodes: map[string]tree{
					"work": {
						FileInfoPath: filepath.FromSlash("foo/work"),
						Nodes: map[string]tree{
							"user1": {Path: filepath.FromSlash("foo/work/user1")},
							"user2": {Path: filepath.FromSlash("foo/work/user2")},
						},
					},
				}},
			}},
		},
		{
			src: TestDir{
				"foo": TestDir{
					"work": TestDir{
						"user1": TestFile{Content: "file content"},
						"user2": TestFile{Content: "other file content"},
					},
				},
			},
			targets: []string{"foo/work/user2", "foo/work"},
			want: tree{Nodes: map[string]tree{
				"foo": {Root: ".", FileInfoPath: "foo", Nodes: map[string]tree{
					"work": {FileInfoPath: filepath.FromSlash("foo/work"),
						Nodes: map[string]tree{
							"user1": {Path: filepath.FromSlash("foo/work/user1")},
							"user2": {Path: filepath.FromSlash("foo/work/user2")},
						},
					},
				}},
			}},
		},
		{
			src: TestDir{
				"foo": TestDir{
					"other": TestFile{Content: "file content"},
					"work": TestDir{
						"user2": TestDir{
							"data": TestDir{
								"secret": TestFile{Content: "secret file content"},
							},
						},
						"user3": TestDir{
							"important.txt": TestFile{Content: "important work"},
						},
					},
				},
			},
			targets: []string{"foo/work/user2/data/secret", "foo"},
			want: tree{Nodes: map[string]tree{
				"foo": {Root: ".", FileInfoPath: "foo", Nodes: map[string]tree{
					"other": {Path: filepath.FromSlash("foo/other")},
					"work": {FileInfoPath: filepath.FromSlash("foo/work"), Nodes: map[string]tree{
						"user2": {FileInfoPath: filepath.FromSlash("foo/work/user2"), Nodes: map[string]tree{
							"data": {FileInfoPath: filepath.FromSlash("foo/work/user2/data"), Nodes: map[string]tree{
								"secret": {
									Path: filepath.FromSlash("foo/work/user2/data/secret"),
								},
							}},
						}},
						"user3": {Path: filepath.FromSlash("foo/work/user3")},
					}},
				}},
			}},
		},
		{
			src: TestDir{
				"mnt": TestDir{
					"driveA": TestDir{
						"work": TestDir{
							"driveB": TestDir{
								"secret": TestFile{Content: "secret file content"},
							},
							"test1": TestDir{
								"important.txt": TestFile{Content: "important work"},
							},
						},
						"test2": TestDir{
							"important.txt": TestFile{Content: "other important work"},
						},
					},
				},
			},
			unix:    true,
			targets: []string{"mnt/driveA", "mnt/driveA/work/driveB"},
			want: tree{Nodes: map[string]tree{
				"mnt": {Root: ".", FileInfoPath: filepath.FromSlash("mnt"), Nodes: map[string]tree{
					"driveA": {FileInfoPath: filepath.FromSlash("mnt/driveA"), Nodes: map[string]tree{
						"work": {FileInfoPath: filepath.FromSlash("mnt/driveA/work"), Nodes: map[string]tree{
							"driveB": {
								Path: filepath.FromSlash("mnt/driveA/work/driveB"),
							},
							"test1": {Path: filepath.FromSlash("mnt/driveA/work/test1")},
						}},
						"test2": {Path: filepath.FromSlash("mnt/driveA/test2")},
					}},
				}},
			}},
		},
		{
			targets: []string{"foo/work/user", "foo/work/user"},
			want: tree{Nodes: map[string]tree{
				"foo": {Root: ".", FileInfoPath: "foo", Nodes: map[string]tree{
					"work": {FileInfoPath: filepath.FromSlash("foo/work"), Nodes: map[string]tree{
						"user": {Path: filepath.FromSlash("foo/work/user")},
					}},
				}},
			}},
		},
		{
			targets: []string{"./foo/work/user", "foo/work/user"},
			want: tree{Nodes: map[string]tree{
				"foo": {Root: ".", FileInfoPath: "foo", Nodes: map[string]tree{
					"work": {FileInfoPath: filepath.FromSlash("foo/work"), Nodes: map[string]tree{
						"user": {Path: filepath.FromSlash("foo/work/user")},
					}},
				}},
			}},
		},
		{
			win:     true,
			targets: []string{`c:\users\foobar\temp`},
			want: tree{Nodes: map[string]tree{
				"c": {Root: `c:\`, FileInfoPath: `c:\`, Nodes: map[string]tree{
					"users": {FileInfoPath: `c:\users`, Nodes: map[string]tree{
						"foobar": {FileInfoPath: `c:\users\foobar`, Nodes: map[string]tree{
							"temp": {Path: `c:\users\foobar\temp`},
						}},
					}},
				}},
			}},
		},
		{
			targets:   []string{"."},
			mustError: true,
		},
		{
			targets:   []string{".."},
			mustError: true,
		},
		{
			targets:   []string{"../.."},
			mustError: true,
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			if test.unix && runtime.GOOS == "windows" {
				t.Skip("skip test on windows")
			}

			if test.win && runtime.GOOS != "windows" {
				t.Skip("skip test on unix")
			}

			tempdir := rtest.TempDir(t)
			TestCreateFiles(t, tempdir, test.src)

			back := rtest.Chdir(t, tempdir)
			defer back()

			tree, err := newTree(fs.Local{}, test.targets)
			if test.mustError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				t.Logf("found expected error: %v", err)
				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if !cmp.Equal(&test.want, tree) {
				t.Error(cmp.Diff(&test.want, tree))
			}
		})
	}
}
