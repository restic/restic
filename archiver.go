package restic

import (
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
	s  Server
	ch *ContentHandler

	bl *BlobList // blobs used for the current snapshot

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

	// abort on all errors
	arch.Error = func(string, os.FileInfo, error) error { return err }
	// allow all files
	arch.Filter = func(string, os.FileInfo) bool { return true }

	arch.bl = NewBlobList()
	arch.ch = NewContentHandler(s)

	// load all blobs from all snapshots
	// TODO: only use bloblist from old snapshot if available
	err = arch.ch.LoadAllMaps()
	if err != nil {
		return nil, err
	}

	return arch, nil
}

func (arch *Archiver) Save(t backend.Type, data []byte) (Blob, error) {
	blob, err := arch.ch.Save(t, data)
	if err != nil {
		return Blob{}, err
	}

	// store blob in storage map for current snapshot
	arch.bl.Insert(blob)

	return blob, nil
}

func (arch *Archiver) SaveJSON(t backend.Type, item interface{}) (Blob, error) {
	blob, err := arch.ch.SaveJSON(t, item)
	if err != nil {
		return Blob{}, err
	}

	// store blob in storage map for current snapshot
	arch.bl.Insert(blob)

	return blob, nil
}

// SaveFile stores the content of the file on the backend as a Blob by calling
// Save for each chunk.
func (arch *Archiver) SaveFile(node *Node) error {
	file, err := os.Open(node.path)
	defer file.Close()
	if err != nil {
		return err
	}

	// check file again
	fi, err := file.Stat()
	if err != nil {
		return err
	}

	if fi.ModTime() != node.ModTime {
		e2 := arch.Error(node.path, fi, errors.New("file changed as we read it\n"))

		if e2 == nil {
			// create new node
			n, err := NodeFromFileInfo(node.path, fi)
			if err != nil {
				return err
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
			return arrar.Annotate(err, "SaveFile() read small file")
		}

		if err == io.EOF {
			// use empty blob list for empty files
			blobs = Blobs{}
		} else {
			blob, err := arch.ch.Save(backend.Data, buf[:n])
			if err != nil {
				return arrar.Annotate(err, "SaveFile() save chunk")
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
				return arrar.Annotate(err, "SaveFile() chunker.Next()")
			}

			chunks++

			// acquire token, start goroutine to save chunk
			token := <-arch.blobToken
			resCh := make(chan Blob, 1)

			go func(ch chan<- Blob) {
				blob, err := arch.ch.Save(backend.Data, chunk.Data)
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
			return fmt.Errorf("chunker returned %v chunks, but only %v blobs saved", chunks, len(blobs))
		}
	}

	var bytes uint64

	node.Content = make([]backend.ID, len(blobs))
	for i, blob := range blobs {
		node.Content[i] = blob.ID
		arch.bl.Insert(blob)
		bytes += blob.Size
	}

	if bytes != node.Size {
		return fmt.Errorf("errors saving node %q: saved %d bytes, wanted %d bytes", node.path, bytes, node.Size)
	}

	return nil
}

func (arch *Archiver) saveTree(t *Tree) (Blob, error) {
	var wg sync.WaitGroup

	for _, node := range *t {
		if node.tree != nil && node.Subtree == nil {
			b, err := arch.saveTree(node.tree)
			if err != nil {
				return Blob{}, err
			}
			node.Subtree = b.ID
			arch.p.Report(Stat{Dirs: 1})
		} else if node.Type == "file" && len(node.Content) == 0 {
			// get token
			token := <-arch.fileToken

			// start goroutine
			wg.Add(1)
			go func(n *Node) {
				defer wg.Done()
				defer func() {
					arch.fileToken <- token
				}()

				node.err = arch.SaveFile(n)
				arch.p.Report(Stat{Files: 1})
			}(node)
		}
	}

	wg.Wait()

	// check for invalid file nodes
	for _, node := range *t {
		if node.Type == "file" && node.Content == nil && node.err == nil {
			return Blob{}, fmt.Errorf("node %v has empty content", node.Name)
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

	blob, err := arch.SaveJSON(backend.Tree, t)
	if err != nil {
		return Blob{}, err
	}

	return blob, nil
}

func (arch *Archiver) Snapshot(dir string, t *Tree, parentSnapshot backend.ID) (*Snapshot, backend.ID, error) {
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
	sn.Tree = blob.ID

	// save bloblist
	blob, err = arch.SaveJSON(backend.Map, arch.bl)
	if err != nil {
		return nil, nil, err
	}
	sn.Map = blob.Storage

	// save snapshot
	blob, err = arch.SaveJSON(backend.Snapshot, sn)
	if err != nil {
		return nil, nil, err
	}

	return sn, blob.Storage, nil
}
