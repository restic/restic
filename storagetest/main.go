package main

import (
	"bytes"
	"crypto/sha256"
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

func archive_dir(repo storage.Repository, path string) (storage.ID, error) {
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

func main() {
	if len(os.Args) <= 2 {
		log.Fatalf("usage: %s repo [add|link|putdir] ...", os.Args[0])
	}

	repo, err := storage.NewDirRepository(os.Args[1])
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	switch os.Args[2] {
	case "add":
		for _, file := range os.Args[3:] {
			f, err := os.Open(file)
			if err != nil {
				log.Printf("error opening file %q: %v", file, err)
				continue
			}
			id, err := repo.Put(f)
			if err != nil {
				log.Printf("Put() error: %v", err)
				continue
			}

			log.Printf("archived file %q as ID %v", file, id)
		}
	case "link":
		file := os.Args[3]
		name := os.Args[4]

		id, err := hash(file)
		if err != nil {
			log.Fatalf("error hashing filq %q: %v", file, err)
		}

		present, err := repo.Test(id)
		if err != nil {
			log.Fatalf("error testing presence of id %v: %v", id, err)
		}

		if !present {
			log.Printf("adding file to repo as ID %v", id)
			_, err = repo.PutFile(file)
			if err != nil {
				log.Fatalf("error adding file %q: %v", file, err)
			}
		}

		err = repo.Link(name, id)
		if err != nil {
			log.Fatalf("error linking name %q to id %v", name, id)
		}
	case "putdir":
		for _, dir := range os.Args[3:] {
			archive_dir(repo, dir)
		}
	default:
		log.Fatalf("unknown command: %q", os.Args[2])
	}
}
