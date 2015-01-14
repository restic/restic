package restic

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/juju/arrar"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/chunker"
)

const (
	maxConcurrentFiles = 8
	maxConcurrentBlobs = 8
)

type Archiver struct {
	s Server
	m *Map

	fileToken chan struct{}
	blobToken chan struct{}

	Error  func(dir string, fi os.FileInfo, err error) error
	Filter func(item string, fi os.FileInfo) bool

	p *Progress
}

func NewArchiver(s Server, p *Progress) (*Archiver, error) {
	var err error
	arch := &Archiver{
		s:         s,
		p:         p,
		fileToken: make(chan struct{}, maxConcurrentFiles),
		blobToken: make(chan struct{}, maxConcurrentBlobs),
	}

	// fill file and blob token
	for i := 0; i < maxConcurrentFiles; i++ {
		arch.fileToken <- struct{}{}
	}

	for i := 0; i < maxConcurrentBlobs; i++ {
		arch.blobToken <- struct{}{}
	}

	// create new map to store all blobs in
	arch.m = NewMap()

	// abort on all errors
	arch.Error = func(string, os.FileInfo, error) error { return err }
	// allow all files
	arch.Filter = func(string, os.FileInfo) bool { return true }

	return arch, nil
}

func (arch *Archiver) Save(t backend.Type, data []byte) (Blob, error) {
	// compute plaintext hash
	id := backend.Hash(data)

	debug("Archiver.Save", "Save(%v, %v)\n", t, id.Str())

	// test if this blob is already known
	blob, err := arch.m.FindID(id)
	if err == nil {
		debug("Archiver.Save", "Save(%v, %v): reusing %v\n", t, id.Str(), blob.Storage.Str())
		id.Free()
		return blob, nil
	}

	// else encrypt and save data
	blob, err = arch.s.Save(t, data, id)

	// store blob in storage map
	smapblob := arch.m.Insert(blob)

	// if the map has a different storage id for this plaintext blob, use that
	// one and remove the other. This happens if the same plaintext blob was
	// stored concurrently and finished earlier than this blob.
	if blob.Storage.Compare(smapblob.Storage) != 0 {
		debug("Archiver.Save", "using other block, removing %v\n", blob.Storage.Str())

		// remove the blob again
		// TODO: implement a list of blobs in transport, so this doesn't happen so often
		err = arch.s.Remove(t, blob.Storage)
		if err != nil {
			return Blob{}, err
		}
	}

	debug("Archiver.Save", "Save(%v, %v): new blob %v\n", t, id.Str(), blob)

	return smapblob, nil
}

func (arch *Archiver) SaveTreeJSON(item interface{}) (Blob, error) {
	// convert to json
	data, err := json.Marshal(item)
	if err != nil {
		return Blob{}, err
	}

	// check if tree has been saved before
	buf := backend.Compress(data)
	id := backend.Hash(buf)
	blob, err := arch.m.FindID(id)

	// return the blob if we found it
	if err == nil {
		return blob, nil
	}

	// otherwise save the data
	blob, err = arch.s.Save(backend.Tree, buf, id)
	if err != nil {
		return Blob{}, err
	}

	// store blob in storage map
	arch.m.Insert(blob)

	return blob, nil
}

// SaveFile stores the content of the file on the backend as a Blob by calling
// Save for each chunk.
func (arch *Archiver) SaveFile(node *Node) (Blobs, error) {
	file, err := os.Open(node.path)
	defer file.Close()
	if err != nil {
		return nil, err
	}

	// check file again
	fi, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if fi.ModTime() != node.ModTime {
		e2 := arch.Error(node.path, fi, errors.New("file was updated, using new version"))

		if e2 == nil {
			// create new node
			n, err := NodeFromFileInfo(node.path, fi)
			if err != nil {
				return nil, err
			}

			// copy node
			*node = *n
		}
	}

	var blobs Blobs

	// if the file is small enough, store it directly
	if node.Size < chunker.MinSize {
		// acquire token
		token := <-arch.blobToken
		defer func() {
			arch.blobToken <- token
		}()

		buf := GetChunkBuf("blob single file")
		defer FreeChunkBuf("blob single file", buf)
		n, err := io.ReadFull(file, buf)
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			return nil, arrar.Annotate(err, "SaveFile() read small file")
		}

		if err == io.EOF {
			// use empty blob list for empty files
			blobs = Blobs{}
		} else {
			blob, err := arch.Save(backend.Data, buf[:n])
			if err != nil {
				return nil, arrar.Annotate(err, "SaveFile() save chunk")
			}

			arch.p.Report(Stat{Bytes: blob.Size})

			blobs = Blobs{blob}
		}
	} else {
		// else store all chunks
		chnker := chunker.New(file)
		chans := [](<-chan Blob){}
		defer chnker.Free()

		chunks := 0

		for {
			buf := GetChunkBuf("blob chunker")
			chunk, err := chnker.Next(buf)
			if err == io.EOF {
				FreeChunkBuf("blob chunker", buf)
				break
			}

			if err != nil {
				FreeChunkBuf("blob chunker", buf)
				return nil, arrar.Annotate(err, "SaveFile() chunker.Next()")
			}

			chunks++

			// acquire token, start goroutine to save chunk
			token := <-arch.blobToken
			resCh := make(chan Blob, 1)

			go func(ch chan<- Blob) {
				blob, err := arch.Save(backend.Data, chunk.Data)
				// TODO handle error
				if err != nil {
					panic(err)
				}

				FreeChunkBuf("blob chunker", buf)

				arch.p.Report(Stat{Bytes: blob.Size})
				arch.blobToken <- token
				ch <- blob
			}(resCh)

			chans = append(chans, resCh)
		}

		blobs = []Blob{}
		for _, ch := range chans {
			blobs = append(blobs, <-ch)
		}

		if len(blobs) != chunks {
			return nil, fmt.Errorf("chunker returned %v chunks, but only %v blobs saved", chunks, len(blobs))
		}
	}

	var bytes uint64

	node.Content = make([]backend.ID, len(blobs))
	for i, blob := range blobs {
		node.Content[i] = blob.ID
		bytes += blob.Size
	}

	if bytes != node.Size {
		return nil, fmt.Errorf("errors saving node %q: saved %d bytes, wanted %d bytes", node.path, bytes, node.Size)
	}

	debug("Archiver.SaveFile", "SaveFile(%q): %v\n", node.path, blobs)

	return blobs, nil
}

