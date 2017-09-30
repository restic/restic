package archiver

import (
	"context"
	"io"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/errors"

	"github.com/restic/chunker"
)

// Reader allows saving a stream of data to the repository.
type Reader struct {
	restic.Repository

	Tags     []string
	Hostname string
}

// Archive reads data from the reader and saves it to the repo.
func (r *Reader) Archive(ctx context.Context, name string, rd io.Reader, p *restic.Progress) (*restic.Snapshot, restic.ID, error) {
	if name == "" {
		return nil, restic.ID{}, errors.New("no filename given")
	}

	debug.Log("start archiving %s", name)
	sn, err := restic.NewSnapshot([]string{name}, r.Tags, r.Hostname, time.Now())
	if err != nil {
		return nil, restic.ID{}, err
	}

	p.Start()
	defer p.Done()

	repo := r.Repository
	chnker := chunker.New(rd, repo.Config().ChunkerPolynomial)

	ids := restic.IDs{}
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
			_, _, err := repo.SaveBlob(ctx, restic.DataBlob, chunk.Data, id)
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
			{
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

	treeID, _, err := repo.SaveTree(ctx, tree)
	if err != nil {
		return nil, restic.ID{}, err
	}
	sn.Tree = &treeID
	debug.Log("tree saved as %v", treeID.Str())

	id, _, err := repo.SaveJSONUnpacked(ctx, restic.SnapshotFile, sn)
	if err != nil {
		return nil, restic.ID{}, err
	}

	debug.Log("snapshot saved as %v", id.Str())

	_, err = repo.Flush()
	if err != nil {
		return nil, restic.ID{}, err
	}

	_, err = repo.SaveIndex(ctx)
	if err != nil {
		return nil, restic.ID{}, err
	}

	return sn, id, nil
}
