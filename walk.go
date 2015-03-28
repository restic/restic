package restic

import (
	"path/filepath"

	"github.com/restic/restic/debug"
)

type WalkTreeJob struct {
	Path  string
	Error error

	Node *Node
	Tree *Tree
}

func walkTree(s Server, path string, treeBlob Blob, done chan struct{}, jobCh chan<- WalkTreeJob) {
	debug.Log("walkTree", "start on %q (%v)", path, treeBlob)
	// load tree
	t, err := LoadTree(s, treeBlob)
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
			walkTree(s, p, blob, done, jobCh)
		} else {
			// load old blobs
			node.blobs, err = t.Map.Select(node.Content)
			if err != nil {
				debug.Log("walkTree", "unable to load bobs for %q (%v): %v", path, treeBlob, err)
			}
			jobCh <- WalkTreeJob{Path: p, Node: node, Error: err}
		}
	}

	jobCh <- WalkTreeJob{Path: filepath.Join(path), Tree: t}
	debug.Log("walkTree", "done for %q (%v)", path, treeBlob)
}

// WalkTree walks the tree specified by ID recursively and sends a job for each
// file and directory it finds. When the channel done is closed, processing
// stops.
func WalkTree(server Server, blob Blob, done chan struct{}, jobCh chan<- WalkTreeJob) {
	debug.Log("WalkTree", "start on %v", blob)
	walkTree(server, "", blob, done, jobCh)
	close(jobCh)
	debug.Log("WalkTree", "done")
}
