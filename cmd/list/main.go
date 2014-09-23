package main

import (
	"encoding/json"
	"fmt"
	"os"

	"code.google.com/p/go.crypto/ssh/terminal"

	"github.com/fd0/khepri"
	"github.com/fd0/khepri/backend"
)

func read_password(prompt string) string {
	p := os.Getenv("KHEPRI_PASSWORD")
	if p != "" {
		return p
	}

	fmt.Print(prompt)
	pw, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to read password: %v", err)
		os.Exit(2)
	}
	fmt.Println()

	return string(pw)
}

func list(be backend.Server, key *khepri.Key, t backend.Type) {
	ids, err := be.List(t)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed: %v\n", err)
		os.Exit(3)
	}

	for _, id := range ids {
		buf, err := be.Get(t, id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to get snapshot %s: %v\n", id, err)
			continue
		}

		if t != backend.Key && t != backend.Blob {
			buf, err = key.Decrypt(buf)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				continue
			}

		}

		if t == backend.Snapshot {
			var sn khepri.Snapshot
			err = json.Unmarshal(backend.Uncompress(buf), &sn)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				continue
			}

			fmt.Printf("%s %s\n", id, sn.String())
		} else if t == backend.Blob {
			fmt.Printf("%s %d bytes (encrypted)\n", id, len(buf))
		} else if t == backend.Tree {
			fmt.Printf("%s\n", backend.Hash(buf))
		} else if t == backend.Key {
			k := &khepri.Key{}
			err = json.Unmarshal(buf, k)
			if err != nil {
				fmt.Fprintf(os.Stderr, "unable to unmashal key: %v\n", err)
				continue
			}
			fmt.Println(key)
		} else if t == backend.Lock {
			fmt.Printf("lock: %v\n", id)
		}
	}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: archive REPO\n")
		os.Exit(1)
	}
	repo := os.Args[1]

	be, err := backend.OpenLocal(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed: %v\n", err)
		os.Exit(1)
	}

	key, err := khepri.SearchKey(be, read_password("Enter Password for Repository: "))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed: %v\n", err)
		os.Exit(2)
	}

	fmt.Printf("keys:\n")
	list(be, key, backend.Key)
	fmt.Printf("---\nlocks:\n")
	list(be, key, backend.Lock)
	fmt.Printf("---\nsnapshots:\n")
	list(be, key, backend.Snapshot)
	fmt.Printf("---\ntrees:\n")
	list(be, key, backend.Tree)
	fmt.Printf("---\nblobs:\n")
	list(be, key, backend.Blob)
}
