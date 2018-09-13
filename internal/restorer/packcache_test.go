package restorer

import (
	"io"
	"testing"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func assertNotOK(t *testing.T, msg string, err error) {
	rtest.Assert(t, err != nil, msg+" did not fail")
}

func TestBytesWriterSeeker(t *testing.T) {
	wr := &bytesWriteSeeker{data: make([]byte, 10)}

	n, err := wr.Write([]byte{1, 2})
	rtest.OK(t, err)
	rtest.Equals(t, 2, n)
	rtest.Equals(t, []byte{1, 2}, wr.data[0:2])

	n64, err := wr.Seek(0, io.SeekStart)
	rtest.OK(t, err)
	rtest.Equals(t, int64(0), n64)

	n, err = wr.Write([]byte{0, 1, 2, 3, 4})
	rtest.OK(t, err)
	rtest.Equals(t, 5, n)
	n, err = wr.Write([]byte{5, 6, 7, 8, 9})
	rtest.OK(t, err)
	rtest.Equals(t, 5, n)
	rtest.Equals(t, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, wr.data)

	// negative tests
	_, err = wr.Write([]byte{1})
	assertNotOK(t, "write overflow", err)
	_, err = wr.Seek(1, io.SeekStart)
	assertNotOK(t, "unsupported seek", err)
}

func TestPackCacheBasic(t *testing.T) {
	assertReader := func(expected []byte, offset int64, rd io.ReaderAt) {
		actual := make([]byte, len(expected))
		rd.ReadAt(actual, offset)
		rtest.Equals(t, expected, actual)
	}

	c := newPackCache(10)

	id := restic.NewRandomID()

	// load pack to the cache
	rd, err := c.get(id, 10, 5, func(offset int64, length int, wr io.WriteSeeker) error {
		rtest.Equals(t, int64(10), offset)
		rtest.Equals(t, 5, length)
		wr.Write([]byte{1, 2, 3, 4, 5})
		return nil
	})
	rtest.OK(t, err)
	assertReader([]byte{1, 2, 3, 4, 5}, 10, rd)

	// must close pack reader before can request it again
	_, err = c.get(id, 10, 5, func(offset int64, length int, wr io.WriteSeeker) error {
		t.Error("unexpected cache load call")
		return nil
	})
	assertNotOK(t, "double-reservation", err)

	// close the pack reader and get it from cache
	rd.Close()
	rd, err = c.get(id, 10, 5, func(offset int64, length int, wr io.WriteSeeker) error {
		t.Error("unexpected cache load call")
		return nil
	})
	rtest.OK(t, err)
	assertReader([]byte{1, 2, 3, 4, 5}, 10, rd)

	// close the pack reader and remove the pack from cache, assert the pack is loaded on request
	rd.Close()
	c.remove(id)
	rd, err = c.get(id, 10, 5, func(offset int64, length int, wr io.WriteSeeker) error {
		rtest.Equals(t, int64(10), offset)
		rtest.Equals(t, 5, length)
		wr.Write([]byte{1, 2, 3, 4, 5})
		return nil
	})
	rtest.OK(t, err)
	assertReader([]byte{1, 2, 3, 4, 5}, 10, rd)
}

func TestPackCacheInvalidRange(t *testing.T) {
	c := newPackCache(10)

	id := restic.NewRandomID()

	_, err := c.get(id, -1, 1, func(offset int64, length int, wr io.WriteSeeker) error {
		t.Error("unexpected cache load call")
		return nil
	})
	assertNotOK(t, "negative offset request", err)

	_, err = c.get(id, 0, 0, func(offset int64, length int, wr io.WriteSeeker) error {
		t.Error("unexpected cache load call")
		return nil
	})
	assertNotOK(t, "zero length request", err)

	_, err = c.get(id, 0, -1, func(offset int64, length int, wr io.WriteSeeker) error {
		t.Error("unexpected cache load call")
		return nil
	})
	assertNotOK(t, "negative length", err)
}

func TestPackCacheCapacity(t *testing.T) {
	c := newPackCache(10)

	id1, id2, id3 := restic.NewRandomID(), restic.NewRandomID(), restic.NewRandomID()

	// load and reserve pack1
	rd1, err := c.get(id1, 0, 5, func(offset int64, length int, wr io.WriteSeeker) error {
		wr.Write([]byte{1, 2, 3, 4, 5})
		return nil
	})
	rtest.OK(t, err)

	// load and reserve pack2
	_, err = c.get(id2, 0, 5, func(offset int64, length int, wr io.WriteSeeker) error {
		wr.Write([]byte{1, 2, 3, 4, 5})
		return nil
	})
	rtest.OK(t, err)

	// can't load pack3 because not enough space in the cache
	_, err = c.get(id3, 0, 5, func(offset int64, length int, wr io.WriteSeeker) error {
		t.Error("unexpected cache load call")
		return nil
	})
	assertNotOK(t, "request over capacity", err)

	// release pack1 and try again
	rd1.Close()
	rd3, err := c.get(id3, 0, 5, func(offset int64, length int, wr io.WriteSeeker) error {
		wr.Write([]byte{1, 2, 3, 4, 5})
		return nil
	})
	rtest.OK(t, err)

	// release pack3 and load pack1 (should not come from cache)
	rd3.Close()
	loaded := false
	rd1, err = c.get(id1, 0, 5, func(offset int64, length int, wr io.WriteSeeker) error {
		wr.Write([]byte{1, 2, 3, 4, 5})
		loaded = true
		return nil
	})
	rtest.OK(t, err)
	rtest.Equals(t, true, loaded)
}

func TestPackCacheDownsizeRecord(t *testing.T) {
	c := newPackCache(10)

	id := restic.NewRandomID()

	// get bigger range first
	rd, err := c.get(id, 5, 5, func(offset int64, length int, wr io.WriteSeeker) error {
		wr.Write([]byte{1, 2, 3, 4, 5})
		return nil
	})
	rtest.OK(t, err)
	rd.Close()

	// invalid "resize" requests
	_, err = c.get(id, 5, 10, func(offset int64, length int, wr io.WriteSeeker) error {
		t.Error("unexpected pack load")
		return nil
	})
	assertNotOK(t, "resize cached record", err)

	// invalid before cached range request
	_, err = c.get(id, 0, 5, func(offset int64, length int, wr io.WriteSeeker) error {
		t.Error("unexpected pack load")
		return nil
	})
	assertNotOK(t, "before cached range request", err)

	// invalid after cached range request
	_, err = c.get(id, 10, 5, func(offset int64, length int, wr io.WriteSeeker) error {
		t.Error("unexpected pack load")
		return nil
	})
	assertNotOK(t, "after cached range request", err)

	// now get smaller "nested" range
	rd, err = c.get(id, 7, 1, func(offset int64, length int, wr io.WriteSeeker) error {
		t.Error("unexpected pack load")
		return nil
	})
	rtest.OK(t, err)

	// assert expected data
	buf := make([]byte, 1)
	rd.ReadAt(buf, 7)
	rtest.Equals(t, byte(3), buf[0])
	_, err = rd.ReadAt(buf, 0)
	assertNotOK(t, "read before downsized pack range", err)
	_, err = rd.ReadAt(buf, 9)
	assertNotOK(t, "read after downsized pack range", err)

	// can't request downsized record again
	_, err = c.get(id, 7, 1, func(offset int64, length int, wr io.WriteSeeker) error {
		t.Error("unexpected pack load")
		return nil
	})
	assertNotOK(t, "double-allocation of cache record subrange", err)

	// can't request another subrange of the original record
	_, err = c.get(id, 6, 1, func(offset int64, length int, wr io.WriteSeeker) error {
		t.Error("unexpected pack load")
		return nil
	})
	assertNotOK(t, "allocation of another subrange of cache record", err)

	// release downsized record and assert the original is back in the cache
	rd.Close()
	rd, err = c.get(id, 5, 5, func(offset int64, length int, wr io.WriteSeeker) error {
		t.Error("unexpected pack load")
		return nil
	})
	rtest.OK(t, err)
	rd.Close()
}

func TestPackCacheFailedDownload(t *testing.T) {
	c := newPackCache(10)
	assertEmpty := func() {
		rtest.Equals(t, 0, len(c.cachedPacks))
		rtest.Equals(t, 10, c.capacity)
		rtest.Equals(t, 0, c.reservedCapacity)
		rtest.Equals(t, 0, c.allocatedCapacity)
	}

	_, err := c.get(restic.NewRandomID(), 0, 5, func(offset int64, length int, wr io.WriteSeeker) error {
		return errors.Errorf("expected induced test error")
	})
	assertNotOK(t, "not enough bytes read", err)
	assertEmpty()

	_, err = c.get(restic.NewRandomID(), 0, 5, func(offset int64, length int, wr io.WriteSeeker) error {
		wr.Write([]byte{1})
		return nil
	})
	assertNotOK(t, "not enough bytes read", err)
	assertEmpty()

	_, err = c.get(restic.NewRandomID(), 0, 5, func(offset int64, length int, wr io.WriteSeeker) error {
		wr.Write([]byte{1, 2, 3, 4, 5, 6})
		return nil
	})
	assertNotOK(t, "too many bytes read", err)
	assertEmpty()
}

func TestPackCacheInvalidRequests(t *testing.T) {
	c := newPackCache(10)

	id := restic.NewRandomID()

	//
	rd, _ := c.get(id, 0, 1, func(offset int64, length int, wr io.WriteSeeker) error {
		wr.Write([]byte{1})
		return nil
	})
	assertNotOK(t, "remove() reserved pack", c.remove(id))
	rtest.OK(t, rd.Close())
	assertNotOK(t, "multiple reader Close() calls)", rd.Close())

	//
	rtest.OK(t, c.remove(id))
	assertNotOK(t, "double remove() the same pack", c.remove(id))
}

func TestPackCacheRecord(t *testing.T) {
	rd := &packCacheRecord{
		offset: 10,
		data:   []byte{1},
	}
	buf := make([]byte, 1)
	n, err := rd.ReadAt(buf, 10)
	rtest.OK(t, err)
	rtest.Equals(t, 1, n)
	rtest.Equals(t, byte(1), buf[0])

	_, err = rd.ReadAt(buf, 0)
	assertNotOK(t, "read before loaded range", err)

	_, err = rd.ReadAt(buf, 11)
	assertNotOK(t, "read after loaded range", err)

	_, err = rd.ReadAt(make([]byte, 2), 10)
	assertNotOK(t, "read more than available data", err)
}
