package global

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"github.com/restic/restic/internal/errors"
	rtest "github.com/restic/restic/internal/test"
)

func TestReadRepo(t *testing.T) {
	tempDir := rtest.TempDir(t)

	// test --repo option
	var gopts Options
	gopts.Repo = tempDir
	repo, err := readRepo(gopts)
	rtest.OK(t, err)
	rtest.Equals(t, tempDir, repo)

	// test --repository-file option
	foo := filepath.Join(tempDir, "foo")
	err = os.WriteFile(foo, []byte(tempDir+"\n"), 0666)
	rtest.OK(t, err)

	var gopts2 Options
	gopts2.RepositoryFile = foo
	repo, err = readRepo(gopts2)
	rtest.OK(t, err)
	rtest.Equals(t, tempDir, repo)

	var gopts3 Options
	gopts3.RepositoryFile = foo + "-invalid"
	_, err = readRepo(gopts3)
	if err == nil {
		t.Fatal("must not read repository path from invalid file path")
	}
}

func TestReadEmptyPassword(t *testing.T) {
	opts := Options{InsecureNoPassword: true}
	password, err := readPassword(context.TODO(), opts, "test")
	rtest.OK(t, err)
	rtest.Equals(t, "", password, "got unexpected password")

	opts.Password = "invalid"
	_, err = readPassword(context.TODO(), opts, "test")
	rtest.Assert(t, strings.Contains(err.Error(), "must not be specified together with providing a password via a cli option or environment variable"), "unexpected error message, got %v", err)
}

func TestPackSizeEnvParseError(t *testing.T) {
	t.Setenv("RESTIC_PACK_SIZE", "64MiB")

	var gopts Options
	gopts.AddFlags(pflag.NewFlagSet("test", pflag.ContinueOnError))

	err := gopts.PreRun(false)
	rtest.Assert(t, err != nil, "expected error for invalid pack size env")
	rtest.Assert(t, errors.IsFatal(err), "expected fatal error for invalid pack size env, got %T", err)
	rtest.Assert(t, strings.Contains(err.Error(), "RESTIC_PACK_SIZE"), "error should mention RESTIC_PACK_SIZE, got %v", err)
}

func TestPackSizeEnvApplied(t *testing.T) {
	t.Setenv("RESTIC_PACK_SIZE", "64")

	var gopts Options
	gopts.AddFlags(pflag.NewFlagSet("test", pflag.ContinueOnError))

	err := gopts.PreRun(false)
	rtest.OK(t, err)
	rtest.Equals(t, uint(64), gopts.PackSize)
}

func TestPackSizeEnvIgnoredWhenFlagSet(t *testing.T) {
	t.Setenv("RESTIC_PACK_SIZE", "64MiB")

	var gopts Options
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	gopts.AddFlags(fs)

	err := fs.Set("pack-size", "64")
	rtest.OK(t, err)

	err = gopts.PreRun(false)
	rtest.OK(t, err)
	rtest.Equals(t, uint(64), gopts.PackSize)
}
