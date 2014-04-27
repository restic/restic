package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/fd0/khepri/storage"
)

func hash(filename string) (storage.ID, error) {
	h := sha256.New()
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	io.Copy(h, f)
	return h.Sum([]byte{}), nil
}

func archive_dir(repo *storage.DirRepository, path string) (storage.ID, error) {
	log.Printf("archiving dir %q", path)
	dir, err := os.Open(path)
	if err != nil {
		log.Printf("open(%q): %v\n", path, err)
		return nil, err
	}

	entries, err := dir.Readdir(-1)
	if err != nil {
		log.Printf("readdir(%q): %v\n", path, err)
		return nil, err
	}

	// use nil ID for empty directories
	if len(entries) == 0 {
		return nil, nil
	}

	t := storage.NewTree()
	for _, e := range entries {
		node := storage.NodeFromFileInfo(e)

		var id storage.ID
		var err error

		if e.IsDir() {
			id, err = archive_dir(repo, filepath.Join(path, e.Name()))
		} else {
			id, err = repo.PutFile(filepath.Join(path, e.Name()))
		}

		node.Content = id

		t.Nodes = append(t.Nodes, node)

		if err != nil {
			log.Printf("  error storing %q: %v\n", e.Name(), err)
			continue
		}
	}

	log.Printf("  dir %q: %v entries", path, len(t.Nodes))

	var buf bytes.Buffer
	t.Save(&buf)
	id, err := repo.PutRaw(buf.Bytes())

	if err != nil {
		log.Printf("error saving tree to repo: %v", err)
	}

	log.Printf("tree for %q saved at %s", path, id)

	return id, nil
}

func commandBackup(repo *storage.DirRepository, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: backup dir")
	}

	target := args[0]

	id, err := archive_dir(repo, target)
	if err != nil {
		return err
	}

	fmt.Printf("%q archived as %v\n", target, id)

	return nil
}
