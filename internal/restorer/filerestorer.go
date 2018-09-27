package restorer

import (
	"context"
	"io"
	"path/filepath"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

// TODO if a blob is corrupt, there may be good blob copies in other packs
// TODO evaluate if it makes sense to split download and processing workers
//      pro: can (slowly) read network and decrypt/write files concurrently
//      con: each worker needs to keep one pack in memory
// TODO evaluate memory footprint for larger repositories, say 10M packs/10M files
// TODO consider replacing pack file cache with blob cache
// TODO avoid decrypting the same blob multiple times
// TODO evaluate disabled debug logging overhead for large repositories

const (
	workerCount = 8

	// max number of open output file handles
	filesWriterCount = 32

	// estimated average pack size used to calculate pack cache capacity
	averagePackSize = 5 * 1024 * 1024

	// pack cache capacity should support at least one cached pack per worker
	// allow space for extra 5 packs for actual caching
	packCacheCapacity = (workerCount + 5) * averagePackSize
)

// information about regular file being restored
type fileInfo struct {
	location string      // file on local filesystem relative to restorer basedir
	blobs    []restic.ID // remaining blobs of the file
}

// information about a data pack required to restore one or more files
type packInfo struct {
	// the pack id
	id restic.ID

	// set of files that use blobs from this pack
	files map[*fileInfo]struct{}

	// number of other packs that must be downloaded before all blobs in this pack can be used
	cost int

	// used by packHeap
	index int
}

// fileRestorer restores set of files
type fileRestorer struct {
	key        *crypto.Key
	idx        filePackTraverser
	packLoader func(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error

	packCache   *packCache   // pack cache
	filesWriter *filesWriter // file write

	dst   string
	files []*fileInfo
}

func newFileRestorer(dst string, packLoader func(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error, key *crypto.Key, idx filePackTraverser) *fileRestorer {
	return &fileRestorer{
		packLoader:  packLoader,
		key:         key,
		idx:         idx,
		filesWriter: newFilesWriter(filesWriterCount),
		packCache:   newPackCache(packCacheCapacity),
		dst:         dst,
	}
}

func (r *fileRestorer) addFile(location string, content restic.IDs) {
	r.files = append(r.files, &fileInfo{location: location, blobs: content})
}

func (r *fileRestorer) targetPath(location string) string {
	return filepath.Join(r.dst, location)
}

// used to pass information among workers (wish golang channels allowed multivalues)
type processingInfo struct {
	pack  *packInfo
	files map[*fileInfo]error
}

func (r *fileRestorer) restoreFiles(ctx context.Context, onError func(path string, err error)) error {
	// TODO conditionally enable when debug log is on
	// for _, file := range r.files {
	// 	dbgmsg := file.location + ": "
	// 	r.idx.forEachFilePack(file, func(packIdx int, packID restic.ID, packBlobs []restic.Blob) bool {
	// 		if packIdx > 0 {
	// 			dbgmsg += ", "
	// 		}
	// 		dbgmsg += "pack{id=" + packID.Str() + ", blobs: "
	// 		for blobIdx, blob := range packBlobs {
	// 			if blobIdx > 0 {
	// 				dbgmsg += ", "
	// 			}
	// 			dbgmsg += blob.ID.Str()
	// 		}
	// 		dbgmsg += "}"
	// 		return true // keep going
	// 	})
	// 	debug.Log(dbgmsg)
	// }

	inprogress := make(map[*fileInfo]struct{})
	queue, err := newPackQueue(r.idx, r.files, func(files map[*fileInfo]struct{}) bool {
		for file := range files {
			if _, found := inprogress[file]; found {
				return true
			}
		}
		return false
	})
	if err != nil {
		return err
	}

	// workers
	downloadCh := make(chan processingInfo)
	feedbackCh := make(chan processingInfo)

	defer close(downloadCh)
	defer close(feedbackCh)

	worker := func() {
		for {
			select {
			case <-ctx.Done():
				return
			case request, ok := <-downloadCh:
				if !ok {
					return // channel closed
				}
				rd, err := r.downloadPack(ctx, request.pack)
				if err == nil {
					r.processPack(ctx, request, rd)
				} else {
					// mark all files as failed
					for file := range request.files {
						request.files[file] = err
					}
				}
				feedbackCh <- request
			}
		}
	}
	for i := 0; i < workerCount; i++ {
		go worker()
	}

	processFeedback := func(pack *packInfo, ferrors map[*fileInfo]error) {
		// update files blobIdx
		// must do it here to avoid race among worker and processing feedback threads
		var success []*fileInfo
		var failure []*fileInfo
		for file, ferr := range ferrors {
			target := r.targetPath(file.location)
			if ferr != nil {
				onError(file.location, ferr)
				r.filesWriter.close(target)
				delete(inprogress, file)
				failure = append(failure, file)
			} else {
				r.idx.forEachFilePack(file, func(packIdx int, packID restic.ID, packBlobs []restic.Blob) bool {
					file.blobs = file.blobs[len(packBlobs):]
					return false // only interesed in the first pack
				})
				if len(file.blobs) == 0 {
					r.filesWriter.close(target)
					delete(inprogress, file)
				}
				success = append(success, file)
			}
		}
		// update the queue and requeueu the pack as necessary
		if !queue.requeuePack(pack, success, failure) {
			r.packCache.remove(pack.id)
			debug.Log("Purged used up pack %s from pack cache", pack.id.Str())
		}
	}

	// the main restore loop
	for !queue.isEmpty() {
		debug.Log("-----------------------------------")
		pack, files := queue.nextPack()
		if pack != nil {
			ferrors := make(map[*fileInfo]error)
			for _, file := range files {
				ferrors[file] = nil
				inprogress[file] = struct{}{}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case downloadCh <- processingInfo{pack: pack, files: ferrors}:
				debug.Log("Scheduled download pack %s (%d files)", pack.id.Str(), len(files))
			case feedback := <-feedbackCh:
				queue.requeuePack(pack, []*fileInfo{}, []*fileInfo{}) // didn't use the pack during this iteration
				processFeedback(feedback.pack, feedback.files)
			}
		} else {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case feedback := <-feedbackCh:
				processFeedback(feedback.pack, feedback.files)
			}
		}
	}

	return nil
}

func (r *fileRestorer) downloadPack(ctx context.Context, pack *packInfo) (readerAtCloser, error) {
	const MaxInt64 = 1<<63 - 1 // odd Go does not have this predefined somewhere

	// calculate pack byte range
	start, end := int64(MaxInt64), int64(0)
	for file := range pack.files {
		r.idx.forEachFilePack(file, func(packIdx int, packID restic.ID, packBlobs []restic.Blob) bool {
			if packID.Equal(pack.id) {
				for _, blob := range packBlobs {
					if start > int64(blob.Offset) {
						start = int64(blob.Offset)
					}
					if end < int64(blob.Offset+blob.Length) {
						end = int64(blob.Offset + blob.Length)
					}
				}
			}

			return true // keep going
		})
	}

	packReader, err := r.packCache.get(pack.id, start, int(end-start), func(offset int64, length int, wr io.WriteSeeker) error {
		h := restic.Handle{Type: restic.DataFile, Name: pack.id.String()}
		return r.packLoader(ctx, h, length, offset, func(rd io.Reader) error {
			// reset the file in case of a download retry
			_, err := wr.Seek(0, io.SeekStart)
			if err != nil {
				return err
			}

			len, err := io.Copy(wr, rd)
			if err != nil {
				return err
			}
			if len != int64(length) {
				return errors.Errorf("unexpected pack size: expected %d but got %d", length, len)
			}

			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	return packReader, nil
}

func (r *fileRestorer) processPack(ctx context.Context, request processingInfo, rd readerAtCloser) {
	defer rd.Close()

	for file := range request.files {
		target := r.targetPath(file.location)
		r.idx.forEachFilePack(file, func(packIdx int, packID restic.ID, packBlobs []restic.Blob) bool {
			for _, blob := range packBlobs {
				debug.Log("Writing blob %s (%d bytes) from pack %s to %s", blob.ID.Str(), blob.Length, packID.Str(), file.location)
				buf, err := r.loadBlob(rd, blob)
				if err == nil {
					err = r.filesWriter.writeToFile(target, buf)
				}
				if err != nil {
					request.files[file] = err
					break // could not restore the file
				}
			}
			return false
		})
	}
}

func (r *fileRestorer) loadBlob(rd io.ReaderAt, blob restic.Blob) ([]byte, error) {
	// TODO reconcile with Repository#loadBlob implementation

	buf := make([]byte, blob.Length)

	n, err := rd.ReadAt(buf, int64(blob.Offset))
	if err != nil {
		return nil, err
	}

	if n != int(blob.Length) {
		return nil, errors.Errorf("error loading blob %v: wrong length returned, want %d, got %d", blob.ID.Str(), blob.Length, n)
	}

	// decrypt
	nonce, ciphertext := buf[:r.key.NonceSize()], buf[r.key.NonceSize():]
	plaintext, err := r.key.Open(ciphertext[:0], nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.Errorf("decrypting blob %v failed: %v", blob.ID, err)
	}

	// check hash
	if !restic.Hash(plaintext).Equal(blob.ID) {
		return nil, errors.Errorf("blob %v returned invalid hash", blob.ID)
	}

	return plaintext, nil
}
