package restorer

import (
	"bytes"
	"context"
	"io"
	"math"
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

	largeFileBlobCount = 25
)

// information about regular file being restored
type fileInfo struct {
	lock     sync.Mutex
	flags    int
	size     int64
	location string      // file on local filesystem relative to restorer basedir
	blobs    interface{} // blobs of the file
}

type fileBlobInfo struct {
	id     restic.ID // the blob id
	offset int64     // blob offset in the file
}

// information about a data pack required to restore one or more files
type packInfo struct {
	id    restic.ID              // the pack id
	files map[*fileInfo]struct{} // set of files that use blobs from this pack
}

// fileRestorer restores set of files
type fileRestorer struct {
	key        *crypto.Key
	idx        func(restic.BlobHandle) []restic.PackedBlob
	packLoader func(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error

	filesWriter *filesWriter

	dst   string
	files []*fileInfo
}

func newFileRestorer(dst string,
	packLoader func(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error,
	key *crypto.Key,
	idx func(restic.BlobHandle) []restic.PackedBlob) *fileRestorer {

	return &fileRestorer{
		key:         key,
		idx:         idx,
		packLoader:  packLoader,
		filesWriter: newFilesWriter(workerCount),
		dst:         dst,
	}
}

func (r *fileRestorer) addFile(location string, content restic.IDs, size int64) {
	r.files = append(r.files, &fileInfo{location: location, blobs: content, size: size})
}

func (r *fileRestorer) targetPath(location string) string {
	return filepath.Join(r.dst, location)
}

func (r *fileRestorer) forEachBlob(blobIDs []restic.ID, fn func(packID restic.ID, packBlob restic.Blob)) error {
	if len(blobIDs) == 0 {
		return nil
	}

	for _, blobID := range blobIDs {
		packs := r.idx(restic.BlobHandle{ID: blobID, Type: restic.DataBlob})
		if len(packs) == 0 {
			return errors.Errorf("Unknown blob %s", blobID.String())
		}
		fn(packs[0].PackID, packs[0].Blob)
	}

	return nil
}

func (r *fileRestorer) restoreFiles(ctx context.Context) error {

	packs := make(map[restic.ID]*packInfo) // all packs
	// Process packs in order of first access. While this cannot guarantee
	// that file chunks are restored sequentially, it offers a good enough
	// approximation to shorten restore times by up to 19% in some test.
	var packOrder restic.IDs

	// create packInfo from fileInfo
	for _, file := range r.files {
		fileBlobs := file.blobs.(restic.IDs)
		largeFile := len(fileBlobs) > largeFileBlobCount
		var packsMap map[restic.ID][]fileBlobInfo
		if largeFile {
			packsMap = make(map[restic.ID][]fileBlobInfo)
		}
		fileOffset := int64(0)
		err := r.forEachBlob(fileBlobs, func(packID restic.ID, blob restic.Blob) {
			if largeFile {
				packsMap[packID] = append(packsMap[packID], fileBlobInfo{id: blob.ID, offset: fileOffset})
				fileOffset += int64(blob.Length) - crypto.Extension
			}
			pack, ok := packs[packID]
			if !ok {
				pack = &packInfo{
					id:    packID,
					files: make(map[*fileInfo]struct{}),
				}
				packs[packID] = pack
				packOrder = append(packOrder, packID)
			}
			pack.files[file] = struct{}{}
		})
		if err != nil {
			// repository index is messed up, can't do anything
			return err
		}
		if largeFile {
			file.blobs = packsMap
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
				r.downloadPack(ctx, pack)
			}
		}
	}
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go worker()
	}

	// the main restore loop
	for _, id := range packOrder {
		pack := packs[id]
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

func (r *fileRestorer) downloadPack(ctx context.Context, pack *packInfo) {

	// calculate pack byte range and blob->[]files->[]offsets mappings
	start, end := int64(math.MaxInt64), int64(0)
	blobs := make(map[restic.ID]struct {
		offset int64                 // offset of the blob in the pack
		length int                   // length of the blob
		files  map[*fileInfo][]int64 // file -> offsets (plural!) of the blob in the file
	})
	for file := range pack.files {
		addBlob := func(blob restic.Blob, fileOffset int64) {
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
		if fileBlobs, ok := file.blobs.(restic.IDs); ok {
			fileOffset := int64(0)
			r.forEachBlob(fileBlobs, func(packID restic.ID, blob restic.Blob) {
				if packID.Equal(pack.id) {
					addBlob(blob, fileOffset)
				}
				fileOffset += int64(blob.Length) - crypto.Extension
			})
		} else if packsMap, ok := file.blobs.(map[restic.ID][]fileBlobInfo); ok {
			for _, blob := range packsMap[pack.id] {
				idxPacks := r.idx(restic.BlobHandle{ID: blob.id, Type: restic.DataBlob})
				for _, idxPack := range idxPacks {
					if idxPack.PackID.Equal(pack.id) {
						addBlob(idxPack.Blob, blob.offset)
						break
					}
				}
			}
		}
	}

	packData := make([]byte, int(end-start))

	h := restic.Handle{Type: restic.PackFile, Name: pack.id.String()}
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
				writeToFile := func() error {
					// this looks overly complicated and needs explanation
					// two competing requirements:
					// - must create the file once and only once
					// - should allow concurrent writes to the file
					// so write the first blob while holding file lock
					// write other blobs after releasing the lock
					file.lock.Lock()
					create := file.flags&fileProgress == 0
					createSize := int64(-1)
					if create {
						defer file.lock.Unlock()
						file.flags |= fileProgress
						createSize = file.size
					} else {
						file.lock.Unlock()
					}
					return r.filesWriter.writeToFile(r.targetPath(file.location), blobData, offset, createSize)
				}
				err := writeToFile()
				if err != nil {
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
