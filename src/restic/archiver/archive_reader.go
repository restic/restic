package archiver

import (
	"encoding/json"
	"io"
	"restic"
	"restic/debug"
	"time"

	"restic/errors"

	"github.com/restic/chunker"
)

// saveTreeJSON stores a tree in the repository.
func saveTreeJSON(repo restic.Repository, item interface{}) (restic.ID, error) {
	data, err := json.Marshal(item)
	if err != nil {
		return restic.ID{}, errors.Wrap(err, "")
	}
	data = append(data, '\n')

	// check if tree has been saved before
	id := restic.Hash(data)
	if repo.Index().Has(id, restic.TreeBlob) {
		return id, nil
	}

	return repo.SaveJSON(restic.TreeBlob, item)
}

// ArchiveReader reads from the reader and archives the data. Returned is the
// resulting snapshot and its ID.
func ArchiveReader(repo restic.Repository, p *restic.Progress, rd io.Reader, name string) (*restic.Snapshot, restic.ID, error) {
	debug.Log("ArchiveReader", "start archiving %s", name)
	sn, err := restic.NewSnapshot([]string{name})
	if err != nil {
		return nil, restic.ID{}, err
	}

	p.Start()
	defer p.Done()

	chnker := chunker.New(rd, repo.Config().ChunkerPolynomial)

	var ids restic.IDs
	var fileSize uint64

	for {
		chunk, err := chnker.Next(getBuf())
		if errors.Cause(err) == io.EOF {
			break
		}

		if err != nil {
			return nil, restic.ID{}, errors.Wrap(err, "chunker.Next()")
		}

		id := restic.Hash(chunk.Data)

		if !repo.Index().Has(id, restic.DataBlob) {
			_, err := repo.SaveBlob(restic.DataBlob, chunk.Data, id)
			if err != nil {
				return nil, restic.ID{}, err
			}
			debug.Log("ArchiveReader", "saved blob %v (%d bytes)\n", id.Str(), chunk.Length)
		} else {
			debug.Log("ArchiveReader", "blob %v already saved in the repo\n", id.Str())
		}

		freeBuf(chunk.Data)

		ids = append(ids, id)

		p.Report(restic.Stat{Bytes: uint64(chunk.Length)})
		fileSize += uint64(chunk.Length)
	}

	tree := &restic.Tree{
		Nodes: []*restic.Node{
			&restic.Node{
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
		return nil, restic.ID{}, err
	}
	sn.Tree = &treeID
	debug.Log("ArchiveReader", "tree saved as %v", treeID.Str())

	id, err := repo.SaveJSONUnpacked(restic.SnapshotFile, sn)
	if err != nil {
		return nil, restic.ID{}, err
	}

	debug.Log("ArchiveReader", "snapshot saved as %v", id.Str())

	err = repo.Flush()
	if err != nil {
		return nil, restic.ID{}, err
	}

	err = repo.SaveIndex()
	if err != nil {
		return nil, restic.ID{}, err
	}

	return sn, id, nil
}
