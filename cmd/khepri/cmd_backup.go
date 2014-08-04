package main

import (
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

func store_file(repo *khepri.Repository, path string) (khepri.ID, error) {
	obj, err := repo.NewObject(khepri.TYPE_BLOB)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	defer func() {
		file.Close()
	}()

	_, err = io.Copy(obj, file)
	if err != nil {
		return nil, err
	}

	err = obj.Close()
	if err != nil {
		return nil, err
	}

	return obj.ID(), nil
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
			id, err = store_file(repo, filepath.Join(path, e.Name()))
		}

		node.Content = id

		t.Nodes = append(t.Nodes, node)

		if err != nil {
			log.Printf("  error storing %q: %v\n", e.Name(), err)
			continue
		}
	}

	log.Printf("  dir %q: %v entries", path, len(t.Nodes))

	obj, err := repo.NewObject(khepri.TYPE_BLOB)

	if err != nil {
		log.Printf("error creating object for tree: %v", err)
		return nil, err
	}

	err = t.Save(obj)
	if err != nil {
		log.Printf("error saving tree to repo: %v", err)
	}

	obj.Close()

	id := obj.ID()
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
