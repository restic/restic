package cache

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"restic"
	"restic/crypto"
	"restic/errors"
	"strconv"
)

// Cache manages a local cache.
type Cache struct {
	Path string
	Key  *crypto.Key
}

const dirMode = 0700
const fileMode = 0600

func readVersion(dir string) (v uint, err error) {
	buf, err := ioutil.ReadFile(filepath.Join(dir, "version"))
	if os.IsNotExist(err) {
		return 0, nil
	}

	if err != nil {
		return 0, errors.Wrap(err, "ReadFile")
	}

	ver, err := strconv.ParseUint(string(buf), 10, 32)
	if err != nil {
		return 0, errors.Wrap(err, "ParseUint")
	}

	return uint(ver), nil
}

const cacheVersion = 1

// New returns a new cache for the repo ID at dir. If dir is the empty string,
// the default cache location (according to the XDG standard) is used.
func New(id string, dir string, repo restic.Repository, key *crypto.Key) (c *Cache, err error) {
	if dir == "" {
		dir, err = getXDGCacheDir()
		if err != nil {
			return nil, err
		}
	}

	v, err := readVersion(dir)
	if err != nil {
		return nil, err
	}

	if v > cacheVersion {
		return nil, errors.New("cache version is newer")
	}

	if v < cacheVersion {
		err = ioutil.WriteFile(filepath.Join(dir, "version"), []byte(fmt.Sprintf("%d", cacheVersion)), 0644)
		if err != nil {
			return nil, errors.Wrap(err, "WriteFile")
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
