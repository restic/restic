package restic

import (
	"path/filepath"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
)

type WalkTreeJob struct {
	Path  string
	Error error

	Node *Node
	Tree *Tree
}

func walkTree(s Server, path string, id backend.ID, done chan struct{}, jobCh chan<- WalkTreeJob) {
	debug.Log("walkTree", "start on %q (%v)", path, id.Str())
	// load tree
	t, err := LoadTree(s, id)
	if err != nil {
		jobCh <- WalkTreeJob{Path: path, Error: err}
		return
	}

	for _, node := range t.Nodes {
		p := filepath.Join(path, node.Name)
		if node.Type == "dir" {
			blob, err := t.Map.FindID(node.Subtree)
			if err != nil {
				jobCh <- WalkTreeJob{Path: p, Error: err}
				continue
			}
			walkTree(s, p, blob.Storage, done, jobCh)
		} else {
			jobCh <- WalkTreeJob{Path: p, Node: node}
		}
	}

	if path != "" {
		jobCh <- WalkTreeJob{Path: filepath.Join(path), Tree: t}
	}
	debug.Log("walkTree", "done for %q (%v)", path, id.Str())
}

// WalkTree walks the tree specified by ID recursively and sends a job for each
// file and directory it finds. When the channel done is closed, processing
// stops.
func WalkTree(server Server, id backend.ID, done chan struct{}, jobCh chan<- WalkTreeJob) {
	debug.Log("WalkTree", "start on %v", id.Str())
	walkTree(server, "", id, done, jobCh)
	close(jobCh)
	debug.Log("WalkTree", "done", id.Str())
}
