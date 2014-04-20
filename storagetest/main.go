package main

import (
	"crypto/sha256"
	"io"
	"log"
	"os"

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

func archive_dir(repo storage.Repository, path string) {
	log.Printf("archiving dir %q", path)
	// items := make()
	// filepath.Walk(path, func(item string, info os.FileInfo, err error) error {
	// 	log.Printf("  archiving %q", item)

	// 	if item != path && info.IsDir() {
	// 		archive_dir(repo, item)
	// 	} else {

	// 	}

	// 	return nil
	// })
}

func main() {
	repo, err := storage.NewDir("repo")
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	switch os.Args[1] {
	case "add":
		for _, file := range os.Args[2:] {
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
		file := os.Args[2]
		name := os.Args[3]

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
		for _, dir := range os.Args[2:] {
			archive_dir(repo, dir)
		}
	default:
		log.Fatalf("unknown command: %q", os.Args[1])
	}
}
