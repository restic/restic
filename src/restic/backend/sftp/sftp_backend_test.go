package sftp_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"restic"
	"strings"

	"restic/errors"

	"restic/backend/sftp"
	"restic/backend/test"

	. "restic/test"
)

var tempBackendDir string

//go:generate go run ../test/generate_backend_tests.go

func createTempdir() error {
	if tempBackendDir != "" {
		return nil
	}

	tempdir, err := ioutil.TempDir("", "restic-local-test-")
	if err != nil {
		return err
	}

	tempBackendDir = tempdir
	return nil
}

func init() {
	sftpserver := ""

	for _, dir := range strings.Split(TestSFTPPath, ":") {
		testpath := filepath.Join(dir, "sftp-server")
		_, err := os.Stat(testpath)
		if !os.IsNotExist(errors.Cause(err)) {
			sftpserver = testpath
			break
		}
	}

	if sftpserver == "" {
		SkipMessage = "sftp server binary not found, skipping tests"
		return
	}

	cfg := sftp.Config{
		Command: fmt.Sprintf("%q -e", sftpserver),
	}

	test.CreateFn = func() (restic.Backend, error) {
		err := createTempdir()
		if err != nil {
			return nil, err
		}

		cfg.Dir = tempBackendDir

		return sftp.Create(cfg)
	}

	test.OpenFn = func() (restic.Backend, error) {
		err := createTempdir()
		if err != nil {
			return nil, err
		}

		cfg.Dir = tempBackendDir

		return sftp.Open(cfg)
	}

	test.CleanupFn = func() error {
		if tempBackendDir == "" {
			return nil
		}

		err := os.RemoveAll(tempBackendDir)
		tempBackendDir = ""
		return err
	}
}