func (arch *Archiver) saveTree(t *Tree) (Blob, error) {
	debug("Archiver.saveTree", "saveTree(%v)\n", t)
	var wg sync.WaitGroup

	// add all blobs to global map
	arch.m.Merge(t.Map)

	// TODO: do all this in parallel
	for _, node := range t.Nodes {
		if node.tree != nil {
			b, err := arch.saveTree(node.tree)
			if err != nil {
				return Blob{}, err
			}
			node.Subtree = b.ID
			t.Map.Insert(b)
			arch.p.Report(Stat{Dirs: 1})
		} else if node.Type == "file" {
			if len(node.Content) > 0 {
				removeContent := false

				// check content
				for _, id := range node.Content {
					blob, err := t.Map.FindID(id)
					if err != nil {
						debug("Archiver.saveTree", "unable to find storage id for data blob %v", id.Str())
						arch.Error(node.path, nil, fmt.Errorf("unable to find storage id for data blob %v", id.Str()))
						removeContent = true
						t.Map.DeleteID(id)
						arch.m.DeleteID(id)
						continue
					}

					if ok, err := arch.s.Test(backend.Data, blob.Storage); !ok || err != nil {
						debug("Archiver.saveTree", "blob %v not in repository (error is %v)", blob, err)
						arch.Error(node.path, nil, fmt.Errorf("blob %v not in repository (error is %v)", blob.Storage.Str(), err))
						removeContent = true
						t.Map.DeleteID(id)
						arch.m.DeleteID(id)
					}
				}

				if removeContent {
					debug("Archiver.saveTree", "removing content for %s", node.path)
					node.Content = node.Content[:0]
				}
			}

			if len(node.Content) == 0 {
				// get token
				token := <-arch.fileToken

				// start goroutine
				wg.Add(1)
				go func(n *Node) {
					defer wg.Done()
					defer func() {
						arch.fileToken <- token
					}()

					var blobs Blobs
					blobs, n.err = arch.SaveFile(n)
					for _, b := range blobs {
						t.Map.Insert(b)
					}

					arch.p.Report(Stat{Files: 1})
				}(node)
			}
		}
	}

	wg.Wait()

	usedIDs := backend.NewIDSet()

	// check for invalid file nodes
	for _, node := range t.Nodes {
		if node.Type == "file" && node.Content == nil && node.err == nil {
			return Blob{}, fmt.Errorf("node %v has empty content", node.Name)
		}

		// remember used hashes
		if node.Type == "file" && node.Content != nil {
			for _, id := range node.Content {
				usedIDs.Insert(id)
			}
		}

		if node.Type == "dir" && node.Subtree != nil {
			usedIDs.Insert(node.Subtree)
		}

		if node.err != nil {
			err := arch.Error(node.path, nil, node.err)
			if err != nil {
				return Blob{}, err
			}

			// save error message in node
			node.Error = node.err.Error()
		}
	}

	before := len(t.Map.IDs())
	t.Map.Prune(usedIDs)
	after := len(t.Map.IDs())

	if before != after {
		debug("Archiver.saveTree", "pruned %d ids from map for tree %v\n", before-after, t)
	}

	blob, err := arch.SaveTreeJSON(t)
	if err != nil {
		return Blob{}, err
	}

	return blob, nil
}

func (arch *Archiver) Snapshot(dir string, t *Tree, parentSnapshot backend.ID) (*Snapshot, backend.ID, error) {
	debug_break("Archiver.Snapshot")

	arch.p.Start()
	defer arch.p.Done()

	sn, err := NewSnapshot(dir)
	if err != nil {
		return nil, nil, err
	}

	sn.Parent = parentSnapshot

	blob, err := arch.saveTree(t)
	if err != nil {
		return nil, nil, err
	}
	sn.Tree = blob

	// save snapshot
	blob, err = arch.s.SaveJSON(backend.Snapshot, sn)
	if err != nil {
		return nil, nil, err
	}

	return sn, blob.Storage, nil
}
