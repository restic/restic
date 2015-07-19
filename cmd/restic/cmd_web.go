package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/repository"
)

type CmdWeb struct {
	global *GlobalOptions
}

func init() {
	_, err := parser.AddCommand("web",
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
		repo:      repo,
		snapshots: make(map[string]*restic.Snapshot),
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

	// snapshot timestamp -> snapshot
	snapshots map[string]*restic.Snapshot
}

func (hr httpRepo) Open(name string) (http.File, error) {
	parts := strings.Split(name, "/")[1:]
	log.Printf("Serving %s: %#v\n", name, parts)

	done := make(chan struct{})
	defer close(done)
	for id := range hr.repo.List(backend.Snapshot, done) {
		snapshot, err := restic.LoadSnapshot(hr.repo, id)
		if err != nil {
			return nil, err
		}
		hr.snapshots[snapshot.Time.Format(time.RFC3339)] = snapshot
	}

	if parts[0] == "" {
		return &httpRoot{repo: hr.repo}, nil
	}
	snapshot, ok := hr.snapshots[parts[0]]
	if ok && len(parts) == 1 {
		return &httpSnapshot{repo: hr.repo, snapshot: snapshot}, nil
	}
	return nil, fmt.Errorf("Not found")
}

type httpRoot struct {
	repo          *repository.Repository
	readdirOffset int

	// Lazily loaded
	snapshots []os.FileInfo
}

func (hr *httpRoot) Close() error                                 { return nil }
func (hr *httpRoot) Read(p []byte) (int, error)                   { return len(p), nil }
func (hr *httpRoot) Seek(offset int64, whence int) (int64, error) { return offset, nil }
func (hr *httpRoot) Stat() (os.FileInfo, error)                   { return hr, nil }

func (hr *httpRoot) Readdir(n int) ([]os.FileInfo, error) {
	if hr.readdirOffset == 0 || hr.snapshots == nil {
		if hr.snapshots == nil {
			hr.snapshots = make([]os.FileInfo, 0)
		} else {
			hr.snapshots = hr.snapshots[:0]
		}

		done := make(chan struct{})
		defer close(done)
		for id := range hr.repo.List(backend.Snapshot, done) {
			snapshot, err := restic.LoadSnapshot(hr.repo, id)
			if err != nil {
				return nil, err
			}
			hr.snapshots = append(hr.snapshots, httpSnapshot{
				repo:     hr.repo,
				snapshot: snapshot,
			})
		}
	}

	start := hr.readdirOffset
	end := start + n
	if end > len(hr.snapshots) {
		end = len(hr.snapshots)
	}
	hr.readdirOffset = end

	return hr.snapshots[start:end], nil
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

type httpSnapshot struct {
	repo     *repository.Repository
	snapshot *restic.Snapshot

	// Lazily loaded
	httpNodes     []os.FileInfo
	readdirOffset int
}

var _ = http.File(&httpSnapshot{})

func (hs *httpSnapshot) Close() error                                 { return nil }
func (hs *httpSnapshot) Read(p []byte) (int, error)                   { return len(p), nil }
func (hs *httpSnapshot) Seek(offset int64, whence int) (int64, error) { return offset, nil }
func (hs *httpSnapshot) Stat() (os.FileInfo, error)                   { return hs, nil }

func (hs *httpSnapshot) Readdir(n int) ([]os.FileInfo, error) {
	if len(hs.httpNodes) == 0 {
		tree, err := restic.LoadTree(hs.repo, hs.snapshot.Tree)
		if err != nil {
			return nil, err
		}
		hs.httpNodes = make([]os.FileInfo, len(tree.Nodes))
		for i := range tree.Nodes {
			hs.httpNodes[i] = httpNodeInfo{tree.Nodes[i]}
		}
	}

	start := hs.readdirOffset
	end := start + n
	if end > len(hs.httpNodes) {
		end = len(hs.httpNodes)
	}
	hs.readdirOffset = end

	return hs.httpNodes[start:end], nil
}

func (hs httpSnapshot) Name() string       { return hs.snapshot.Time.Format(time.RFC3339) }
func (hs httpSnapshot) Size() int64        { return 0 }
func (hs httpSnapshot) Mode() os.FileMode  { return os.ModeDir }
func (hs httpSnapshot) ModTime() time.Time { return hs.snapshot.Time }
func (hs httpSnapshot) IsDir() bool        { return true }
func (hs httpSnapshot) Sys() interface{}   { return nil }

/*
type treeer interface {
	tree() backend.ID
}
type snapshotTreeer struct {
	*restic.Snapshot
}

func (sn snapshotTreeer) tree() backend.ID { return sn.Snapshot.Tree }

type treeTreeer struct {
	*restic.Node
}

func (sn treeTreeer) tree() backend.ID { return sn.Node.Subtree }
*/

type httpNode struct {
	repo *repository.Repository
	node *restic.Node

	httpNodes     []os.FileInfo
	readdirOffset int
}

var _ = http.File(&httpNode{})

// http.File interface
func (hn *httpNode) Close() error                                 { return nil }
func (hn *httpNode) Read(p []byte) (int, error)                   { return len(p), nil }
func (hn *httpNode) Seek(offset int64, whence int) (int64, error) { return offset, nil }
func (hn *httpNode) Stat() (os.FileInfo, error)                   { return httpNodeInfo{hn.node}, nil }

func (hn *httpNode) Readdir(n int) ([]os.FileInfo, error) {
	if hn.node.Subtree == nil {
		return nil, nil
	}

	if len(hn.httpNodes) == 0 {
		subtree, err := restic.LoadTree(hn.repo, hn.node.Subtree)
		if err != nil {
			return nil, err
		}
		hn.httpNodes = make([]os.FileInfo, len(subtree.Nodes))
		for i := range subtree.Nodes {
			hn.httpNodes[i] = httpNodeInfo{subtree.Nodes[i]}
		}
	}

	start := hn.readdirOffset
	end := start + n
	if end > len(hn.httpNodes) {
		end = len(hn.httpNodes)
	}
	hn.readdirOffset = end

	return hn.httpNodes[start:end], nil
}

type httpNodeInfo struct {
	node *restic.Node
}

func (hn httpNodeInfo) Name() string       { return hn.node.Name }
func (hn httpNodeInfo) Size() int64        { return int64(hn.node.Size) }
func (hn httpNodeInfo) Mode() os.FileMode  { return hn.node.Mode }
func (hn httpNodeInfo) ModTime() time.Time { return hn.node.ModTime }
func (hn httpNodeInfo) IsDir() bool        { return hn.node.Subtree != nil }
func (hn httpNodeInfo) Sys() interface{}   { return nil }
