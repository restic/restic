package restic

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
)

type Cache struct {
	base string
}

func NewCache(be backend.IDer) (c *Cache, err error) {
	// try to get explicit cache dir from environment
	dir := os.Getenv("RESTIC_CACHE")

	// otherwise try OS specific default
	if dir == "" {
		dir, err = GetCacheDir()
		if err != nil {
			return nil, err
		}
	}

	basedir := filepath.Join(dir, be.ID().String())
	debug.Log("Cache.New", "opened cache at %v", basedir)

	return &Cache{base: basedir}, nil
}

func (c *Cache) Has(t backend.Type, subtype string, id backend.ID) (bool, error) {
	// try to open file
	filename, err := c.filename(t, subtype, id)
	if err != nil {
		return false, err
	}

	fd, err := os.Open(filename)
	defer fd.Close()
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func (c *Cache) Store(t backend.Type, subtype string, id backend.ID) (io.WriteCloser, error) {
	filename, err := c.filename(t, subtype, id)
	if err != nil {
		return nil, err
	}

	dirname := filepath.Dir(filename)
	err = os.MkdirAll(dirname, 0700)
	if err != nil {
		return nil, err
	}

	file, err := os.Create(filename)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (c *Cache) Load(t backend.Type, subtype string, id backend.ID) (io.ReadCloser, error) {
	// try to open file
	filename, err := c.filename(t, subtype, id)
	if err != nil {
		return nil, err
	}

	return os.Open(filename)
}

// Construct file name for given Type.
func (c *Cache) filename(t backend.Type, subtype string, id backend.ID) (string, error) {
	filename := id.String()
	if subtype != "" {
		filename += "." + subtype
	}

	switch t {
	case backend.Snapshot:
		return filepath.Join(c.base, "snapshots", filename), nil
	case backend.Tree:
		return filepath.Join(c.base, "trees", filename), nil
	}

	return "", fmt.Errorf("cache not supported for type %v", t)
}
