package repository

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"math/big"
	"os"
	"sync"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository/hashing"
	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fileio"
	"github.com/restic/restic/internal/repository/crypto"
	"github.com/restic/restic/internal/repository/pack"
)

// packer holds a pack.packer together with a hash writer.
type packer struct {
	*pack.Packer
	tmpfile *os.File
	bufWr   *bufio.Writer
}

// packerManager keeps a list of open packs and creates new on demand.
//
// Open packers are bucketed by a caller-supplied group id. Blobs written with
// the same group id end up in the same pack files, blobs of different groups are
// kept apart. Regular callers (backup, copy) always use group 0, so there is a
// single bucket and the behaviour is identical to a flat list of packers.
// `prune --group-by` uses this to cluster repacked tree blobs of the same
// snapshot group.
type packerManager struct {
	tpe     restic.BlobType
	key     *crypto.Key
	queueFn func(ctx context.Context, t restic.BlobType, p *packer) error

	pm          sync.Mutex
	packers     map[uint32][]*packer
	packerCount int
	packSize    uint

	// maxOpenGroups bounds the number of groups with open packers. Each open
	// packer holds an open temporary file, so this limits file-descriptor and
	// temp-disk usage, not memory (packers stream to a tmpfile). Once the limit
	// is reached, blobs of further new groups fall back to the shared bucket
	// (group 0), i.e. they lose locality but are still packed normally. Groups
	// are never flushed early, so this never produces undersized packs. Zero
	// means unlimited.
	maxOpenGroups int
}

const defaultPackerCount = 2

// newPackerManager returns a new packer manager which writes temporary files
// to a temporary directory. The number of groups kept open simultaneously is
// derived from the process file-descriptor limit, see MaxOpenTreeGroups.
func newPackerManager(key *crypto.Key, tpe restic.BlobType, packSize uint, packerCount int, queueFn func(ctx context.Context, t restic.BlobType, p *packer) error) *packerManager {
	return &packerManager{
		tpe:           tpe,
		key:           key,
		queueFn:       queueFn,
		packers:       make(map[uint32][]*packer),
		packerCount:   packerCount,
		packSize:      packSize,
		maxOpenGroups: MaxOpenTreeGroups(),
	}
}

func (r *packerManager) Flush(ctx context.Context) error {
	r.pm.Lock()
	defer r.pm.Unlock()

	for group := range r.packers {
		if err := r.flushGroup(ctx, group); err != nil {
			return err
		}
	}
	return nil
}

// flushGroup merges and queues all open packers of a single group. The caller
// must hold r.pm.
func (r *packerManager) flushGroup(ctx context.Context, group uint32) error {
	pendingPackers, err := r.mergePackers(group)
	if err != nil {
		return err
	}

	for _, packer := range pendingPackers {
		debug.Log("manually flushing pending pack")
		err := r.queueFn(ctx, r.tpe, packer)
		if err != nil {
			return err
		}
	}
	return nil
}

// mergePackers merges small pack files of the given group before those are
// uploaded by Flush(). The main purpose of this method is to reduce information
// leaks if a small file is backed up and the blobs end up in separate pack
// files. If the file only consists of two blobs this would leak the size of the
// individual blobs. Only packers of the same group are merged so that grouping
// is preserved. The caller must hold r.pm.
func (r *packerManager) mergePackers(group uint32) ([]*packer, error) {
	packers := r.packers[group]
	// the group's packers are consumed here, drop the now-empty bucket
	delete(r.packers, group)

	pendingPackers := []*packer{}
	var p *packer
	for _, packer := range packers {
		if packer == nil {
			continue
		}

		if p == nil {
			p = packer
		} else if p.Size()+packer.Size() < r.packSize {
			// merge if the result stays below the target pack size
			err := packer.bufWr.Flush()
			if err != nil {
				return nil, err
			}
			_, err = packer.tmpfile.Seek(0, io.SeekStart)
			if err != nil {
				return nil, err
			}

			err = p.Merge(packer.Packer, packer.tmpfile)
			if err != nil {
				return nil, err
			}
		} else {
			pendingPackers = append(pendingPackers, p)
			p = packer
		}
	}
	if p != nil {
		pendingPackers = append(pendingPackers, p)
	}
	return pendingPackers, nil
}

