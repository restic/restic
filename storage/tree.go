package storage

import (
	"encoding/json"
	"io"
	"os"
	"syscall"
	"time"
)

type Tree struct {
	Nodes []Node `json:"nodes"`
}

type Node struct {
	Name    string      `json:"name"`
	Mode    os.FileMode `json:"mode"`
	ModTime time.Time   `json:"mtime"`
	User    uint32      `json:"user"`
	Group   uint32      `json:"group"`
	Content ID          `json:"content"`
}

func NewTree() *Tree {
	return &Tree{
		Nodes: []Node{},
	}
}

func (t *Tree) Restore(r io.Reader) error {
	dec := json.NewDecoder(r)
	return dec.Decode(t)
}

func (t *Tree) Save(w io.Writer) error {
	enc := json.NewEncoder(w)
	return enc.Encode(t)
}

func NodeFromFileInfo(fi os.FileInfo) Node {
	node := Node{
		Name:    fi.Name(),
		Mode:    fi.Mode(),
		ModTime: fi.ModTime(),
	}

	if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
		node.User = stat.Uid
		node.Group = stat.Gid
	}

	return node
}
