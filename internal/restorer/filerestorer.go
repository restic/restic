package restorer

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"sync"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

// TODO if a blob is corrupt, there may be good blob copies in other packs
// TODO evaluate if it makes sense to split download and processing workers
//      pro: can (slowly) read network and decrypt/write files concurrently
//      con: each worker needs to keep one pack in memory

const (
	workerCount = 8

	// fileInfo flags
	fileProgress = 1
	fileError    = 2
)

// information about regular file being restored
type fileInfo struct {
	lock      sync.Mutex
	flags     int
	remaining int64       // remaining download size (includes encryption framing)
	location  string      // file on local filesystem relative to restorer basedir
	blobs     []restic.ID // remaining blobs of the file
}

// information about a data pack required to restore one or more files
type packInfo struct {
	id    restic.ID              // the pack id
	files map[*fileInfo]struct{} // set of files that use blobs from this pack
}

// fileRestorer restores set of files
type fileRestorer struct {
	key        *crypto.Key
	idx        func(restic.ID, restic.BlobType) ([]restic.PackedBlob, bool)
	packLoader func(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error

	filesWriter *filesWriter

	dst   string
	files []*fileInfo
}

func newFileRestorer(dst string,
	packLoader func(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error,
	key *crypto.Key,
	idx func(restic.ID, restic.BlobType) ([]restic.PackedBlob, bool)) *fileRestorer {

	return &fileRestorer{
		key:         key,
		idx:         idx,
		packLoader:  packLoader,
		filesWriter: newFilesWriter(workerCount),
		dst:         dst,
	}
}

func (r *fileRestorer) addFile(location string, content restic.IDs) {
	r.files = append(r.files, &fileInfo{location: location, blobs: content})
}

func (r *fileRestorer) targetPath(location string) string {
	return filepath.Join(r.dst, location)
}

func (r *fileRestorer) forEachBlob(blobIDs []restic.ID, fn func(packID restic.ID, packBlob restic.Blob)) error {
	if len(blobIDs) == 0 {
		return nil
	}

	for _, blobID := range blobIDs {
		packs, found := r.idx(blobID, restic.DataBlob)
		if !found {
			return errors.Errorf("Unknown blob %s", blobID.String())
		}
		fn(packs[0].PackID, packs[0].Blob)
	}

	return nil
}

func (r *fileRestorer) restoreFiles(ctx context.Context,
	reportDoneFileBlob func(path string, size uint),
	reportDoneFile func(path string),
	reportError func(path string, err error)) error {

	packs := make(map[restic.ID]*packInfo) // all packs

	// create packInfo from fileInfo
	for _, file := range r.files {
		err := r.forEachBlob(file.blobs, func(packID restic.ID, blob restic.Blob) {
			file.remaining += int64(blob.Length)
			pack, ok := packs[packID]
			if !ok {
				pack = &packInfo{
					id:    packID,
					files: make(map[*fileInfo]struct{}),
				}
				packs[packID] = pack
			}
			pack.files[file] = struct{}{}
		})
		if err != nil {
			// repository index is messed up, can't do anything
			return err
		}
	}

	var wg sync.WaitGroup
	downloadCh := make(chan *packInfo)
	worker := func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return // context cancelled
			case pack, ok := <-downloadCh:
				if !ok {
					return // channel closed
				}
				r.downloadPack(ctx, pack, reportDoneFileBlob, reportDoneFile, reportError)
			}
		}
	}
	for i := 0; i < workerCount; i++ {
		go worker()
		wg.Add(1)
	}

	// the main restore loop
	for _, pack := range packs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case downloadCh <- pack:
			debug.Log("Scheduled download pack %s", pack.id.Str())
		}
	}

	close(downloadCh)
	wg.Wait()

	return nil
}

