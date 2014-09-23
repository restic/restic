package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

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

func json_pp(data []byte) error {
	var buf bytes.Buffer
	err := json.Indent(&buf, data, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(buf.Bytes()))
	return nil
}

type StopWatch struct {
	start, last time.Time
}

func NewStopWatch() *StopWatch {
	return &StopWatch{
		start: time.Now(),
		last:  time.Now(),
	}
}

func (s *StopWatch) Next(format string, data ...interface{}) {
	t := time.Now()
	d := t.Sub(s.last)
	s.last = t
	arg := make([]interface{}, len(data)+1)
	arg[0] = d
	copy(arg[1:], data)
	fmt.Printf("[%s]: "+format+"\n", arg...)
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "usage: cat REPO ID\n")
		os.Exit(1)
	}
	repo := os.Args[1]
	id, err := backend.ParseID(filepath.Base(os.Args[2]))
	if err != nil {
		panic(err)
	}

	s := NewStopWatch()

	be, err := backend.OpenLocal(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed: %v\n", err)
		os.Exit(1)
	}

	s.Next("OpenLocal()")

	key, err := khepri.SearchKey(be, read_password("Enter Password for Repository: "))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed: %v\n", err)
		os.Exit(2)
	}

	s.Next("SearchKey()")

	// try all possible types
	for _, t := range []backend.Type{backend.Blob, backend.Snapshot, backend.Lock, backend.Tree, backend.Key} {
		buf, err := be.Get(t, id)
		if err != nil {
			continue
		}

		s.Next("Get(%s, %s)", t, id)

		if t == backend.Key {
			json_pp(buf)
		}

		buf2, err := key.Decrypt(buf)
		if err != nil {
			panic(err)
		}

		if t == backend.Blob {
			// directly output blob
			fmt.Println(string(buf2))
		} else {
			// try to uncompress and print as idented json
			err = json_pp(backend.Uncompress(buf2))
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed: %v\n", err)
			}
		}

		break
	}
}
