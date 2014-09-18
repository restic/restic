package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"

	"github.com/fd0/khepri/chunker"
)

func main() {
	count, bytes := 0, 0
	min := 0
	max := 0

	var (
		err  error
		file *os.File = os.Stdin
	)

	if len(os.Args) > 1 {
		file, err = os.Open(os.Args[1])
		if err != nil {
			panic(err)
		}
	}

	ch := chunker.New(file)

	for {
		chunk, err := ch.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			panic(err)
		}

		fmt.Printf("%d %016x %02x\n", chunk.Length, chunk.Cut, sha256.Sum256(chunk.Data))
		count++
		bytes += chunk.Length

		if chunk.Length == chunker.MaxSize {
			max++
		} else if chunk.Length == chunker.MinSize {
			min++
		}
	}

	var avg int
	if count > 0 {
		avg = bytes / count
	}

	fmt.Fprintf(os.Stderr, "%d chunks from %d bytes, average size %d (%d min size, %d max size chunks)\n",
		count, bytes, avg, min, max)
}
