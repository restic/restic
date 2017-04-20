package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/cmd/restic/web"
	"github.com/restic/restic/repository"
)

type CmdWeb struct {
	global *GlobalOptions
}

func init() {
	_, err := parser.AddCommand("server",
		"serve repository in a web interface",
		"The web command serves the repository in a web interface",
		&CmdWeb{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdWeb) Usage() string {
	return "PORT"
}

func (cmd CmdWeb) Execute(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("port not specified, usage:%s", cmd.Usage())
	}

	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}
	repo.LoadIndex()
	httpRepo := &httpRepo{
		repo: repo,
		objects: map[string]http.File{
			"": &httpRoot{repo: repo},
		},
	}

	portStr := args[0]
	if port, err := strconv.Atoi(portStr); err != nil || port > 65535 {
		return fmt.Errorf("%s is not a valid port number", port)
	}

	cmd.global.Printf("Now serving on %s\n", portStr)
	http.ListenAndServe(":"+portStr, http.FileServer(httpRepo))
	return nil
}

type httpRepo struct {
	repo *repository.Repository

	// Object path -> object
	objects map[string]http.File
}

func (hr httpRepo) Open(name string) (http.File, error) {
	var object http.File
	var found bool

	parts := strings.Split(name, "/")
	for _, part := range parts {
		object, found = hr.objects[part]
		if !found {
			return nil, fmt.Errorf("%s not found", name)
		}
		children, err := object.(childrener).loadChildren()
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			if _, ok := hr.objects[child.Name()]; !ok {
				hr.objects[child.Name()] = child
			}
		}
	}

	return object, nil
}

type childrener interface {
	loadChildren() ([]*httpObject, error)
}

type httpObject struct {
	name    string
	size    int64
	mode    os.FileMode
	modtime time.Time

	repo *repository.Repository
	tree backend.ID
	file *web.File

	// lazily loaded
	children      []*httpObject
	readdirOffset int
}

func newHttpObject(repo *repository.Repository, node *restic.Node) (*httpObject, error) {
	file, err := web.NewFile(repo, node)
	if err != nil {
		return nil, err
	}
	return &httpObject{
		name:    node.Name,
		size:    int64(node.Size),
		mode:    node.Mode,
		modtime: node.ModTime,

		repo: repo,
		tree: node.Subtree,
		file: file,
	}, nil
}

func newHttpObjectFromSnapshot(repo *repository.Repository, snapshot *restic.Snapshot) *httpObject {
	return &httpObject{
		name:    snapshot.Time.Format(time.RFC3339),
		size:    0,
		mode:    os.ModeDir,
		modtime: snapshot.Time,

		repo: repo,
		tree: snapshot.Tree,
	}
}

func (ho *httpObject) Close() error               { return nil }
func (ho *httpObject) Read(p []byte) (int, error) { return ho.file.Read(p) }
func (ho *httpObject) Seek(offset int64, whence int) (int64, error) {
	return ho.file.Seek(offset, whence)
}
func (ho *httpObject) Stat() (os.FileInfo, error) { return ho, nil }

func (ho *httpObject) loadChildren() ([]*httpObject, error) {
	if len(ho.children) > 0 {
		return ho.children, nil
	}

	if ho.file != nil {
		return nil, nil
	}

	tree, err := restic.LoadTree(ho.repo, ho.tree)
	if err != nil {
		return nil, err
	}
	ho.children = make([]*httpObject, len(tree.Nodes))
	for i, node := range tree.Nodes {
		child, err := newHttpObject(ho.repo, node)
		if err != nil {
			return nil, err
		}
		ho.children[i] = child
	}

	return ho.children, nil
}

func (ho *httpObject) Readdir(n int) ([]os.FileInfo, error) {
	children, err := ho.loadChildren()

	start := ho.readdirOffset
	end := start + n
	if end > len(children) {
		end = len(children)
	}
	ho.readdirOffset = end

	if start == end {
		ho.readdirOffset = 0
	}

	return conv(children[start:end]), err
}

func (ho *httpObject) Name() string       { return ho.name }
func (ho *httpObject) Size() int64        { return ho.size }
func (ho *httpObject) Mode() os.FileMode  { return ho.mode }
func (ho *httpObject) IsDir() bool        { return ho.mode.IsDir() }
func (ho *httpObject) Sys() interface{}   { return nil }
func (ho *httpObject) ModTime() time.Time { return ho.modtime }

type httpRoot struct {
	repo          *repository.Repository
	readdirOffset int

	// Lazily loaded
	snapshots []*httpObject
}

func (hr *httpRoot) Close() error                                 { return nil }
func (hr *httpRoot) Read(p []byte) (int, error)                   { return len(p), nil }
func (hr *httpRoot) Seek(offset int64, whence int) (int64, error) { return offset, nil }
func (hr *httpRoot) Stat() (os.FileInfo, error)                   { return hr, nil }

func (hr *httpRoot) loadChildren() ([]*httpObject, error) {
	if len(hr.snapshots) > 0 {
		return hr.snapshots, nil
	}

	done := make(chan struct{})
	defer close(done)
	for id := range hr.repo.List(backend.Snapshot, done) {
		snapshot, err := restic.LoadSnapshot(hr.repo, id)
		if err != nil {
			return nil, err
		}
		hr.snapshots = append(hr.snapshots, newHttpObjectFromSnapshot(hr.repo, snapshot))
	}

	return hr.snapshots, nil
}

func (hr *httpRoot) Readdir(n int) ([]os.FileInfo, error) {
	snapshots, err := hr.loadChildren()
	if err != nil {
		return nil, err
	}

	start := hr.readdirOffset
	end := start + n
	if end > len(snapshots) {
		end = len(snapshots)
	}
	hr.readdirOffset = end

	if start == end {
		hr.readdirOffset = 0
	}

	return conv(snapshots[start:end]), nil
}

func (hr httpRoot) Name() string      { return "/" }
func (hr httpRoot) Size() int64       { return 0 }
func (hr httpRoot) Mode() os.FileMode { return os.ModeDir }
func (hr httpRoot) IsDir() bool       { return true }
func (hr httpRoot) Sys() interface{}  { return nil }
func (hr httpRoot) ModTime() time.Time {
	var modtime time.Time
	for _, sn := range hr.snapshots {
		if sn.ModTime().After(modtime) {
			modtime = sn.ModTime()
		}
	}
	return modtime
}

func conv(in []*httpObject) (out []os.FileInfo) {
	out = make([]os.FileInfo, len(in))
	for i, iin := range in {
		out[i] = os.FileInfo(iin)
	}
	return out
}
