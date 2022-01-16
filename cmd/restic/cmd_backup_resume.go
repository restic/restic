package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

type finishedDir struct {
	ID   restic.ID `json:"id"`
	Path string    `json:"path"`
}

type resume struct {
	*os.File
}

// removeOldResumeFiles removes all resume files in resumedir
// that are older than maxAge
func removeOldResumeFiles(resumedir string, maxAge time.Duration) {
	f, err := fs.Open(resumedir)
	if err != nil {
		if !os.IsNotExist(errors.Cause(err)) {
			Warnf("could not read resume dir %v: %v\n", resumedir, err)
		}
		return
	}
	defer func() {
		_ = f.Close()
	}()

	entries, err := f.Readdir(-1)
	if err != nil {
		Warnf("could not read resume dir %v: %v\n", resumedir, err)
		return
	}

	for _, entry := range entries {
		if entry.ModTime().Before(time.Now().Add(-maxAge)) {
			filename := filepath.Join(resumedir, entry.Name())
			Verboseff("removing old resume file %s\n", filename)
			err := os.Remove(filename)
			if err != nil {
				Warnf("could not remove old resume file %v: %v\n", filename, err)
			}
		}
	}
}

const maxResumeAge = 30 * 24 * time.Hour

// getResume tries to load resume data for the given targets string.
// Moreover, it opens a new resume file to write resume data into.
// This function does not return an error as all error cases only
// lead to warnings that are printed out.
func getResume(targets []string, repo *repository.Repository, force bool) (*resume, map[string]restic.ID) {
	cache := repo.Cache
	if force || cache == nil {
		// only use resume functionality if no forced backup and if there is a cache
		return nil, nil
	}
	resumedir := filepath.Join(cache.BaseDir(), repo.Config().ID, "resume")
	_ = fs.MkdirAll(resumedir, 0700)
	removeOldResumeFiles(resumedir, maxResumeAge)

	targetString := fmt.Sprintf("%v", targets)
	id := restic.Hash([]byte(targetString))
	filename := filepath.Join(resumedir, id.String())

	dirs := readResume(filename, repo)

	f, err := os.Create(filename)
	if err != nil {
		Warnf("could not create resume file %v: %v\n", filename, err)
		return nil, dirs
	}

	return &resume{f}, dirs
}

// getResume tries to load resume data from the given filename
func readResume(filename string, repo restic.Repository) map[string]restic.ID {
	dirs := make(map[string]restic.ID)
	f, err := fs.Open(filename)
	if err != nil {
		if !os.IsNotExist(errors.Cause(err)) {
			Warnf("could not open resume file %v: %v\n", filename, err)
		}
		return nil
	}
	defer func() {
		_ = f.Close()
	}()

	found := false
	dec := json.NewDecoder(f)
	for {
		var fd finishedDir
		if err := dec.Decode(&fd); err != nil {
			break
		}
		debug.Log("found entry '%v' in resume file", fd)

		found = true

		// only use trees that are in the index
		if !repo.Index().Has(restic.BlobHandle{ID: fd.ID, Type: restic.TreeBlob}) {
			debug.Log("ignoring id '%v' from resume file - not in index", fd.ID)
			continue
		}

		// remove paths if superpaths is also contained
		for path := range dirs {
			if strings.HasPrefix(path, fd.Path) {
				debug.Log("resume file: '%v' is superseded by '%v'", path, fd.Path)
				delete(dirs, path)
			}
		}

		dirs[fd.Path] = fd.ID
	}

	if found && len(dirs) == 0 {
		Verboseff("found resume data but index doesn't contain it\n")
	}

	return dirs
}

// writeFinishedDir writes the information about a finished tree to the resume file
func (r *resume) writeFinishedDir(item string, current *restic.Node) {
	if r == nil {
		return
	}

	if current != nil && current.Type == "dir" {
		f := finishedDir{ID: *current.Subtree, Path: item}
		_ = json.NewEncoder(r).Encode(f)
	}
}

// Done removes the resume file.
// Should be called after the snapshot has been written to the repo.
func (r *resume) Done() {
	if r == nil {
		return
	}

	filename := r.Name()
	_ = r.Close()
	_ = os.Remove(filename)
}
