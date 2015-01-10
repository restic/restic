package restic

import (
	"os"
	"path/filepath"

	"github.com/juju/arrar"
)

type FilterFunc func(item string, fi os.FileInfo) bool
type ErrorFunc func(dir string, fi os.FileInfo, err error) error

type Scanner struct {
	Error  ErrorFunc
	Filter FilterFunc

	p *Progress
}

func NewScanner(p *Progress) *Scanner {
	sc := &Scanner{p: p}

	// abort on all errors
	sc.Error = func(s string, fi os.FileInfo, err error) error { return err }
	// allow all files
	sc.Filter = func(string, os.FileInfo) bool { return true }

	return sc
}

func scan(filterFn FilterFunc, progress *Progress, dir string) (*Tree, error) {
	var err error

	// open and list path
	fd, err := os.Open(dir)
	defer fd.Close()

	if err != nil {
		return nil, err
	}

	entries, err := fd.Readdir(-1)
	if err != nil {
		return nil, err
	}

	// build new tree
	tree := NewTree()
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())

		if !filterFn(path, entry) {
			continue
		}

		node, err := NodeFromFileInfo(path, entry)
		if err != nil {
			// TODO: error processing
			return nil, err
		}

		err = tree.Insert(node)
		if err != nil {
			return nil, err
		}

		if entry.IsDir() {
			// save all errors in node.err, sort out later
			node.tree, node.err = scan(filterFn, progress, path)
		}
	}

	for _, node := range tree.Nodes {
		if node.Type == "file" && node.Content != nil {
			continue
		}

		switch node.Type {
		case "file":
			progress.Report(Stat{Files: 1, Bytes: node.Size})
		case "dir":
			progress.Report(Stat{Dirs: 1})
		}
	}

	return tree, nil
}

func (sc *Scanner) Scan(path string) (*Tree, error) {
	sc.p.Start()
	defer sc.p.Done()

	fi, err := os.Lstat(path)
	if err != nil {
		return nil, arrar.Annotatef(err, "Lstat(%q)", path)
	}

	node, err := NodeFromFileInfo(path, fi)
	if err != nil {
		return nil, arrar.Annotate(err, "NodeFromFileInfo()")
	}

	tree := NewTree()
	tree.Insert(node)
	if node.Type != "dir" {
		sc.p.Report(Stat{Files: 1, Bytes: node.Size})

		return tree, nil
	}

	sc.p.Report(Stat{Dirs: 1})

	node.tree, err = scan(sc.Filter, sc.p, path)
	if err != nil {
		return nil, arrar.Annotate(err, "loadTree()")
	}

	return tree, nil
}
