package restic

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/restic/restic/backend"
)

type Cache struct {
	base string
}

func NewCache() (*Cache, error) {
	dir, err := GetCacheDir()
	if err != nil {
		return nil, err
	}

	return &Cache{base: dir}, nil
}

func (c *Cache) Has(t backend.Type, id backend.ID) (bool, error) {
	// try to open file
	filename, err := c.filename(t, id)
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

func (c *Cache) Store(t backend.Type, id backend.ID, rd io.Reader) error {
	filename, err := c.filename(t, id)
	if err != nil {
		return err
	}

	dirname := filepath.Dir(filename)
	err = os.MkdirAll(dirname, 0700)
	if err != nil {
		return err
	}

	file, err := os.Create(filename)
	defer file.Close()
	if err != nil {
		return err
	}

	_, err = io.Copy(file, rd)
	return err
}

func (c *Cache) Load(t backend.Type, id backend.ID) (io.ReadCloser, error) {
	// try to open file
	filename, err := c.filename(t, id)
	if err != nil {
		return nil, err
	}

	return os.Open(filename)
}

// Construct file name for given Type.
func (c *Cache) filename(t backend.Type, id backend.ID) (string, error) {
	cachedir, err := GetCacheDir()
	if err != nil {
		return "", err
	}

	switch t {
	case backend.Snapshot:
		return filepath.Join(cachedir, "snapshots", id.String()), nil
	case backend.Tree:
		return filepath.Join(cachedir, "trees", id.String()), nil
	}

	return "", fmt.Errorf("cache not supported for type %v", t)
}
