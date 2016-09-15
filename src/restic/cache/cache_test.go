package cache

import (
	"math/rand"
	"restic"
	"restic/test"
	"testing"
)

func TestNew(t *testing.T) {
	c, cleanup := TestNewCache(t)
	defer cleanup()

	buf := test.Random(23, 2*1024*1024)
	id := restic.Hash(buf)

	h := restic.BlobHandle{ID: id, Type: restic.DataBlob}
	if c.HasBlob(h) {
		t.Errorf("cache has blob before storing it")
	}

	test.OK(t, c.PutBlob(h, buf))

	if !c.HasBlob(h) {
		t.Errorf("cache does not have blob after store")
	}

	treeHandle := restic.BlobHandle{ID: id, Type: restic.TreeBlob}
	if c.HasBlob(treeHandle) {
		t.Errorf("cache has tree blob although only a data blob was stored")
	}

	buf2 := make([]byte, len(buf))
	ok, err := c.GetBlob(h, buf2)
	test.OK(t, err)
	if !ok {
		t.Errorf("could not get blob from cache")
	}

	ok, err = c.GetBlob(treeHandle, buf2)
	test.OK(t, err)
	test.Assert(t, !ok, "got blob for tree that was never stored")

	err = c.DeleteBlob(treeHandle)

	test.OK(t, c.DeleteBlob(h))

	if c.HasBlob(h) {
		t.Errorf("cache still has blob after delete")
	}
}

func TestCacheBufsize(t *testing.T) {
	c, cleanup := TestNewCache(t)
	defer cleanup()

	h := restic.BlobHandle{ID: restic.NewRandomID(), Type: restic.TreeBlob}
	buf := test.Random(5, 1000)

	test.OK(t, c.PutBlob(h, buf))

	for i := len(buf) - 1; i <= len(buf)+1; i++ {
		buf2 := make([]byte, i)
		ok, err := c.GetBlob(h, buf2)

		if i == len(buf) {
			test.OK(t, err)
			test.Assert(t, ok, "unable to get blob for correct buf size")
			test.Equals(t, buf, buf2)
			continue
		}

		test.Assert(t, !ok, "ok is true for wrong buffer size %v", i)
		test.Assert(t, err != nil, "error is nil, although buffer size is wrong")
	}
}

type blobIndex struct {
	blobs restic.BlobSet
}

func (idx blobIndex) Has(id restic.ID, t restic.BlobType) bool {
	_, ok := idx.blobs[restic.BlobHandle{ID: id, Type: t}]
	return ok
}

func TestUpdateBlobs(t *testing.T) {
	c, cleanup := TestNewCache(t)
	defer cleanup()

	blobs := restic.NewBlobSet()

	buf := test.Random(23, 15*1024)
	for i := 0; i < 100; i++ {
		id := restic.NewRandomID()
		h := restic.BlobHandle{ID: id, Type: restic.TreeBlob}
		err := c.PutBlob(h, buf)
		test.OK(t, err)
		blobs.Insert(h)
	}

	// use an index with all blobs, this must not remove anything
	idx := blobIndex{blobs: blobs}
	test.OK(t, c.UpdateBlobs(idx))

	for h := range blobs {
		if !c.HasBlob(h) {
			t.Errorf("blob %v was removed\n", h)
		}
	}

	// next, remove about 20% of the blobs
	keepBlobs := restic.NewBlobSet()
	for h := range blobs {
		if rand.Float32() <= 0.8 {
			keepBlobs.Insert(h)
		}
	}
	idx = blobIndex{blobs: keepBlobs}
	test.OK(t, c.UpdateBlobs(idx))

	for h := range blobs {
		if keepBlobs.Has(h) {
			if !c.HasBlob(h) {
				t.Errorf("blob %v was removed\n", h)
			}
			continue
		}

		if c.HasBlob(h) {
			t.Errorf("blob %v was kept although it should've been removed", h)
		}
	}

	// remove the remaining blobs
	idx = blobIndex{blobs: restic.NewBlobSet()}
	test.OK(t, c.UpdateBlobs(idx))
	for h := range blobs {
		if c.HasBlob(h) {
			t.Errorf("blob %v was not removed\n", h)
		}
	}
}

type fileIndex struct {
	files map[restic.Handle]struct{}
}

func (idx fileIndex) Test(t restic.FileType, name string) (bool, error) {
	h := restic.Handle{Type: t, Name: name}
	_, ok := idx.files[h]
	return ok, nil
}

func TestUpdateFiles(t *testing.T) {
	c, cleanup := TestNewCache(t)
	defer cleanup()

	files := make(map[restic.Handle]struct{})

	buf := test.Random(23, 15*1024)
	for i := 0; i < 10; i++ {
		id := restic.NewRandomID()
		h := restic.Handle{Type: restic.IndexFile, Name: id.String()}
		err := c.PutFile(h, buf)
		test.OK(t, err)
		files[h] = struct{}{}
	}

	// use an index with all files, this must not remove anything
	idx := fileIndex{files: files}
	test.OK(t, c.UpdateFiles(idx))

	for h := range files {
		if !c.HasFile(h) {
			t.Errorf("file %v was removed\n", h)
		}
	}

	// next, remove about 20% of the files
	keepFiles := make(map[restic.Handle]struct{})
	for h := range files {
		if rand.Float32() <= 0.8 {
			keepFiles[h] = struct{}{}
		}
	}
	idx = fileIndex{files: keepFiles}
	test.OK(t, c.UpdateFiles(idx))

	for h := range files {
		if _, ok := keepFiles[h]; ok {
			if !c.HasFile(h) {
				t.Errorf("file %v was removed\n", h)
			}
			continue
		}

		if c.HasFile(h) {
			t.Errorf("file %v was kept although it should've been removed", h)
		}
	}

	// remove the remaining files
	idx = fileIndex{files: make(map[restic.Handle]struct{})}
	test.OK(t, c.UpdateFiles(idx))
	for h := range files {
		if c.HasFile(h) {
			t.Errorf("file %v was not removed\n", h)
		}
	}
}
