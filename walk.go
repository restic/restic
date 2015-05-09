package restic

import (
	"path/filepath"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/repo"
)

type WalkTreeJob struct {
	Path  string
	Error error

	Node *Node
	Tree *Tree
}

func walkTree(s *repo.Server, path string, treeID backend.ID, done chan struct{}, jobCh chan<- WalkTreeJob) {
	debug.Log("walkTree", "start on %q (%v)", path, treeID.Str())

	t, err := LoadTree(s, treeID)
	if err != nil {
		jobCh <- WalkTreeJob{Path: path, Error: err}
		return
	}

	for _, node := range t.Nodes {
		p := filepath.Join(path, node.Name)
		if node.Type == "dir" {
			walkTree(s, p, node.Subtree, done, jobCh)
		} else {
			jobCh <- WalkTreeJob{Path: p, Node: node}
		}
	}

	jobCh <- WalkTreeJob{Path: path, Tree: t}
	debug.Log("walkTree", "done for %q (%v)", path, treeID.Str())
}

// WalkTree walks the tree specified by id recursively and sends a job for each
// file and directory it finds. When the channel done is closed, processing
// stops.
func WalkTree(server *repo.Server, id backend.ID, done chan struct{}, jobCh chan<- WalkTreeJob) {
	debug.Log("WalkTree", "start on %v", id.Str())
	walkTree(server, "", id, done, jobCh)
	close(jobCh)
	debug.Log("WalkTree", "done")
}
