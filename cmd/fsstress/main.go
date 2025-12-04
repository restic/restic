package main

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"os"
)

func main() {
	r := rand.New(rand.NewSource(123456))

	var filenames []string
	for i := 0; i < 10; i++ {
		filenames = append(filenames, fmt.Sprintf("file-%d.txt", r.Int()))
	}
	for _, filename := range filenames {
		// create file but make sure to not overwrite existing files
		if _, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600); err != nil {
			panic(err)
		}
	}

	// prepare chunks
	var chunks [][]byte
	for i := 0; i < 1000; i++ {
		buf := make([]byte, r.Intn(10000)+10_000)
		if _, err := r.Read(buf); err != nil {
			panic(err)
		}
		chunks = append(chunks, buf)
	}

	type task struct {
		filename string
		offset   int64
		blob     []byte
	}

	// schedule to files
	var tasks []task
	for _, filename := range filenames {
		offset := int64(0)
		for i := 0; i < len(chunks); i++ {
			tasks = append(tasks, task{filename: filename, offset: offset, blob: chunks[i]})
			offset += int64(len(chunks[i]))
		}
	}
	rand.Shuffle(len(tasks), func(i, j int) {
		tasks[i], tasks[j] = tasks[j], tasks[i]
	})

	// actually write to files
	for _, task := range tasks {
		f, err := os.OpenFile(task.filename, os.O_WRONLY, 0600)
		if err != nil {
			panic(err)
		}
		if _, err := f.WriteAt(task.blob, task.offset); err != nil {
			panic(err)
		}
		if err := f.Close(); err != nil {
			panic(err)
		}
	}

	// verify content
	for _, filename := range filenames {
		f, err := os.OpenFile(filename, os.O_RDONLY, 0600)
		if err != nil {
			panic(err)
		}
		offset := int64(0)
		for _, chunk := range chunks {
			buf := make([]byte, len(chunk))
			if _, err := io.ReadFull(f, buf); err != nil {
				panic(err)
			}
			if !bytes.Equal(buf, chunk) {
				panic(fmt.Errorf("content mismatch for %s at offset %d", filename, offset))
			}
			offset += int64(len(chunk))
		}

		if err := f.Close(); err != nil {
			panic(err)
		}

		if err := os.Remove(filename); err != nil {
			panic(err)
		}
	}

	fmt.Println("files verified successfully")
}
