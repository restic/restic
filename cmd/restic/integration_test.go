// +build integration

package main

import (
	"flag"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	. "github.com/restic/restic/test"
)

var TestDataFile = flag.String("test.datafile", "", `specify tar.gz file with test data to backup and restore (required for integration test)`)

func setupTempdir(t testing.TB) (tempdir string) {
	tempdir, err := ioutil.TempDir(*TestTempDir, "restic-test-")
	OK(t, err)

	return tempdir
}

func configureRestic(t testing.TB, tempdir string) {
	// use cache dir within tempdir
	OK(t, os.Setenv("RESTIC_CACHE", filepath.Join(tempdir, "cache")))

	// configure environment
	opts.Repo = filepath.Join(tempdir, "repo")
	OK(t, os.Setenv("RESTIC_PASSWORD", *TestPassword))
}

func cleanupTempdir(t testing.TB, tempdir string) {
	if !*TestCleanup {
		t.Logf("leaving temporary directory %v used for test", tempdir)
		return
	}

	OK(t, os.RemoveAll(tempdir))
}

func setupTarTestFixture(t testing.TB, outputDir, tarFile string) {
	err := system("sh", "-c", `mkdir "$1" && (cd "$1" && tar xz) < "$2"`,
		"sh", outputDir, tarFile)
	OK(t, err)
}

func system(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func cmdInit(t testing.TB) {
	cmd := &CmdInit{}
	OK(t, cmd.Execute(nil))

	t.Logf("repository initialized at %v", opts.Repo)
}

func cmdBackup(t testing.TB, target []string) {
	cmd := &CmdBackup{}
	t.Logf("backing up %v", target)

	OK(t, cmd.Execute(target))
}

func TestBackup(t *testing.T) {
	if *TestDataFile == "" {
		t.Fatal("no data tar file specified, use flag '-test.datafile'")
	}

	tempdir := setupTempdir(t)
	defer cleanupTempdir(t, tempdir)

	configureRestic(t, tempdir)

	cmdInit(t)

	datadir := filepath.Join(tempdir, "testdata")

	setupTarTestFixture(t, datadir, *TestDataFile)

	cmdBackup(t, []string{datadir})
}
