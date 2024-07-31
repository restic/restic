package restorer

import (
	"context"
	"path/filepath"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/restore"
)

const (
	largeFileBlobCount = 25
)

// information about regular file being restored
type fileInfo struct {
	lock       sync.Mutex
	inProgress bool
	sparse     bool
	size       int64
	location   string      // file on local filesystem relative to restorer basedir
	blobs      interface{} // blobs of the file
	state      *fileState
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

type blobsLoaderFn func(ctx context.Context, packID restic.ID, blobs []restic.Blob, handleBlobFn func(blob restic.BlobHandle, buf []byte, err error) error) error

// fileRestorer restores set of files
type fileRestorer struct {
	idx         func(restic.BlobType, restic.ID) []restic.PackedBlob
	blobsLoader blobsLoaderFn

	workerCount int
	filesWriter *filesWriter
	zeroChunk   restic.ID
	sparse      bool
	progress    *restore.Progress

	allowRecursiveDelete bool

	dst   string
	files []*fileInfo
	Error func(string, error) error
}

func newFileRestorer(dst string,
	blobsLoader blobsLoaderFn,
	idx func(restic.BlobType, restic.ID) []restic.PackedBlob,
	connections uint,
	sparse bool,
	allowRecursiveDelete bool,
	progress *restore.Progress) *fileRestorer {

	// as packs are streamed the concurrency is limited by IO
	workerCount := int(connections)

	return &fileRestorer{
		idx:                  idx,
		blobsLoader:          blobsLoader,
		filesWriter:          newFilesWriter(workerCount, allowRecursiveDelete),
		zeroChunk:            repository.ZeroChunk(),
		sparse:               sparse,
		progress:             progress,
		allowRecursiveDelete: allowRecursiveDelete,
		workerCount:          workerCount,
		dst:                  dst,
		Error:                restorerAbortOnAllErrors,
	}
}

func (r *fileRestorer) addFile(location string, content restic.IDs, size int64, state *fileState) {
	r.files = append(r.files, &fileInfo{location: location, blobs: content, size: size, state: state})
}

func (r *fileRestorer) targetPath(location string) string {
	return filepath.Join(r.dst, location)
}

func (r *fileRestorer) forEachBlob(blobIDs []restic.ID, fn func(packID restic.ID, packBlob restic.Blob, idx int, fileOffset int64)) error {
	if len(blobIDs) == 0 {
		return nil
	}

	fileOffset := int64(0)
	for i, blobID := range blobIDs {
		packs := r.idx(restic.DataBlob, blobID)
		if len(packs) == 0 {
			return errors.Errorf("Unknown blob %s", blobID.String())
		}
		pb := packs[0]
		fn(pb.PackID, pb.Blob, i, fileOffset)
		fileOffset += int64(pb.DataLength())
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
		if ctx.Err() != nil {
			return ctx.Err()
		}

		fileBlobs := file.blobs.(restic.IDs)
		largeFile := len(fileBlobs) > largeFileBlobCount
		var packsMap map[restic.ID][]fileBlobInfo
		if largeFile {
			packsMap = make(map[restic.ID][]fileBlobInfo)
			file.blobs = packsMap
		}
		restoredBlobs := false
		err := r.forEachBlob(fileBlobs, func(packID restic.ID, blob restic.Blob, idx int, fileOffset int64) {
			if !file.state.HasMatchingBlob(idx) {
				if largeFile {
					packsMap[packID] = append(packsMap[packID], fileBlobInfo{id: blob.ID, offset: fileOffset})
				}
				restoredBlobs = true
			} else {
				r.reportBlobProgress(file, uint64(blob.DataLength()))
				// completely ignore blob
				return
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
			if blob.ID.Equal(r.zeroChunk) {
				file.sparse = r.sparse
			}
		})
		if err != nil {
			// repository index is messed up, can't do anything
			return err
		}

		if len(fileBlobs) == 1 {
			// no need to preallocate files with a single block, thus we can always consider them to be sparse
			// in addition, a short chunk will never match r.zeroChunk which would prevent sparseness for short files
			file.sparse = r.sparse
		}
		if file.state != nil {
			// The restorer currently cannot punch new holes into an existing files.
			// Thus sections that contained data but should be sparse after restoring
			// the snapshot would still contain the old data resulting in a corrupt restore.
			file.sparse = false
		}

		// empty file or one with already uptodate content. Make sure that the file size is correct
		if !restoredBlobs {
			err := r.truncateFileToSize(file.location, file.size)
			if errFile := r.sanitizeError(file, err); errFile != nil {
				return errFile
			}

			// the progress events were already sent for non-zero size files
			if file.size == 0 {
				r.reportBlobProgress(file, 0)
			}
		}
	}
	// drop no longer necessary file list
	r.files = nil

	wg, ctx := errgroup.WithContext(ctx)
	downloadCh := make(chan *packInfo)

	worker := func() error {
		for pack := range downloadCh {
			if err := r.downloadPack(ctx, pack); err != nil {
				return err
			}
		}
		return nil
	}
	for i := 0; i < r.workerCount; i++ {
		wg.Go(worker)
	}

	// the main restore loop
	wg.Go(func() error {
		defer close(downloadCh)
		for _, id := range packOrder {
			pack := packs[id]
			// allow garbage collection of packInfo
			delete(packs, id)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case downloadCh <- pack:
				debug.Log("Scheduled download pack %s", pack.id.Str())
			}
		}
		return nil
	})

	return wg.Wait()
}

func (r *fileRestorer) truncateFileToSize(location string, size int64) error {
	f, err := createFile(r.targetPath(location), size, false, r.allowRecursiveDelete)
	if err != nil {
		return err
	}
	return f.Close()
}

type blobToFileOffsetsMapping map[restic.ID]struct {
	files map[*fileInfo][]int64 // file -> offsets (plural!) of the blob in the file
	blob  restic.Blob
}

func (r *fileRestorer) downloadPack(ctx context.Context, pack *packInfo) error {
	// calculate blob->[]files->[]offsets mappings
	blobs := make(blobToFileOffsetsMapping)
	for file := range pack.files {
		addBlob := func(blob restic.Blob, fileOffset int64) {
			blobInfo, ok := blobs[blob.ID]
			if !ok {
				blobInfo.files = make(map[*fileInfo][]int64)
				blobInfo.blob = blob
				blobs[blob.ID] = blobInfo
			}
			blobInfo.files[file] = append(blobInfo.files[file], fileOffset)
		}
		if fileBlobs, ok := file.blobs.(restic.IDs); ok {
			err := r.forEachBlob(fileBlobs, func(packID restic.ID, blob restic.Blob, idx int, fileOffset int64) {
				if packID.Equal(pack.id) && !file.state.HasMatchingBlob(idx) {
					addBlob(blob, fileOffset)
				}
			})
			if err != nil {
				// restoreFiles should have caught this error before
				panic(err)
			}
		} else if packsMap, ok := file.blobs.(map[restic.ID][]fileBlobInfo); ok {
			for _, blob := range packsMap[pack.id] {
				idxPacks := r.idx(restic.DataBlob, blob.id)
				for _, idxPack := range idxPacks {
					if idxPack.PackID.Equal(pack.id) {
						addBlob(idxPack.Blob, blob.offset)
						break
					}
				}
			}
		}
	}

	// track already processed blobs for precise error reporting
	processedBlobs := restic.NewBlobSet()
	err := r.downloadBlobs(ctx, pack.id, blobs, processedBlobs)
	return r.reportError(blobs, processedBlobs, err)
}

func (r *fileRestorer) sanitizeError(file *fileInfo, err error) error {
	switch err {
	case nil, context.Canceled, context.DeadlineExceeded:
		// Context errors are permanent.
		return err
	default:
		return r.Error(file.location, err)
	}
}

func (r *fileRestorer) reportError(blobs blobToFileOffsetsMapping, processedBlobs restic.BlobSet, err error) error {
	if err == nil {
		return nil
	}

	// only report error for not yet processed blobs
	affectedFiles := make(map[*fileInfo]struct{})
	for _, entry := range blobs {
		if processedBlobs.Has(entry.blob.BlobHandle) {
			continue
		}
		for file := range entry.files {
			affectedFiles[file] = struct{}{}
		}
	}

	for file := range affectedFiles {
		if errFile := r.sanitizeError(file, err); errFile != nil {
			return errFile
		}
	}
	return nil
}

func (r *fileRestorer) downloadBlobs(ctx context.Context, packID restic.ID,
	blobs blobToFileOffsetsMapping, processedBlobs restic.BlobSet) error {

	blobList := make([]restic.Blob, 0, len(blobs))
	for _, entry := range blobs {
		blobList = append(blobList, entry.blob)
	}
	return r.blobsLoader(ctx, packID, blobList,
		func(h restic.BlobHandle, blobData []byte, err error) error {
			processedBlobs.Insert(h)
			blob := blobs[h.ID]
			if err != nil {
				for file := range blob.files {
					if errFile := r.sanitizeError(file, err); errFile != nil {
						return errFile
					}
				}
				return nil
			}
			for file, offsets := range blob.files {
				for _, offset := range offsets {
					// avoid long cancelation delays for frequently used blobs
					if ctx.Err() != nil {
						return ctx.Err()
					}

					writeToFile := func() error {
						// this looks overly complicated and needs explanation
						// two competing requirements:
						// - must create the file once and only once
						// - should allow concurrent writes to the file
						// so write the first blob while holding file lock
						// write other blobs after releasing the lock
						createSize := int64(-1)
						file.lock.Lock()
						if file.inProgress {
							file.lock.Unlock()
						} else {
							defer file.lock.Unlock()
							file.inProgress = true
							createSize = file.size
						}
						writeErr := r.filesWriter.writeToFile(r.targetPath(file.location), blobData, offset, createSize, file.sparse)
						r.reportBlobProgress(file, uint64(len(blobData)))
						return writeErr
					}
					err := r.sanitizeError(file, writeToFile())
					if err != nil {
						return err
					}
				}
			}
			return nil
		})
}

func (r *fileRestorer) reportBlobProgress(file *fileInfo, blobSize uint64) {
	action := restore.ActionFileUpdated
	if file.state == nil {
		action = restore.ActionFileRestored
	}
	r.progress.AddProgress(file.location, action, uint64(blobSize), uint64(file.size))
}