func (r *packerManager) SaveBlob(ctx context.Context, t restic.BlobType, id restic.ID, ciphertext []byte, uncompressedLength int, group uint32) (int, error) {
	r.pm.Lock()
	defer r.pm.Unlock()

	// pickPacker may remap the blob to the shared bucket (group 0) when too many
	// groups are already open, so use the returned effective group afterwards.
	packer, effGroup, err := r.pickPacker(len(ciphertext), group)
	if err != nil {
		return 0, err
	}

	// save ciphertext
	// Add only appends bytes in memory to avoid being a scaling bottleneck
	size, err := packer.Add(t, id, ciphertext, uncompressedLength)
	if err != nil {
		return 0, err
	}

	// if the pack and header is not full enough, put back to the list
	if packer.Size() < r.packSize && !packer.HeaderFull() {
		debug.Log("pack is not full enough (%d bytes)", packer.Size())
		return size, nil
	}

	// forget full packer
	r.forgetPacker(packer, effGroup)

	// call while holding lock to prevent findPacker from creating new packers if the uploaders are busy
	// else write the pack to the backend
	err = r.queueFn(ctx, t, packer)
	if err != nil {
		return 0, err
	}

	return size + packer.HeaderOverhead(), nil
}

func randomInt(max int) (int, error) {
	rangeSize := big.NewInt(int64(max))
	randomInt, err := rand.Int(rand.Reader, rangeSize)
	if err != nil {
		return 0, err
	}
	return int(randomInt.Int64()), nil
}

// pickPacker returns or creates a randomly selected packer for the given group
// into which the blob should be stored, together with the effective group it was
// placed in (which may differ from the requested one, see below). If the
// ciphertext is larger than the packSize, a new standalone packer is returned.
// The caller must hold r.pm.
func (r *packerManager) pickPacker(ciphertextLen int, group uint32) (*packer, uint32, error) {
	// use separate packer if compressed length is larger than the packsize
	// this speeds up the garbage collection of oversized blobs and reduces the cache size
	// as the oversize blobs are only downloaded if necessary
	if ciphertextLen >= int(r.packSize) {
		p, err := r.newPacker()
		return p, group, err
	}

	// Once too many groups are open, further new groups fall back to the shared
	// bucket (group 0). This bounds open temporary files and, crucially, avoids
	// flushing partially filled packers early (which would create many tiny
	// packs). Overflow groups simply lose locality, they are packed as before.
	// The shared bucket itself is always allowed and does not count against the
	// budget, so exactly maxOpenGroups groups can be localized.
	//
	// In the prune path this fallback is normally never hit: getUsedBlobs
	// already caps the number of distinct groups to MaxOpenTreeGroups() (the
	// same value maxOpenGroups is derived from) before assigning them. This
	// second cap is a safety net that also bounds any other/future caller that
	// hands out group ids without pre-capping.
	if group != 0 {
		if _, ok := r.packers[group]; !ok && r.maxOpenGroups > 0 && r.openGroupCount() >= r.maxOpenGroups {
			group = 0
		}
	}

	slots, ok := r.packers[group]
	if !ok {
		// The shared bucket keeps packerCount packers so the anti-chunking-attack
		// mitigation (random distribution) applies to regular backups. Grouped
		// buckets use a single packer: it packs tighter (fewer partial packs) and
		// halves the open-file count, and the mitigation is not relevant for
		// already-chunked blobs that are merely being repacked.
		slotCount := 1
		if group == 0 {
			slotCount = r.packerCount
		}
		slots = make([]*packer, slotCount)
		r.packers[group] = slots
	}

	// randomly distribute blobs onto multiple packer instances. This makes it harder for
	// an attacker to learn at which points a file was chunked and therefore mitigates the attack described in
	// https://www.daemonology.net/blog/chunking-attacks.pdf .
	// See https://github.com/restic/restic/issues/5291#issuecomment-2746146193 for details on the mitigation.
	idx, err := randomInt(len(slots))
	if err != nil {
		return nil, group, err
	}

	// retrieve packer or get a new one
	packer := slots[idx]
	if packer == nil {
		packer, err = r.newPacker()
		if err != nil {
			return nil, group, err
		}
		slots[idx] = packer
	}
	return packer, group, nil
}

