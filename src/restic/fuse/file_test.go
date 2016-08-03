// +build !openbsd
// +build !windows

package fuse

import (
	"bytes"
	"errors"
	"math/rand"
	"testing"
	"time"

	"bazil.org/fuse"

	"restic"
	"restic/backend"
	"restic/pack"
	. "restic/test"
)

type MockRepo struct {
	blobs map[backend.ID][]byte
}

func NewMockRepo(content map[backend.ID][]byte) *MockRepo {
	return &MockRepo{blobs: content}
}

func (m *MockRepo) LookupBlobSize(id backend.ID, t pack.BlobType) (uint, error) {
	buf, ok := m.blobs[id]
	if !ok {
		return 0, errors.New("blob not found")
	}

	return uint(len(buf)), nil
}

func (m *MockRepo) LoadBlob(id backend.ID, t pack.BlobType, buf []byte) ([]byte, error) {
	size, err := m.LookupBlobSize(id, t)
	if err != nil {
		return nil, err
	}

	if uint(cap(buf)) < size {
		return nil, errors.New("buffer too small")
	}

	buf = buf[:size]
	copy(buf, m.blobs[id])
	return buf, nil
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

func genTestContent() map[backend.ID][]byte {
	m := make(map[backend.ID][]byte)

	for _, length := range testContentLengths {
		buf := Random(int(length), int(length))
		id := backend.Hash(buf)
		m[id] = buf
		testMaxFileSize += length
	}

	return m
}

const maxBufSize = 20 * 1024 * 1024

func testRead(t *testing.T, f *file, offset, length int, data []byte) []byte {
	ctx := MockContext{}

	req := &fuse.ReadRequest{
		Offset: int64(offset),
		Size:   length,
	}
	resp := &fuse.ReadResponse{
		Data: make([]byte, length),
	}
	OK(t, f.Read(ctx, req, resp))

	return resp.Data
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

	var ids backend.IDs
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
		res := testRead(t, f, test.offset, test.length, b)
		if !bytes.Equal(b, res) {
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
		res := testRead(t, f, offset, length, b)
		if !bytes.Equal(b, res) {
			t.Errorf("test %d failed (offset %d, length %d), wrong data returned", i, offset, length)
		}
	}
}
