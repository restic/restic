package dump

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestWriteTar(t *testing.T) {
	WriteTest(t, "tar", checkTar)
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
			contentsFile, err := os.ReadFile(matchPath)
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

// #4307.
func TestFieldTooLong(t *testing.T) {
	const maxSpecialFileSize = 1 << 20 // Unexported limit in archive/tar.

	node := restic.Node{
		Name: "file_with_xattr",
		Path: "/file_with_xattr",
		Type: "file",
		Mode: 0644,
		ExtendedAttributes: []restic.ExtendedAttribute{
			{
				Name:  "user.way_too_large",
				Value: make([]byte, 2*maxSpecialFileSize),
			},
		},
	}

	d := Dumper{format: "tar"}
	err := d.dumpNodeTar(context.Background(), &node, tar.NewWriter(io.Discard))

	// We want a tar.ErrFieldTooLong that has the filename.
	rtest.Assert(t, errors.Is(err, tar.ErrFieldTooLong), "wrong type %T", err)
	rtest.Assert(t, strings.Contains(err.Error(), node.Path),
		"no filename in %q", err)
}
