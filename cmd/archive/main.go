package main

import (
	"fmt"
	"os"

	"github.com/fd0/khepri"
	"github.com/fd0/khepri/backend"
)

const pass = "foobar"

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "usage: archive REPO DIR\n")
		os.Exit(1)
	}
	repo := os.Args[1]
	dir := os.Args[2]

	// fmt.Printf("import %s into backend %s\n", dir, repo)

	var (
		be  backend.Server
		key *khepri.Key
	)

	be, err := backend.OpenLocal(repo)
	if err != nil {
		fmt.Printf("creating %s\n", repo)
		be, err = backend.CreateLocal(repo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed: %v\n", err)
			os.Exit(2)
		}

		key, err = khepri.CreateKey(be, pass)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed: %v\n", err)
			os.Exit(2)
		}
	}

	key, err = khepri.SearchKey(be, pass)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed: %v\n", err)
		os.Exit(2)
	}

	arch, err := khepri.NewArchiver(be, key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "err: %v\n", err)
	}
	arch.Error = func(dir string, fi os.FileInfo, err error) error {
		fmt.Fprintf(os.Stderr, "error for %s: %v\n%s\n", dir, err, fi)
		return err
	}

	arch.Filter = func(item string, fi os.FileInfo) bool {
		// if fi.IsDir() {
		// 	if fi.Name() == ".svn" {
		// 		return false
		// 	}
		// } else {
		// 	if filepath.Ext(fi.Name()) == ".bz2" {
		// 		return false
		// 	}
		// }

		fmt.Printf("%s\n", item)
		return true
	}

	_, blob, err := arch.Import(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Import() error: %v\n", err)
		os.Exit(2)
	}

	fmt.Printf("saved as %+v\n", blob)
}
