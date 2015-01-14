package main

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
)

const (
	MaxFiles = 23
	MaxDepth = 3
)

var urnd *os.File

func init() {
	f, err := os.Open("/dev/urandom")
	if err != nil {
		panic(err)
	}

	urnd = f
}

func rndRd(bytes int) io.Reader {
	return io.LimitReader(urnd, int64(bytes))
}

func create_dir(target string, depth int) {
	fmt.Printf("create_dir %s, depth %d\n", target, depth)
	err := os.Mkdir(target, 0755)
	if err != nil && !os.IsExist(err) {
		panic(err)
	}

	for i := 0; i < MaxFiles; i++ {
		if depth == 0 {
			filename := filepath.Join(target, fmt.Sprintf("file%d", i))
			fmt.Printf("create file %v\n", filename)
			f, err := os.Create(filename)
			if err != nil {
				panic(err)
			}

			_, err = io.Copy(f, rndRd(rand.Intn(1024)))
			if err != nil {
				panic(err)
			}

			err = f.Close()
			if err != nil {
				panic(err)
			}
		} else {
			create_dir(filepath.Join(target, fmt.Sprintf("dir%d", i)), depth-1)
		}
	}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "USAGE: %s TARGETDIR\n", os.Args[0])
		os.Exit(1)
	}

	create_dir(os.Args[1], MaxDepth)
}
