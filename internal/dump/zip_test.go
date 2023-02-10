package dump

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/fs"
)

func TestWriteZip(t *testing.T) {
	WriteTest(t, "zip", checkZip)
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}

	b := &bytes.Buffer{}
	_, err = b.ReadFrom(rc)
	if err != nil {
		// ignore subsequent errors
		_ = rc.Close()
		return nil, err
	}

	err = rc.Close()
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func checkZip(t *testing.T, testDir string, srcZip *bytes.Buffer) error {
	z, err := zip.NewReader(bytes.NewReader(srcZip.Bytes()), int64(srcZip.Len()))
	if err != nil {
		return err
	}

	fileNumber := 0
	zipFiles := len(z.File)

	err = filepath.Walk(testDir, func(path string, info os.FileInfo, err error) error {
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

	for _, f := range z.File {
		matchPath := filepath.Join(testDir, f.Name)
		match, err := os.Lstat(matchPath)
		if err != nil {
			return err
		}

		// check metadata, zip header contains time rounded to seconds
		fileTime := match.ModTime().Truncate(time.Second)
		zipTime := f.Modified
		if !fileTime.Equal(zipTime) {
			return fmt.Errorf("modTime does not match, got: %s, want: %s", zipTime, fileTime)
		}
		if f.Mode() != match.Mode() {
			return fmt.Errorf("mode does not match, got: %v [%08x], want: %v [%08x]",
				f.Mode(), uint32(f.Mode()), match.Mode(), uint32(match.Mode()))
		}
		t.Logf("Mode is %v [%08x] for %s", f.Mode(), uint32(f.Mode()), f.Name)

		switch {
		case f.FileInfo().IsDir():
			filebase := filepath.ToSlash(match.Name())
			if filepath.Base(f.Name) != filebase {
				return fmt.Errorf("foldernames don't match got %v want %v", filepath.Base(f.Name), filebase)
			}
			if !strings.HasSuffix(f.Name, "/") {
				return fmt.Errorf("foldernames must end with separator got %v", f.Name)
			}
		case f.Mode()&os.ModeSymlink != 0:
			target, err := fs.Readlink(matchPath)
			if err != nil {
				return err
			}
			linkName, err := readZipFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if target != string(linkName) {
				return fmt.Errorf("symlink target does not match, got %s want %s", string(linkName), target)
			}
		default:
			if uint64(match.Size()) != f.UncompressedSize64 {
				return fmt.Errorf("size does not match got %v want %v", f.UncompressedSize64, match.Size())
			}
			contentsFile, err := os.ReadFile(matchPath)
			if err != nil {
				t.Fatal(err)
			}
			contentsZip, err := readZipFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if string(contentsZip) != string(contentsFile) {
				return fmt.Errorf("contents does not match, got %s want %s", contentsZip, contentsFile)
			}
		}
	}

	if zipFiles != fileNumber {
		return fmt.Errorf("not the same amount of files got %v want %v", zipFiles, fileNumber)
	}

	return nil
}
