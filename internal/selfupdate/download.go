package selfupdate

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/bzip2"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"
)

func findHash(buf []byte, filename string) (hash []byte, err error) {
	sc := bufio.NewScanner(bytes.NewReader(buf))
	for sc.Scan() {
		data := strings.Split(sc.Text(), "  ")
		if len(data) != 2 {
			continue
		}

		if data[1] == filename {
			h, err := hex.DecodeString(data[0])
			if err != nil {
				return nil, err
			}

			return h, nil
		}
	}

	return nil, fmt.Errorf("hash for file %v not found", filename)
}

func extractToFile(buf []byte, filename, target string, printf func(string, ...interface{})) error {
	var mode = os.FileMode(0755)

	// get information about the target file
	fi, err := os.Lstat(target)
	if err == nil {
		mode = fi.Mode()
	}

	var rd io.Reader = bytes.NewReader(buf)
	switch filepath.Ext(filename) {
	case ".bz2":
		rd = bzip2.NewReader(rd)
	case ".zip":
		zrd, err := zip.NewReader(bytes.NewReader(buf), int64(len(buf)))
		if err != nil {
			return err
		}

		if len(zrd.File) != 1 {
			return errors.New("ZIP archive contains more than one file")
		}

		file, err := zrd.File[0].Open()
		if err != nil {
			return err
		}

		defer func() {
			_ = file.Close()
		}()

		rd = file
	}

	err = os.Remove(target)
	if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		return fmt.Errorf("unable to remove target file: %v", err)
	}

	dest, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}

	n, err := io.Copy(dest, rd)
	if err != nil {
		_ = dest.Close()
		_ = os.Remove(dest.Name())
		return err
	}

	err = dest.Close()
	if err != nil {
		return err
	}

	printf("saved %d bytes in %v\n", n, dest.Name())
	return nil
}

// DownloadLatestStableRelease downloads the latest stable released version of
// restic and saves it to target. It returns the version string for the newest
// version. The function printf is used to print progress information.
func DownloadLatestStableRelease(ctx context.Context, target, currentVersion string, printf func(string, ...interface{})) (version string, err error) {
	if printf == nil {
		printf = func(string, ...interface{}) {}
	}

	printf("find latest release of restic at GitHub\n")

	rel, err := GitHubLatestRelease(ctx, "restic", "restic")
	if err != nil {
		return "", err
	}

	if rel.Version == currentVersion {
		printf("restic is up to date\n")
		return currentVersion, nil
	}

	printf("latest version is %v\n", rel.Version)

	_, sha256sums, err := getGithubDataFile(ctx, rel.Assets, "SHA256SUMS", printf)
	if err != nil {
		return "", err
	}

	_, sig, err := getGithubDataFile(ctx, rel.Assets, "SHA256SUMS.asc", printf)
	if err != nil {
		return "", err
	}

	ok, err := GPGVerify(sha256sums, sig)
	if err != nil {
		return "", err
	}

	if !ok {
		return "", errors.New("GPG signature verification of the file SHA256SUMS failed")
	}

	printf("GPG signature verification succeeded\n")

	ext := "bz2"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}

	suffix := fmt.Sprintf("%s_%s.%s", runtime.GOOS, runtime.GOARCH, ext)
	downloadFilename, buf, err := getGithubDataFile(ctx, rel.Assets, suffix, printf)
	if err != nil {
		return "", err
	}

	printf("downloaded %v\n", downloadFilename)

	wantHash, err := findHash(sha256sums, downloadFilename)
	if err != nil {
		return "", err
	}

	gotHash := sha256.Sum256(buf)
	if !bytes.Equal(wantHash, gotHash[:]) {
		return "", fmt.Errorf("SHA256 hash mismatch, want hash %02x, got %02x", wantHash, gotHash)
	}

	err = extractToFile(buf, downloadFilename, target, printf)
	if err != nil {
		return "", err
	}

	return rel.Version, nil
}
