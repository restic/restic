// +build !openbsd
// +build !windows

package fuse

import (
	"bytes"
	"math/rand"
	"testing"
	"time"

	"restic/errors"

	"bazil.org/fuse"

	"restic"
	. "restic/test"
)

type MockRepo struct {
	blobs map[restic.ID][]byte
}

func NewMockRepo(content map[restic.ID][]byte) *MockRepo {
	return &MockRepo{blobs: content}
}

func (m *MockRepo) LookupBlobSize(id restic.ID, t restic.BlobType) (uint, error) {
	buf, ok := m.blobs[id]
	if !ok {
		return 0, errors.New("blob not found")
	}

	return uint(len(buf)), nil
}

func (m *MockRepo) LoadBlob(t restic.BlobType, id restic.ID, buf []byte) (int, error) {
	size, err := m.LookupBlobSize(id, t)
	if err != nil {
		return 0, err
	}

	if uint(len(buf)) < size {
		return 0, errors.New("buffer too small")
	}

	buf = buf[:size]
	copy(buf, m.blobs[id])
	return int(size), nil
}

type MockContext struct{}

func (m MockContext) Deadline() (time.Time, bool)       { return time.Now(), false }
func (m MockContext) Done() <-chan struct{}             { return nil }
func (m MockContext) Err() error                        { return nil }
func (m MockContext) Value(key interface{}) interface{} { return nil }

var testContent = genTestContent()
var testContentLengths = []uint{
	4646 * 1024,
	655 * 1024,
	378 * 1024,
	8108 * 1024,
	558 * 1024,
}
var testMaxFileSize uint

func genTestContent() map[restic.ID][]byte {
	m := make(map[restic.ID][]byte)

	for _, length := range testContentLengths {
		buf := Random(int(length), int(length))
		id := restic.Hash(buf)
		m[id] = buf
		testMaxFileSize += length
	}

	return m
}

const maxBufSize = 20 * 1024 * 1024

func testRead(t *testing.T, f *file, offset, length int, data []byte) {
	ctx := MockContext{}

	req := &fuse.ReadRequest{
		Offset: int64(offset),
		Size:   length,
	}
	resp := &fuse.ReadResponse{
		Data: make([]byte, length),
	}
	OK(t, f.Read(ctx, req, resp))
}

var offsetReadsTests = []struct {
	offset, length int
}{
	{0, 5 * 1024 * 1024},
	{4000 * 1024, 1000 * 1024},
}

func TestFuseFile(t *testing.T) {
	repo := NewMockRepo(testContent)
	ctx := MockContext{}

	memfile := make([]byte, 0, maxBufSize)

	var ids restic.IDs
	for id, buf := range repo.blobs {
		ids = append(ids, id)
		memfile = append(memfile, buf...)
	}

	node := &restic.Node{
		Name:    "foo",
		Inode:   23,
		Mode:    0742,
		Size:    42,
		Content: ids,
	}
	f, err := newFile(repo, node, false)
	OK(t, err)

	attr := fuse.Attr{}
	OK(t, f.Attr(ctx, &attr))

	Equals(t, node.Inode, attr.Inode)
	Equals(t, node.Mode, attr.Mode)
	Equals(t, node.Size, attr.Size)
	Equals(t, (node.Size/uint64(attr.BlockSize))+1, attr.Blocks)

	for i, test := range offsetReadsTests {
		b := memfile[test.offset : test.offset+test.length]
		buf := make([]byte, test.length)
		testRead(t, f, test.offset, test.length, buf)
		if !bytes.Equal(b, buf) {
			t.Errorf("test %d failed, wrong data returned", i)
		}
	}

	for i := 0; i < 200; i++ {
		length := rand.Intn(int(testMaxFileSize) / 2)
		offset := rand.Intn(int(testMaxFileSize))
		if length+offset > int(testMaxFileSize) {
			diff := length + offset - int(testMaxFileSize)
			length -= diff
		}

		b := memfile[offset : offset+length]
		buf := make([]byte, length)
		testRead(t, f, offset, length, buf)
		if !bytes.Equal(b, buf) {
			t.Errorf("test %d failed (offset %d, length %d), wrong data returned", i, offset, length)
		}
	}
}
