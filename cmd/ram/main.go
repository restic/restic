package main

import (
	"os"

	"github.com/restic/restic"
)

func main() {
	max := int(1e6)
	nodes := make([]*restic.Node, 0, max)

	fi, err := os.Lstat("main.go")
	if err != nil {
		panic(err)
	}

	for i := 0; i < max; i++ {
		node, err := restic.NodeFromFileInfo("main.go", fi)
		if err != nil {
			panic(err)
		}

		nodes = append(nodes, node)
	}
}
