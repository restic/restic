package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fd0/khepri"
)

func main() {
	if len(os.Args) == 1 {
		fmt.Printf("usage: %s [file] [file] [...]\n", os.Args[0])
		os.Exit(1)
	}

	for _, path := range os.Args[1:] {
		fmt.Printf("lstat %s\n", path)

		fi, err := os.Lstat(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v", err)
			continue
		}

		node, err := khepri.NodeFromFileInfo(path, fi)
		if err != nil {
			fmt.Printf("err: %v\n", err)
		}

		buf, err := json.MarshalIndent(node, "", "  ")
		if err != nil {
			panic(err)
		}
		fmt.Printf("%s\n", string(buf))
	}
}
