package restic

import (
	"encoding/json"
	"io"
	"restic/backend"
	"restic/debug"
	"restic/pack"
	"restic/repository"
	"time"

	"github.com/restic/chunker"
)

// saveTreeJSON stores a tree in the repository.
func saveTreeJSON(repo *repository.Repository, item interface{}) (backend.ID, error) {
	data, err := json.Marshal(item)
	if err != nil {
		return backend.ID{}, err
	}
	data = append(data, '\n')

	// check if tree has been saved before
	id := backend.Hash(data)
	if repo.Index().Has(id, pack.Tree) {
		return id, nil
	}

	return repo.SaveJSON(pack.Tree, item)
}

// ArchiveReader reads from the reader and archives the data. Returned is the
// resulting snapshot and its ID.
func ArchiveReader(repo *repository.Repository, p *Progress, rd io.Reader, name string) (*Snapshot, backend.ID, error) {
	debug.Log("ArchiveReader", "start archiving %s", name)
	sn, err := NewSnapshot([]string{name})
	if err != nil {
		return nil, backend.ID{}, err
	}

	p.Start()
	defer p.Done()

	chnker := chunker.New(rd, repo.Config.ChunkerPolynomial)

	var ids backend.IDs
	var fileSize uint64

	for {
		chunk, err := chnker.Next(getBuf())
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, backend.ID{}, err
		}

		id := backend.Hash(chunk.Data)

		if !repo.Index().Has(id, pack.Data) {
			_, err := repo.SaveAndEncrypt(pack.Data, chunk.Data, nil)
			if err != nil {
				return nil, backend.ID{}, err
			}
			debug.Log("ArchiveReader", "saved blob %v (%d bytes)\n", id.Str(), chunk.Length)
		} else {
			debug.Log("ArchiveReader", "blob %v already saved in the repo\n", id.Str())
		}

		freeBuf(chunk.Data)

		ids = append(ids, id)

		p.Report(Stat{Bytes: uint64(chunk.Length)})
		fileSize += uint64(chunk.Length)
	}

	tree := &Tree{
		Nodes: []*Node{
			&Node{
				Name:       name,
				AccessTime: time.Now(),
				ModTime:    time.Now(),
				Type:       "file",
				Mode:       0644,
				Size:       fileSize,
				UID:        sn.UID,
				GID:        sn.GID,
				User:       sn.Username,
				Content:    ids,
			},
		},
	}

	treeID, err := saveTreeJSON(repo, tree)
	if err != nil {
		return nil, backend.ID{}, err
	}
	sn.Tree = &treeID
	debug.Log("ArchiveReader", "tree saved as %v", treeID.Str())

	id, err := repo.SaveJSONUnpacked(backend.Snapshot, sn)
	if err != nil {
		return nil, backend.ID{}, err
	}

	sn.id = &id
	debug.Log("ArchiveReader", "snapshot saved as %v", id.Str())

	err = repo.Flush()
	if err != nil {
		return nil, backend.ID{}, err
	}

	err = repo.SaveIndex()
	if err != nil {
		return nil, backend.ID{}, err
	}

	return sn, id, nil
}
