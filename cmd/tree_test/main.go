package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/fd0/khepri"
)

func check(err error) {
	if err == nil {
		return
	}

	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func save(repo *khepri.Repository, path string) {
	tree, err := khepri.NewTreeFromPath(repo, path)

	check(err)

	id, err := tree.Save(repo)

	fmt.Printf("saved tree as %s\n", id)
}

func restore(repo *khepri.Repository, idstr string) {
	id, err := khepri.ParseID(idstr)
	check(err)

	tree, err := khepri.NewTreeFromRepo(repo, id)
	check(err)

	walk(0, tree)
}

func walk(indent int, tree *khepri.Tree) {
	for _, node := range tree.Nodes {
		if node.Type == "dir" {
			fmt.Printf("%s%s:%s/\n", strings.Repeat(" ", indent), node.Type, node.Name)
			walk(indent+1, node.Tree)
		} else {
			fmt.Printf("%s%s:%s\n", strings.Repeat(" ", indent), node.Type, node.Name)
		}
	}
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "usage: %s [save|restore] DIR\n", os.Args[0])
		os.Exit(1)
	}

	command := os.Args[1]
	arg := os.Args[2]

	repo, err := khepri.NewRepository("khepri-repo")
	check(err)

	switch command {
	case "save":
		save(repo, arg)
	case "restore":
		restore(repo, arg)
	}
}
