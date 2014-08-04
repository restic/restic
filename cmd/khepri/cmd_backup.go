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

	"github.com/fd0/khepri"
)

func hash(filename string) (khepri.ID, error) {
	h := sha256.New()
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	io.Copy(h, f)
	return h.Sum([]byte{}), nil
}

func archive_dir(repo *khepri.Repository, path string) (khepri.ID, error) {
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

	t := khepri.NewTree()
	for _, e := range entries {
		node := khepri.NodeFromFileInfo(e)

		var id khepri.ID
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
	id, err := repo.PutRaw(khepri.TYPE_BLOB, buf.Bytes())

	if err != nil {
		log.Printf("error saving tree to repo: %v", err)
	}

	log.Printf("tree for %q saved at %s", path, id)

	return id, nil
}

func commandBackup(repo *khepri.Repository, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: backup dir")
	}

	target := args[0]

	id, err := archive_dir(repo, target)
	if err != nil {
		return err
	}

	sn := repo.NewSnapshot(target)
	sn.Tree = id
	sn.Save()

	fmt.Printf("%q archived as %v\n", target, sn.ID())

	return nil
}
