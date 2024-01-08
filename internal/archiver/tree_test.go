package archiver

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/restic/restic/internal/frontend"
	"github.com/restic/restic/internal/restic"
	restictest "github.com/restic/restic/internal/test"
)

// debug.Log requires Tree.String.
var _ fmt.Stringer = Tree{}

func testLazyFileMetadataFromSlash(path string) restic.LazyFileMetadata {
	return frontend.CreateTestLazyFileMetadata(filepath.FromSlash(path))
}

func createTestRootDirectoryFromSlash(path string) restic.RootDirectory {
	return frontend.CreateTestRootDirectory(path)
}

func TestTree(t *testing.T) {
	var tests = []struct {
		targets   []string
		src       TestDir
		want      Tree
		unix      bool
		win       bool
		mustError bool
	}{
		{
			targets: []string{"foo"},
			want: Tree{Nodes: map[string]Tree{
				"foo": {FileMetadata: frontend.CreateTestLazyFileMetadata("foo"), Root: frontend.CreateTestRootDirectory(".")},
			}},
		},
		{
			targets: []string{"foo", "bar", "baz"},
			want: Tree{Nodes: map[string]Tree{
				"foo": {FileMetadata: frontend.CreateTestLazyFileMetadata("foo"), Root: frontend.CreateTestRootDirectory(".")},
				"bar": {FileMetadata: frontend.CreateTestLazyFileMetadata("bar"), Root: frontend.CreateTestRootDirectory(".")},
				"baz": {FileMetadata: frontend.CreateTestLazyFileMetadata("baz"), Root: frontend.CreateTestRootDirectory(".")},
			}},
		},
		{
			targets: []string{"foo/user1", "foo/user2", "foo/other"},
			want: Tree{Nodes: map[string]Tree{
				"foo": {Root: frontend.CreateTestRootDirectory("."), FileInfoPath: frontend.CreateTestRootDirectory("foo"), Nodes: map[string]Tree{
					"user1": {FileMetadata: testLazyFileMetadataFromSlash("foo/user1")},
					"user2": {FileMetadata: testLazyFileMetadataFromSlash("foo/user2")},
					"other": {FileMetadata: testLazyFileMetadataFromSlash("foo/other")},
				}},
			}},
		},
		{
			targets: []string{"foo/work/user1", "foo/work/user2"},
			want: Tree{Nodes: map[string]Tree{
				"foo": {Root: frontend.CreateTestRootDirectory("."), FileInfoPath: frontend.CreateTestRootDirectory("foo"), Nodes: map[string]Tree{
					"work": {FileInfoPath: createTestRootDirectoryFromSlash("foo/work"), Nodes: map[string]Tree{
						"user1": {FileMetadata: testLazyFileMetadataFromSlash("foo/work/user1")},
						"user2": {FileMetadata: testLazyFileMetadataFromSlash("foo/work/user2")},
					}},
				}},
			}},
		},
		{
			targets: []string{"foo/user1", "bar/user1", "foo/other"},
			want: Tree{Nodes: map[string]Tree{
				"foo": {Root: frontend.CreateTestRootDirectory("."), FileInfoPath: frontend.CreateTestRootDirectory("foo"), Nodes: map[string]Tree{
					"user1": {FileMetadata: testLazyFileMetadataFromSlash("foo/user1")},
					"other": {FileMetadata: testLazyFileMetadataFromSlash("foo/other")},
				}},
				"bar": {Root: frontend.CreateTestRootDirectory("."), FileInfoPath: frontend.CreateTestRootDirectory("bar"), Nodes: map[string]Tree{
					"user1": {FileMetadata: testLazyFileMetadataFromSlash("bar/user1")},
				}},
			}},
		},
		{
			targets: []string{"../work"},
			want: Tree{Nodes: map[string]Tree{
				"work": {Root: frontend.CreateTestRootDirectory(".."), FileMetadata: testLazyFileMetadataFromSlash("../work")},
			}},
		},
		{
			targets: []string{"../work/other"},
			want: Tree{Nodes: map[string]Tree{
				"work": {Root: frontend.CreateTestRootDirectory(".."), FileInfoPath: frontend.CreateTestRootDirectory("../work"), Nodes: map[string]Tree{
					"other": {FileMetadata: testLazyFileMetadataFromSlash("../work/other")},
				}},
			}},
		},
		{
			targets: []string{"foo/user1", "../work/other", "foo/user2"},
			want: Tree{Nodes: map[string]Tree{
				"foo": {Root: frontend.CreateTestRootDirectory("."), FileInfoPath: frontend.CreateTestRootDirectory("foo"), Nodes: map[string]Tree{
					"user1": {FileMetadata: testLazyFileMetadataFromSlash("foo/user1")},
					"user2": {FileMetadata: testLazyFileMetadataFromSlash("foo/user2")},
				}},
				"work": {Root: frontend.CreateTestRootDirectory(".."), FileInfoPath: createTestRootDirectoryFromSlash("../work"), Nodes: map[string]Tree{
					"other": {FileMetadata: testLazyFileMetadataFromSlash("../work/other")},
				}},
			}},
		},
		{
			targets: []string{"foo/user1", "../foo/other", "foo/user2"},
			want: Tree{Nodes: map[string]Tree{
				"foo": {Root: frontend.CreateTestRootDirectory("."), FileInfoPath: frontend.CreateTestRootDirectory("foo"), Nodes: map[string]Tree{
					"user1": {FileMetadata: testLazyFileMetadataFromSlash("foo/user1")},
					"user2": {FileMetadata: testLazyFileMetadataFromSlash("foo/user2")},
				}},
				"foo-1": {Root: frontend.CreateTestRootDirectory(".."), FileInfoPath: createTestRootDirectoryFromSlash("../foo"), Nodes: map[string]Tree{
					"other": {FileMetadata: testLazyFileMetadataFromSlash("../foo/other")},
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
			want: Tree{Nodes: map[string]Tree{
				"foo": {
					Root:         frontend.CreateTestRootDirectory("."),
					FileInfoPath: frontend.CreateTestRootDirectory("foo"),
					Nodes: map[string]Tree{
						"file": {FileMetadata: testLazyFileMetadataFromSlash("foo/file")},
						"work": {FileMetadata: testLazyFileMetadataFromSlash("foo/work")},
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
			want: Tree{Nodes: map[string]Tree{
				"foo": {
					Root:         frontend.CreateTestRootDirectory("."),
					FileInfoPath: frontend.CreateTestRootDirectory("foo"),
					Nodes: map[string]Tree{
						"file": {FileMetadata: testLazyFileMetadataFromSlash("foo/file")},
						"work": {FileMetadata: testLazyFileMetadataFromSlash("foo/work")},
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
			want: Tree{Nodes: map[string]Tree{
				"foo": {Root: frontend.CreateTestRootDirectory("."), FileInfoPath: frontend.CreateTestRootDirectory("foo"), Nodes: map[string]Tree{
					"work": {
						FileInfoPath: createTestRootDirectoryFromSlash("foo/work"),
						Nodes: map[string]Tree{
							"user1": {FileMetadata: testLazyFileMetadataFromSlash("foo/work/user1")},
							"user2": {FileMetadata: testLazyFileMetadataFromSlash("foo/work/user2")},
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
			want: Tree{Nodes: map[string]Tree{
				"foo": {Root: frontend.CreateTestRootDirectory("."), FileInfoPath: frontend.CreateTestRootDirectory("foo"), Nodes: map[string]Tree{
					"work": {FileInfoPath: createTestRootDirectoryFromSlash("foo/work"),
						Nodes: map[string]Tree{
							"user1": {FileMetadata: testLazyFileMetadataFromSlash("foo/work/user1")},
							"user2": {FileMetadata: testLazyFileMetadataFromSlash("foo/work/user2")},
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
			want: Tree{Nodes: map[string]Tree{
				"foo": {Root: frontend.CreateTestRootDirectory("."), FileInfoPath: frontend.CreateTestRootDirectory("foo"), Nodes: map[string]Tree{
					"other": {FileMetadata: testLazyFileMetadataFromSlash("foo/other")},
					"work": {FileInfoPath: createTestRootDirectoryFromSlash("foo/work"), Nodes: map[string]Tree{
						"user2": {FileInfoPath: createTestRootDirectoryFromSlash("foo/work/user2"), Nodes: map[string]Tree{
							"data": {FileInfoPath: createTestRootDirectoryFromSlash("foo/work/user2/data"), Nodes: map[string]Tree{
								"secret": {
									FileMetadata: testLazyFileMetadataFromSlash("foo/work/user2/data/secret"),
								},
							}},
						}},
						"user3": {FileMetadata: testLazyFileMetadataFromSlash("foo/work/user3")},
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
			want: Tree{Nodes: map[string]Tree{
				"mnt": {Root: frontend.CreateTestRootDirectory("."), FileInfoPath: createTestRootDirectoryFromSlash("mnt"), Nodes: map[string]Tree{
					"driveA": {FileInfoPath: createTestRootDirectoryFromSlash("mnt/driveA"), Nodes: map[string]Tree{
						"work": {FileInfoPath: createTestRootDirectoryFromSlash("mnt/driveA/work"), Nodes: map[string]Tree{
							"driveB": {
								FileMetadata: testLazyFileMetadataFromSlash("mnt/driveA/work/driveB"),
							},
							"test1": {FileMetadata: testLazyFileMetadataFromSlash("mnt/driveA/work/test1")},
						}},
						"test2": {FileMetadata: testLazyFileMetadataFromSlash("mnt/driveA/test2")},
					}},
				}},
			}},
		},
		{
			targets: []string{"foo/work/user", "foo/work/user"},
			want: Tree{Nodes: map[string]Tree{
				"foo": {Root: frontend.CreateTestRootDirectory("."), FileInfoPath: frontend.CreateTestRootDirectory("foo"), Nodes: map[string]Tree{
					"work": {FileInfoPath: createTestRootDirectoryFromSlash("foo/work"), Nodes: map[string]Tree{
						"user": {FileMetadata: testLazyFileMetadataFromSlash("foo/work/user")},
					}},
				}},
			}},
		},
		{
			targets: []string{"./foo/work/user", "foo/work/user"},
			want: Tree{Nodes: map[string]Tree{
				"foo": {Root: frontend.CreateTestRootDirectory("."), FileInfoPath: frontend.CreateTestRootDirectory("foo"), Nodes: map[string]Tree{
					"work": {FileInfoPath: createTestRootDirectoryFromSlash("foo/work"), Nodes: map[string]Tree{
						"user": {FileMetadata: testLazyFileMetadataFromSlash("foo/work/user")},
					}},
				}},
			}},
		},
		{
			win:     true,
			targets: []string{`c:\users\foobar\temp`},
			want: Tree{Nodes: map[string]Tree{
				"c": {Root: frontend.CreateTestRootDirectory(`c:\`), FileInfoPath: frontend.CreateTestRootDirectory(`c:\`), Nodes: map[string]Tree{
					"users": {FileInfoPath: frontend.CreateTestRootDirectory(`c:\users`), Nodes: map[string]Tree{
						"foobar": {FileInfoPath: frontend.CreateTestRootDirectory(`c:\users\foobar`), Nodes: map[string]Tree{
							"temp": {FileMetadata: frontend.CreateTestLazyFileMetadata(`c:\users\foobar\temp`)},
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

			tempdir := restictest.TempDir(t)
			TestCreateFiles(t, tempdir, test.src)

			back := restictest.Chdir(t, tempdir)
			defer back()
			pathTargets := make([]restic.LazyFileMetadata, len(test.targets))
			for i, target := range test.targets {
				pathTargets[i] = frontend.CreateTestLazyFileMetadata(target)
			}
			tree, err := newTree(pathTargets)
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
