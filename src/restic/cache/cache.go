package cache

import (
	"os"
	"path/filepath"
	"restic"
	"restic/crypto"
)

// Cache manages a local cache.
type Cache struct {
	Path string
	Key  *crypto.Key
}

const dirMode = 0700
const fileMode = 0600

// New returns a new cache for the repo id at dir. If dir is the empty string,
// the default cache location (according to the XDG standard) is used.
func New(id string, dir string, repo restic.Repository, key *crypto.Key) (c *Cache, err error) {
	if dir == "" {
		dir, err = getXDGCacheDir()
		if err != nil {
			return nil, err
		}
	}
	subdirs := []string{"snapshots"}
	for _, p := range subdirs {
		if err = os.MkdirAll(filepath.Join(dir, id, p), dirMode); err != nil {
			return nil, err
		}
	}

	c = &Cache{
		Path: filepath.Join(dir, id),
		Key:  key,
	}

	return c, nil
}