func (r *fileRestorer) downloadPack(ctx context.Context, pack *packInfo,
	reportDoneFileBlob func(path string, size uint),
	reportDoneFile func(path string),
	reportError func(path string, err error)) {
	const MaxInt64 = 1<<63 - 1 // odd Go does not have this predefined somewhere

	// calculate pack byte range and blob->[]files->[]offsets mappings
	start, end := int64(MaxInt64), int64(0)
	blobs := make(map[restic.ID]struct {
		offset int64                 // offset of the blob in the pack
		length int                   // length of the blob
		files  map[*fileInfo][]int64 // file -> offsets (plural!) of the blob in the file
	})
	for file := range pack.files {
		fileOffset := int64(0)
		r.forEachBlob(file.blobs, func(packID restic.ID, blob restic.Blob) {
			if packID.Equal(pack.id) {
				if start > int64(blob.Offset) {
					start = int64(blob.Offset)
				}
				if end < int64(blob.Offset+blob.Length) {
					end = int64(blob.Offset + blob.Length)
				}
				blobInfo, ok := blobs[blob.ID]
				if !ok {
					blobInfo.offset = int64(blob.Offset)
					blobInfo.length = int(blob.Length)
					blobInfo.files = make(map[*fileInfo][]int64)
					blobs[blob.ID] = blobInfo
				}
				blobInfo.files[file] = append(blobInfo.files[file], fileOffset)
			}
			fileOffset += int64(blob.Length) - crypto.Extension
		})
	}

	packData := make([]byte, int(end-start))

	h := restic.Handle{Type: restic.DataFile, Name: pack.id.String()}
	err := r.packLoader(ctx, h, int(end-start), start, func(rd io.Reader) error {
		l, err := io.ReadFull(rd, packData)
		if err != nil {
			return err
		}
		if l != len(packData) {
			return errors.Errorf("unexpected pack size: expected %d but got %d", len(packData), l)
		}
		return nil
	})

	markFileError := func(file *fileInfo, err error) {
		file.lock.Lock()
		defer file.lock.Unlock()
		if file.flags&fileError == 0 {
			file.flags |= fileError
			reportError(file.location, err)
		}
	}

	if err != nil {
		for file := range pack.files {
			markFileError(file, err)
		}
		return
	}

	rd := bytes.NewReader(packData)

	for blobID, blob := range blobs {
		blobData, err := r.loadBlob(rd, blobID, blob.offset-start, blob.length)
		if err != nil {
			for file := range blob.files {
				markFileError(file, err)
			}
			continue
		}
		for file, offsets := range blob.files {
			for _, offset := range offsets {
				err = r.filesWriter.writeToFile(r.targetPath(file.location), blobData, offset, file.flags&fileProgress == 0)
				file.flags |= fileProgress
				if err == nil {
					reportDoneFileBlob(file.location, uint(len(blobData))) // number of bytes written to disk
					file.remaining -= int64(blob.length)                   // blob size with encryption framing
					if file.remaining <= 0 {
						reportDoneFile(file.location)
					}
				} else {
					markFileError(file, err)
					break
				}
			}
		}
	}
}

func (r *fileRestorer) loadBlob(rd io.ReaderAt, blobID restic.ID, offset int64, length int) ([]byte, error) {
	// TODO reconcile with Repository#loadBlob implementation

	buf := make([]byte, length)

	n, err := rd.ReadAt(buf, offset)
	if err != nil {
		return nil, err
	}

	if n != length {
		return nil, errors.Errorf("error loading blob %v: wrong length returned, want %d, got %d", blobID.Str(), length, n)
	}

	// decrypt
	nonce, ciphertext := buf[:r.key.NonceSize()], buf[r.key.NonceSize():]
	plaintext, err := r.key.Open(ciphertext[:0], nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.Errorf("decrypting blob %v failed: %v", blobID, err)
	}

	// check hash
	if !restic.Hash(plaintext).Equal(blobID) {
		return nil, errors.Errorf("blob %v returned invalid hash", blobID)
	}

	return plaintext, nil
}
