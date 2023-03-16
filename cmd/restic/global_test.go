package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/test"
	rtest "github.com/restic/restic/internal/test"
)

func Test_PrintFunctionsRespectsGlobalStdout(t *testing.T) {
	gopts := globalOptions
	defer func() {
		globalOptions = gopts
	}()

	buf := bytes.NewBuffer(nil)
	globalOptions.stdout = buf

	for _, p := range []func(){
		func() { Println("message") },
		func() { Print("message\n") },
		func() { Printf("mes%s\n", "sage") },
	} {
		p()
		rtest.Equals(t, "message\n", buf.String())
		buf.Reset()
	}
}

func TestReadRepo(t *testing.T) {
	tempDir := test.TempDir(t)

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
