package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

// References content within a repository.
type ID []byte

func (id ID) String() string {
	return hex.EncodeToString(id)
}

func (id ID) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.String())
}

func (id *ID) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}

	*id = make([]byte, len(s)/2)
	_, err = hex.Decode(*id, []byte(s))
	if err != nil {
		return err
	}

	return nil
}

// ParseID converts the given string to an ID.
func ParseID(s string) ID {
	b, err := hex.DecodeString(s)

	if err != nil {
		panic(err)
	}

	return ID(b)
}

type Repository interface {
	Store([]byte) ID
	Get(ID) []byte
}

type Repo map[string][]byte

func (r Repo) Store(buf []byte) ID {
	hash := sha256.New()
	_, err := hash.Write(buf)
	check(err)

	id := ID(hash.Sum([]byte{}))
	r[id.String()] = buf

	return id
}

func (r Repo) Get(id ID) []byte {
	buf, ok := r[id.String()]
	if !ok {
		panic("no such id")
	}

	return buf
}

func (r Repo) Dump(wr io.Writer) {
	for k, v := range r {
		_, err := wr.Write([]byte(k))
		check(err)
		_, err = wr.Write([]byte(":"))
		check(err)
		_, err = wr.Write(v)
		check(err)
		_, err = wr.Write([]byte("\n"))
		check(err)
	}
}

type Tree struct {
	Nodes []*Node `json:"nodes,omitempty"`
}

type Node struct {
	Name    string `json:"name"`
	Tree    *Tree  `json:"tree,omitempty"`
	Subtree ID     `json:"subtree,omitempty"`
	Content ID     `json:"content,omitempty"`
}

func (tree Tree) Save(repo Repository) ID {
	// fmt.Printf("nodes: %#v\n", tree.Nodes)
	for _, node := range tree.Nodes {
		if node.Tree != nil {
			node.Subtree = node.Tree.Save(repo)
			node.Tree = nil
		}
	}

	buf, err := json.Marshal(tree)
	check(err)

	return repo.Store(buf)
}

func (tree Tree) PP(wr io.Writer) {
	tree.pp(0, wr)
}

func (tree Tree) pp(indent int, wr io.Writer) {
	for _, node := range tree.Nodes {
		if node.Tree != nil {
			fmt.Printf("%s%s/\n", strings.Repeat("    ", indent), node.Name)
			node.Tree.pp(indent+1, wr)
		} else {
			fmt.Printf("%s%s [%s]\n", strings.Repeat("    ", indent), node.Name, node.Content)
		}

	}
}

func create_tree(path string) *Tree {
	dir, err := os.Open(path)
	check(err)

	entries, err := dir.Readdir(-1)
	check(err)

	tree := &Tree{
		Nodes: make([]*Node, 0, len(entries)),
	}

	for _, entry := range entries {
		node := &Node{}
		node.Name = entry.Name()

		if !entry.Mode().IsDir() && entry.Mode()&os.ModeType != 0 {
			fmt.Fprintf(os.Stderr, "skipping %q\n", filepath.Join(path, entry.Name()))
			continue
		}

		tree.Nodes = append(tree.Nodes, node)

		if entry.IsDir() {
			node.Tree = create_tree(filepath.Join(path, entry.Name()))
			continue
		}

		file, err := os.Open(filepath.Join(path, entry.Name()))
		defer file.Close()
		check(err)

		hash := sha256.New()
		io.Copy(hash, file)

		node.Content = hash.Sum([]byte{})
	}

	return tree
}

func load_tree(repo Repository, id ID) *Tree {
	tree := &Tree{}

	buf := repo.Get(id)
	json.Unmarshal(buf, tree)

	for _, node := range tree.Nodes {
		if node.Subtree != nil {
			node.Tree = load_tree(repo, node.Subtree)
			node.Subtree = nil
		}
	}

	return tree
}

func main() {
	repo := make(Repo)

	tree := create_tree(os.Args[1])
	// encoder := json.NewEncoder(os.Stdout)
	// fmt.Println("---------------------------")
	// encoder.Encode(tree)
	// fmt.Println("---------------------------")

	id := tree.Save(repo)

	// for k, v := range repo {
	// 	fmt.Printf("%s: %s\n", k, v)
	// }

	// fmt.Println("---------------------------")

	tree2 := load_tree(repo, id)
	tree2.PP(os.Stdout)
	// encoder.Encode(tree2)

	// dumpfile, err := os.Create("dump")
	// defer dumpfile.Close()
	// check(err)

	// repo.Dump(dumpfile)
}
