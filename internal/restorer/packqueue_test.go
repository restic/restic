package restorer

import (
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func processPack(t *testing.T, data *TestRepo, pack *packInfo, files []*fileInfo) {
	for _, file := range files {
		data.idx.forEachFilePack(file, func(packIdx int, packID restic.ID, packBlobs []restic.Blob) bool {
			// assert file's head pack
			rtest.Equals(t, pack.id, packID)
			file.blobs = file.blobs[len(packBlobs):]
			return false // only interested in the head pack
		})
	}
}

func TestPackQueueBasic(t *testing.T) {
	data := newTestRepo([]TestFile{
		TestFile{
			name: "file",
			blobs: []TestBlob{
				TestBlob{"data1", "pack1"},
				TestBlob{"data2", "pack2"},
			},
		},
	})

	queue, err := newPackQueue(data.idx, data.files, func(_ map[*fileInfo]struct{}) bool { return false })
	rtest.OK(t, err)

	// assert initial queue state
	rtest.Equals(t, false, queue.isEmpty())
	rtest.Equals(t, 0, queue.packs[data.packID("pack1")].cost)
	rtest.Equals(t, 1, queue.packs[data.packID("pack2")].cost)

	// get first pack
	pack, files := queue.nextPack()
	rtest.Equals(t, "pack1", data.packName(pack))
	rtest.Equals(t, 1, len(files))
	rtest.Equals(t, false, queue.isEmpty())
	// TODO assert pack is inprogress

	// can't process the second pack until the first one is processed
	{
		pack, files := queue.nextPack()
		rtest.Equals(t, true, pack == nil)
		rtest.Equals(t, true, files == nil)
		rtest.Equals(t, false, queue.isEmpty())
	}

	// requeue the pack without processing
	rtest.Equals(t, true, queue.requeuePack(pack, []*fileInfo{}, []*fileInfo{}))
	rtest.Equals(t, false, queue.isEmpty())
	rtest.Equals(t, 0, queue.packs[data.packID("pack1")].cost)
	rtest.Equals(t, 1, queue.packs[data.packID("pack2")].cost)

	// get the first pack again
	pack, files = queue.nextPack()
	rtest.Equals(t, "pack1", data.packName(pack))
	rtest.Equals(t, 1, len(files))
	rtest.Equals(t, false, queue.isEmpty())

	// process the first pack and return it to the queue
	processPack(t, data, pack, files)
	rtest.Equals(t, false, queue.requeuePack(pack, files, []*fileInfo{}))
	rtest.Equals(t, 0, queue.packs[data.packID("pack2")].cost)

	// get the second pack
	pack, files = queue.nextPack()
	rtest.Equals(t, "pack2", data.packName(pack))
	rtest.Equals(t, 1, len(files))
	rtest.Equals(t, false, queue.isEmpty())

	// process the second pack and return it to the queue
	processPack(t, data, pack, files)
	rtest.Equals(t, false, queue.requeuePack(pack, files, []*fileInfo{}))

	// all packs processed
	rtest.Equals(t, true, queue.isEmpty())
}

func TestPackQueueFailedFile(t *testing.T) {
	// point of this test is to assert that enqueuePack removes
	// all references to files that failed restore

	data := newTestRepo([]TestFile{
		TestFile{
			name: "file",
			blobs: []TestBlob{
				TestBlob{"data1", "pack1"},
				TestBlob{"data2", "pack2"},
			},
		},
	})

	queue, err := newPackQueue(data.idx, data.files, func(_ map[*fileInfo]struct{}) bool { return false })
	rtest.OK(t, err)

	pack, files := queue.nextPack()
	rtest.Equals(t, false, queue.requeuePack(pack, []*fileInfo{}, files /*failed*/))
	rtest.Equals(t, true, queue.isEmpty())
}

func TestPackQueueOrderingCost(t *testing.T) {
	// assert pack1 is selected before pack2:
	// pack1 is ready to restore file1, pack2 is ready to restore file2
	// but pack2 cannot be immediately used to restore file1

	data := newTestRepo([]TestFile{
		TestFile{
			name: "file1",
			blobs: []TestBlob{
				TestBlob{"data1", "pack1"},
				TestBlob{"data2", "pack2"},
			},
		},
		TestFile{
			name: "file2",
			blobs: []TestBlob{
				TestBlob{"data2", "pack2"},
			},
		},
	})

	queue, err := newPackQueue(data.idx, data.files, func(_ map[*fileInfo]struct{}) bool { return false })
	rtest.OK(t, err)

	// assert initial pack costs
	rtest.Equals(t, 0, data.pack(queue, "pack1").cost)
	rtest.Equals(t, 0, data.pack(queue, "pack1").index) // head of the heap
	rtest.Equals(t, 1, data.pack(queue, "pack2").cost)
	rtest.Equals(t, 1, data.pack(queue, "pack2").index)

	pack, files := queue.nextPack()
	// assert selected pack and queue state
	rtest.Equals(t, "pack1", data.packName(pack))
	// process the pack
	processPack(t, data, pack, files)
	rtest.Equals(t, false, queue.requeuePack(pack, files, []*fileInfo{}))
}

func TestPackQueueOrderingInprogress(t *testing.T) {
	// finish restoring one file before starting another

	data := newTestRepo([]TestFile{
		TestFile{
			name: "file1",
			blobs: []TestBlob{
				TestBlob{"data1-1", "pack1-1"},
				TestBlob{"data1-2", "pack1-2"},
			},
		},
		TestFile{
			name: "file2",
			blobs: []TestBlob{
				TestBlob{"data2-1", "pack2-1"},
				TestBlob{"data2-2", "pack2-2"},
			},
		},
	})

	var inprogress *fileInfo
	queue, err := newPackQueue(data.idx, data.files, func(files map[*fileInfo]struct{}) bool {
		_, found := files[inprogress]
		return found
	})
	rtest.OK(t, err)

	// first pack of a file
	pack, files := queue.nextPack()
	rtest.Equals(t, 1, len(files))
	file := files[0]
	processPack(t, data, pack, files)
	inprogress = files[0]
	queue.requeuePack(pack, files, []*fileInfo{})

	// second pack of the same file
	pack, files = queue.nextPack()
	rtest.Equals(t, 1, len(files))
	rtest.Equals(t, true, file == files[0]) // same file as before
	processPack(t, data, pack, files)
	inprogress = nil
	queue.requeuePack(pack, files, []*fileInfo{})

	// first pack of the second file
	pack, files = queue.nextPack()
	rtest.Equals(t, 1, len(files))
	rtest.Equals(t, false, file == files[0]) // different file as before
}

func TestPackQueuePackMultiuse(t *testing.T) {
	// the same pack is required multiple times to restore the same file

	data := newTestRepo([]TestFile{
		TestFile{
			name: "file",
			blobs: []TestBlob{
				TestBlob{"data1", "pack1"},
				TestBlob{"data2", "pack2"},
				TestBlob{"data3", "pack1"}, // pack1 reuse, new blob
				TestBlob{"data2", "pack2"}, // pack2 reuse, same blob
			},
		},
	})

	queue, err := newPackQueue(data.idx, data.files, func(_ map[*fileInfo]struct{}) bool { return false })
	rtest.OK(t, err)

	pack, files := queue.nextPack()
	rtest.Equals(t, "pack1", data.packName(pack))
	rtest.Equals(t, 1, len(pack.files))
	processPack(t, data, pack, files)
	rtest.Equals(t, true, queue.requeuePack(pack, files, []*fileInfo{}))

	pack, files = queue.nextPack()
	rtest.Equals(t, "pack2", data.packName(pack))
	rtest.Equals(t, 1, len(pack.files))
	processPack(t, data, pack, files)
	rtest.Equals(t, true, queue.requeuePack(pack, files, []*fileInfo{}))

	pack, files = queue.nextPack()
	rtest.Equals(t, "pack1", data.packName(pack))
	processPack(t, data, pack, files)
	rtest.Equals(t, false, queue.requeuePack(pack, files, []*fileInfo{}))

	pack, files = queue.nextPack()
	rtest.Equals(t, "pack2", data.packName(pack))
	processPack(t, data, pack, files)
	rtest.Equals(t, false, queue.requeuePack(pack, files, []*fileInfo{}))

	rtest.Equals(t, true, queue.isEmpty())
}
