package cache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestNew(t *testing.T) {
	parent := rtest.TempDir(t)
	basedir := filepath.Join(parent, "cache")
	id := restic.NewRandomID().String()
	tagFile := filepath.Join(basedir, "CACHEDIR.TAG")
	versionFile := filepath.Join(basedir, id, "version")

	const (
		stepCreate = iota
		stepComplete
		stepRmTag
		stepRmVersion
		stepEnd
	)

	for step := stepCreate; step < stepEnd; step++ {
		switch step {
		case stepRmTag:
			rtest.OK(t, os.Remove(tagFile))
		case stepRmVersion:
			rtest.OK(t, os.Remove(versionFile))
		}

		c, err := New(id, basedir)
		rtest.OK(t, err)
		rtest.Equals(t, basedir, c.Base)
		rtest.Equals(t, step == stepCreate, c.Created)

		for _, name := range []string{tagFile, versionFile} {
			info, err := os.Lstat(name)
			rtest.OK(t, err)
			rtest.Assert(t, info.Mode().IsRegular(), "")
		}
	}
}
