package archiver

import (
	"io"
	"restic"
	"restic/debug"
	"time"

	"restic/errors"

	"github.com/restic/chunker"
)

// ArchiveReader reads from the reader and archives the data. Returned is the
// resulting snapshot and its ID.
func ArchiveReader(repo restic.Repository, p *restic.Progress, rd io.Reader, name string, tags []string) (*restic.Snapshot, restic.ID, error) {
	debug.Log("start archiving %s", name)
	sn, err := restic.NewSnapshot([]string{name}, tags)
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
			debug.Log("saved blob %v (%d bytes)\n", id.Str(), chunk.Length)
		} else {
			debug.Log("blob %v already saved in the repo\n", id.Str())
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

	treeID, err := repo.SaveTree(tree)
	if err != nil {
		return nil, restic.ID{}, err
	}
	sn.Tree = &treeID
	debug.Log("tree saved as %v", treeID.Str())

	id, err := repo.SaveJSONUnpacked(restic.SnapshotFile, sn)
	if err != nil {
		return nil, restic.ID{}, err
	}

	debug.Log("snapshot saved as %v", id.Str())

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
