package main

import (
	"os"
	"path/filepath"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func Test_PrintFunctionsRespectsGlobalStdout(t *testing.T) {
	for _, p := range []func(){
		func() { Println("message") },
		func() { Print("message\n") },
		func() { Printf("mes%s\n", "sage") },
	} {
		buf, _ := withCaptureStdout(func() error {
			p()
			return nil
		})
		rtest.Equals(t, "message\n", buf.String())
	}
}

func TestReadRepo(t *testing.T) {
	tempDir := rtest.TempDir(t)

	// test --repo option
	var opts GlobalOptions
	opts.Repo = tempDir
	repo, err := ReadRepo(opts)
	rtest.OK(t, err)
	rtest.Equals(t, tempDir, repo)

	// test --repository-file option
	foo := filepath.Join(tempDir, "foo")
	err = os.WriteFile(foo, []byte(tempDir+"\n"), 0666)
	rtest.OK(t, err)

	var opts2 GlobalOptions
	opts2.RepositoryFile = foo
	repo, err = ReadRepo(opts2)
	rtest.OK(t, err)
	rtest.Equals(t, tempDir, repo)

	var opts3 GlobalOptions
	opts3.RepositoryFile = foo + "-invalid"
	_, err = ReadRepo(opts3)
	if err == nil {
		t.Fatal("must not read repository path from invalid file path")
	}
}
