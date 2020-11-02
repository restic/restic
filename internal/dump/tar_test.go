package dump

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func prepareTempdirRepoSrc(t testing.TB, src archiver.TestDir) (tempdir string, repo restic.Repository, cleanup func()) {
	tempdir, removeTempdir := rtest.TempDir(t)
	repo, removeRepository := repository.TestRepository(t)

	archiver.TestCreateFiles(t, tempdir, src)

	cleanup = func() {
		removeRepository()
		removeTempdir()
	}

	return tempdir, repo, cleanup
}

func TestWriteTar(t *testing.T) {
	tests := []struct {
		name   string
		args   archiver.TestDir
		target string
	}{
		{
			name: "single file in root",
			args: archiver.TestDir{
				"file": archiver.TestFile{Content: "string"},
			},
			target: "/",
		},
		{
			name: "multiple files in root",
			args: archiver.TestDir{
				"file1": archiver.TestFile{Content: "string"},
				"file2": archiver.TestFile{Content: "string"},
			},
			target: "/",
		},
		{
			name: "multiple files and folders in root",
			args: archiver.TestDir{
				"file1": archiver.TestFile{Content: "string"},
				"file2": archiver.TestFile{Content: "string"},
				"firstDir": archiver.TestDir{
					"another": archiver.TestFile{Content: "string"},
				},
				"secondDir": archiver.TestDir{
					"another2": archiver.TestFile{Content: "string"},
				},
			},
			target: "/",
		},
		{
			name: "file and symlink in root",
			args: archiver.TestDir{
				"file1": archiver.TestFile{Content: "string"},
				"file2": archiver.TestSymlink{Target: "file1"},
			},
			target: "/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tmpdir, repo, cleanup := prepareTempdirRepoSrc(t, tt.args)
			defer cleanup()

			arch := archiver.New(repo, fs.Track{FS: fs.Local{}}, archiver.Options{})

			back := rtest.Chdir(t, tmpdir)
			defer back()

			sn, _, err := arch.Snapshot(ctx, []string{"."}, archiver.SnapshotOptions{})
			rtest.OK(t, err)

			tree, err := repo.LoadTree(ctx, *sn.Tree)
			rtest.OK(t, err)

			dst := &bytes.Buffer{}
			if err := WriteTar(ctx, repo, tree, tt.target, dst); err != nil {
				t.Fatalf("WriteTar() error = %v", err)
			}
			if err := checkTar(t, tmpdir, dst); err != nil {
				t.Errorf("WriteTar() = tar does not match: %v", err)
			}
		})
	}
}

func checkTar(t *testing.T, testDir string, srcTar *bytes.Buffer) error {
	tr := tar.NewReader(srcTar)

	fileNumber := 0
	tarFiles := 0

	err := filepath.Walk(testDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Name() != filepath.Base(testDir) {
			fileNumber++
		}
		return nil
	})
	if err != nil {
		return err
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}

		matchPath := filepath.Join(testDir, hdr.Name)
		match, err := os.Lstat(matchPath)
		if err != nil {
			return err
		}

		// check metadata, tar header contains time rounded to seconds
		fileTime := match.ModTime().Round(time.Second)
		tarTime := hdr.ModTime
		if !fileTime.Equal(tarTime) {
			return fmt.Errorf("modTime does not match, got: %s, want: %s", fileTime, tarTime)
		}

		if os.FileMode(hdr.Mode).Perm() != match.Mode().Perm() || os.FileMode(hdr.Mode)&^os.ModePerm != 0 {
			return fmt.Errorf("mode does not match, got: %v, want: %v", os.FileMode(hdr.Mode), match.Mode())
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			// this is a folder
			if hdr.Name == "." {
				// we don't need to check the root folder
				continue
			}

			filebase := filepath.ToSlash(match.Name())
			if filepath.Base(hdr.Name) != filebase {
				return fmt.Errorf("foldernames don't match got %v want %v", filepath.Base(hdr.Name), filebase)
			}
			if !strings.HasSuffix(hdr.Name, "/") {
				return fmt.Errorf("foldernames must end with separator got %v", hdr.Name)
			}
		case tar.TypeSymlink:
			target, err := fs.Readlink(matchPath)
			if err != nil {
				return err
			}
			if target != hdr.Linkname {
				return fmt.Errorf("symlink target does not match, got %s want %s", target, hdr.Linkname)
			}
		default:
			if match.Size() != hdr.Size {
				return fmt.Errorf("size does not match got %v want %v", hdr.Size, match.Size())
			}
			contentsFile, err := ioutil.ReadFile(matchPath)
			if err != nil {
				t.Fatal(err)
			}
			contentsTar := &bytes.Buffer{}
			_, err = io.Copy(contentsTar, tr)
			if err != nil {
				t.Fatal(err)
			}
			if contentsTar.String() != string(contentsFile) {
				return fmt.Errorf("contents does not match, got %s want %s", contentsTar, contentsFile)
			}
		}
		tarFiles++
	}

	if tarFiles != fileNumber {
		return fmt.Errorf("not the same amount of files got %v want %v", tarFiles, fileNumber)
	}

	return nil
}