// openGroupCount returns the number of localized groups that currently have an
// open packer, excluding the shared bucket (group 0). The caller must hold r.pm.
func (r *packerManager) openGroupCount() int {
	n := len(r.packers)
	if _, ok := r.packers[0]; ok {
		n--
	}
	return n
}

// forgetPacker drops the given packer from the given group. This is used to forget full packers.
func (r *packerManager) forgetPacker(packer *packer, group uint32) {
	slots := r.packers[group]
	allNil := true
	for i, p := range slots {
		if packer == p {
			slots[i] = nil
		}
		if slots[i] != nil {
			allNil = false
		}
	}
	// drop the bucket once empty so it no longer counts against maxOpenGroups
	if allNil {
		delete(r.packers, group)
	}
}

// findPacker returns a packer for a new blob of size bytes. Either a new one is
// created or one is returned that already has some blobs.
func (r *packerManager) newPacker() (pck *packer, err error) {
	debug.Log("create new pack")
	tmpfile, err := fileio.TempFile("", "restic-temp-pack-")
	if err != nil {
		return nil, errors.WithStack(err)
	}

	bufWr := bufio.NewWriter(tmpfile)
	p := pack.NewPacker(r.key, bufWr)
	pck = &packer{
		Packer:  p,
		tmpfile: tmpfile,
		bufWr:   bufWr,
	}

	return pck, nil
}

// savePacker stores p in the backend.
func (r *Repository) savePacker(ctx context.Context, t restic.BlobType, p *packer) error {
	debug.Log("save packer for %v with %d blobs (%d bytes)\n", t, p.Packer.Count(), p.Packer.Size())
	err := p.Packer.Finalize()
	if err != nil {
		return err
	}
	err = p.bufWr.Flush()
	if err != nil {
		return err
	}

	// calculate sha256 hash in a second pass
	var rd io.Reader
	rd, err = backend.NewFileReader(p.tmpfile, nil)
	if err != nil {
		return err
	}
	beHasher := r.be.Hasher()
	var beHr *hashing.Reader
	if beHasher != nil {
		beHr = hashing.NewReader(rd, beHasher)
		rd = beHr
	}

	hr := hashing.NewReader(rd, sha256.New())
	_, err = io.Copy(io.Discard, hr)
	if err != nil {
		return err
	}

	id := restic.IDFromHash(hr.Sum(nil))
	h := backend.Handle{Type: backend.PackFile, Name: id.String(), IsMetadata: t.IsMetadata()}
	var beHash []byte
	if beHr != nil {
		beHash = beHr.Sum(nil)
	}
	rrd, err := backend.NewFileReader(p.tmpfile, beHash)
	if err != nil {
		return err
	}

	err = r.be.Save(ctx, h, rrd)
	if err != nil {
		debug.Log("Save(%v) error: %v", h, err)
		return err
	}

	debug.Log("saved as %v", h)

	err = p.tmpfile.Close()
	if err != nil {
		return errors.Wrap(err, "close tempfile")
	}

	// update blobs in the index
	debug.Log("  updating blobs %v to pack %v", p.Packer.Blobs(), id)
	return r.idx.StorePack(ctx, id, p.Packer.Blobs(), &internalRepository{r})
}
