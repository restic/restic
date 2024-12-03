//go:build windows
// +build windows

package archiver

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/restic/restic/internal/checker"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestArchiverSnapshotWithAds(t *testing.T) {
	// The toplevel directory is not counted in the ItemStats
	var tests = []struct {
		name    string
		src     TestDir
		targets []string
		want    TestDir
		stat    ItemStats
		exclude []string
	}{
		{
			name: "Ads_directory_Basic",
			src: TestDir{
				"dir": TestDir{
					"targetfile.txt":               TestFile{Content: string("foobar")},
					"targetfile.txt:Stream1:$DATA": TestFile{Content: string("stream 1")},
					"targetfile.txt:Stream2:$DATA": TestFile{Content: string("stream 2")},
				},
			},
			targets: []string{"dir"},
			stat:    ItemStats{3, 22, 246 + 22, 2, 0, 768},
		},
		{
			name: "Ads_folder_with_dir_streams",
			src: TestDir{
				"dir": TestDir{
					":Stream1:$DATA": TestFile{Content: string("stream 1")},
					":Stream2:$DATA": TestFile{Content: string("stream 2")},
				},
			},
			targets: []string{"dir"},
			want: TestDir{
				"dir":               TestDir{},
				"dir:Stream1:$DATA": TestFile{Content: string("stream 1")},
				"dir:Stream2:$DATA": TestFile{Content: string("stream 2")},
			},
			stat: ItemStats{2, 16, 164 + 16, 2, 0, 563},
		},
		{
			name: "single_Ads_file",
			src: TestDir{
				"targetfile.txt":               TestFile{Content: string("foobar")},
				"targetfile.txt:Stream1:$DATA": TestFile{Content: string("stream 1")},
				"targetfile.txt:Stream2:$DATA": TestFile{Content: string("stream 2")},
			},
			targets: []string{"targetfile.txt"},
			stat:    ItemStats{3, 22, 246 + 22, 1, 0, 457},
		},
		{
			name: "Ads_all_types",
			src: TestDir{
				"dir": TestDir{
					"adsfile.txt":               TestFile{Content: string("foobar")},
					"adsfile.txt:Stream1:$DATA": TestFile{Content: string("stream 1")},
					"adsfile.txt:Stream2:$DATA": TestFile{Content: string("stream 2")},
					":dirstream1:$DATA":         TestFile{Content: string("stream 3")},
					":dirstream2:$DATA":         TestFile{Content: string("stream 4")},
				},
				"targetfile.txt":               TestFile{Content: string("foobar")},
				"targetfile.txt:Stream1:$DATA": TestFile{Content: string("stream 1")},
				"targetfile.txt:Stream2:$DATA": TestFile{Content: string("stream 2")},
			},
			want: TestDir{
				"dir": TestDir{
					"adsfile.txt":               TestFile{Content: string("foobar")},
					"adsfile.txt:Stream1:$DATA": TestFile{Content: string("stream 1")},
					"adsfile.txt:Stream2:$DATA": TestFile{Content: string("stream 2")},
				},
				"dir:dirstream1:$DATA":         TestFile{Content: string("stream 3")},
				"dir:dirstream2:$DATA":         TestFile{Content: string("stream 4")},
				"targetfile.txt":               TestFile{Content: string("foobar")},
				"targetfile.txt:Stream1:$DATA": TestFile{Content: string("stream 1")},
				"targetfile.txt:Stream2:$DATA": TestFile{Content: string("stream 2")},
			},
			targets: []string{"targetfile.txt", "dir"},
			stat:    ItemStats{5, 38, 410 + 38, 2, 0, 1133},
		},
		{
			name: "Ads_directory_exclusion",
			src: TestDir{
				"dir": TestDir{
					"adsfile.txt":               TestFile{Content: string("foobar")},
					"adsfile.txt:Stream1:$DATA": TestFile{Content: string("stream 1")},
					"adsfile.txt:Stream2:$DATA": TestFile{Content: string("stream 2")},
					":dirstream1:$DATA":         TestFile{Content: string("stream 3")},
					":dirstream2:$DATA":         TestFile{Content: string("stream 4")},
				},
				"targetfile.txt":               TestFile{Content: string("foobar")},
				"targetfile.txt:Stream1:$DATA": TestFile{Content: string("stream 1")},
				"targetfile.txt:Stream2:$DATA": TestFile{Content: string("stream 2")},
			},
			want: TestDir{
				"targetfile.txt":               TestFile{Content: string("foobar")},
				"targetfile.txt:Stream1:$DATA": TestFile{Content: string("stream 1")},
				"targetfile.txt:Stream2:$DATA": TestFile{Content: string("stream 2")},
			},
			targets: []string{"targetfile.txt", "dir"},
			exclude: []string{"*\\dir*"},
			stat:    ItemStats{3, 22, 268, 1, 0, 1133},
		},
		{
			name: "Ads_backup_file_exclusion",
			src: TestDir{
				"dir": TestDir{
					"adsfile.txt":               TestFile{Content: string("foobar")},
					"adsfile.txt:Stream1:$DATA": TestFile{Content: string("stream 1")},
					"adsfile.txt:Stream2:$DATA": TestFile{Content: string("stream 2")},
					":dirstream1:$DATA":         TestFile{Content: string("stream 3")},
					":dirstream2:$DATA":         TestFile{Content: string("stream 4")},
				},
				"targetfile.txt":               TestFile{Content: string("foobar")},
				"targetfile.txt:Stream1:$DATA": TestFile{Content: string("stream 1")},
				"targetfile.txt:Stream2:$DATA": TestFile{Content: string("stream 2")},
			},
			want: TestDir{
				"dir":                          TestDir{},
				"dir:dirstream1:$DATA":         TestFile{Content: string("stream 3")},
				"dir:dirstream2:$DATA":         TestFile{Content: string("stream 4")},
				"targetfile.txt":               TestFile{Content: string("foobar")},
				"targetfile.txt:Stream1:$DATA": TestFile{Content: string("stream 1")},
				"targetfile.txt:Stream2:$DATA": TestFile{Content: string("stream 2")},
			},
			targets: []string{"targetfile.txt", "dir"},
			exclude: []string{"*\\dir\\adsfile.txt"},
			stat:    ItemStats{5, 38, 448, 2, 0, 2150},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tempdir, repo := prepareTempdirRepoSrc(t, test.src)

			testFS := fs.Track{FS: fs.Local{}}

			arch := New(repo, testFS, Options{})

			if len(test.exclude) != 0 {
				parsedPatterns := filter.ParsePatterns(test.exclude)
				arch.SelectByName = func(item string) bool {
					//if
					if matched, err := filter.List(parsedPatterns, item); err == nil && matched {
						return false
					} else {
						return true
					}

				}
			}

			var stat *ItemStats = &ItemStats{}
			lock := &sync.Mutex{}

			arch.CompleteItem = func(item string, previous, current *restic.Node, s ItemStats, d time.Duration) {
				lock.Lock()
				defer lock.Unlock()
				stat.Add(s)
			}
			back := rtest.Chdir(t, tempdir)
			defer back()

			var targets []string
			for _, target := range test.targets {
				targets = append(targets, os.ExpandEnv(target))
			}

			sn, snapshotID, _, err := arch.Snapshot(ctx, targets, SnapshotOptions{Time: time.Now(), Excludes: test.exclude})
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("saved as %v", snapshotID.Str())

			want := test.want
			if want == nil {
				want = test.src
			}

			TestEnsureSnapshot(t, repo, snapshotID, want)

			checker.TestCheckRepo(t, repo, false)

			// check that the snapshot contains the targets with absolute paths
			for i, target := range sn.Paths {
				atarget, err := filepath.Abs(test.targets[i])
				if err != nil {
					t.Fatal(err)
				}

				if target != atarget {
					t.Errorf("wrong path in snapshot: want %v, got %v", atarget, target)
				}
			}

			rtest.Equals(t, uint64(test.stat.DataBlobs), uint64(stat.DataBlobs))
			rtest.Equals(t, uint64(test.stat.TreeBlobs), uint64(stat.TreeBlobs))
			rtest.Equals(t, test.stat.DataSize, stat.DataSize)
			rtest.Equals(t, test.stat.DataSizeInRepo, stat.DataSizeInRepo)
		})
	}
}
