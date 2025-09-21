package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestReadRepo(t *testing.T) {
	tempDir := rtest.TempDir(t)

	// test --repo option
	var gopts GlobalOptions
	gopts.Repo = tempDir
	repo, err := ReadRepo(gopts)
	rtest.OK(t, err)
	rtest.Equals(t, tempDir, repo)

	// test --repository-file option
	foo := filepath.Join(tempDir, "foo")
	err = os.WriteFile(foo, []byte(tempDir+"\n"), 0666)
	rtest.OK(t, err)

	var gopts2 GlobalOptions
	gopts2.RepositoryFile = foo
	repo, err = ReadRepo(gopts2)
	rtest.OK(t, err)
	rtest.Equals(t, tempDir, repo)

	var gopts3 GlobalOptions
	gopts3.RepositoryFile = foo + "-invalid"
	_, err = ReadRepo(gopts3)
	if err == nil {
		t.Fatal("must not read repository path from invalid file path")
	}
}

func TestReadEmptyPassword(t *testing.T) {
	opts := GlobalOptions{InsecureNoPassword: true}
	password, err := ReadPassword(context.TODO(), opts, "test")
	rtest.OK(t, err)
	rtest.Equals(t, "", password, "got unexpected password")

	opts.password = "invalid"
	_, err = ReadPassword(context.TODO(), opts, "test")
	rtest.Assert(t, strings.Contains(err.Error(), "must not be specified together with providing a password via a cli option or environment variable"), "unexpected error message, got %v", err)
}
