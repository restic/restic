package rechunker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
	"testing"

	"github.com/restic/chunker"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

// Reference: walker_test.go, rewriter_test.go (v0.18.0)

// test repo that implements minimal Loader/Saver functions
type TestRechunkerRepo struct {
	loadBlob          func(id restic.ID, buf []byte) ([]byte, error)
	loadBlobsFromPack func(packID restic.ID, blobs []restic.Blob, handleBlobFn func(blob restic.BlobHandle, buf []byte, err error) error) error
	saveBlob          func(buf []byte) (newID restic.ID, known bool, size int, err error)
}

func (r *TestRechunkerRepo) LoadBlob(ctx context.Context, t restic.BlobType, id restic.ID, buf []byte) ([]byte, error) {
	return r.loadBlob(id, buf)
}
func (r *TestRechunkerRepo) LoadBlobsFromPack(ctx context.Context, packID restic.ID, blobs []restic.Blob, handleBlobFn func(blob restic.BlobHandle, buf []byte, err error) error) error {
	return r.loadBlobsFromPack(packID, blobs, handleBlobFn)
}
func (r *TestRechunkerRepo) SaveBlob(ctx context.Context, t restic.BlobType, buf []byte, id restic.ID, storeDuplicate bool) (newID restic.ID, known bool, size int, err error) {
	return r.saveBlob(buf)
}

// chunk `files` by `pol` and return fileIndex (map from path to blob IDs) and chunkStore (map from blob ID to bytes data)
func chunkFiles(chnker *chunker.Chunker, pol chunker.Pol, files map[string][]byte) (map[string]restic.IDs, map[restic.ID][]byte) {
	fileIndex := map[string]restic.IDs{}
	chunkStore := map[restic.ID][]byte{}

	for name, data := range files {
		r := bytes.NewReader(data)
		chnker.Reset(r, pol)
		chunks := restic.IDs{}

		for {
			chunk, err := chnker.Next(nil)
			if err == io.EOF {
				break
			}
			if err != nil {
				panic(err)
			}

			id := restic.Hash(chunk.Data)
			chunks = append(chunks, id)
			if _, ok := chunkStore[id]; !ok {
				chunkStore[id] = chunk.Data
			}
		}

		fileIndex[name] = chunks
	}

	return fileIndex, chunkStore
}

// arbitrary pack assignment for blobs in chunkStore
func simulatedPack(chunkStore map[restic.ID][]byte) map[restic.ID]restic.ID {
	blobToPack := map[restic.ID]restic.ID{}
	i := 0
	packID := restic.NewRandomID()
	for blobID := range chunkStore {
		blobToPack[blobID] = packID
		i++
		if i%10 == 0 {
			packID = restic.NewRandomID()
		}
	}

	return blobToPack
}

func TestRechunkerRechunkData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// generate reandom polynomials
	srcChunkerParam, _ := chunker.RandomPolynomial()
	dstChunkerParam, _ := chunker.RandomPolynomial()

	// prepare test data
	files := map[string][]byte{
		"0": {},
		"1": rtest.Random(1, 10_000),
		"2": rtest.Random(4, 20_000_000),
		"3": rtest.Random(5, 100_000_000),
	}
	files["2_duplicate"] = files["2"]
	prefixChanged := make([]byte, 0, 100_500_000)
	prefixChanged = append(prefixChanged, rtest.Random(6, 1_000_000)...)
	prefixChanged = append(prefixChanged, files["3"][500_000:]...)
	files["3_prefix_changed"] = prefixChanged
	suffixChanged := make([]byte, 0, 99_500_000)
	suffixChanged = append(suffixChanged, files["3"][:99_000_000]...)
	suffixChanged = append(suffixChanged, rtest.Random(7, 500_000)...)
	files["3_suffix_changed"] = suffixChanged

	// prepare chunker and minimal repositories
	chnker := chunker.New(nil, 0)
	srcFileIndex, srcChunkStore := chunkFiles(chnker, srcChunkerParam, files)
	dstWantsFileIndex, dstWantsChunkStore := chunkFiles(chnker, dstChunkerParam, files)
	rechunkStore1 := restic.IDSet{}
	rechunkStore2 := restic.IDSet{}

	srcFilesList := []restic.IDs{}
	for _, file := range srcFileIndex {
		srcFilesList = append(srcFilesList, file)
	}
	srcBlobToPack := simulatedPack(srcChunkStore)

	srcRepo := &TestRechunkerRepo{
		loadBlob: func(id restic.ID, buf []byte) ([]byte, error) {
			blob, ok := srcChunkStore[id]
			if !ok {
				return nil, fmt.Errorf("blob not found")
			}

			if cap(buf) < len(blob) {
				buf = make([]byte, len(blob))
			}
			buf = buf[:len(blob)]
			copy(buf, blob)

			return buf, nil
		},
		loadBlobsFromPack: func(packID restic.ID, blobs []restic.Blob, handleBlobFn func(blob restic.BlobHandle, buf []byte, err error) error) error {
			for _, blob := range blobs {
				if packID != srcBlobToPack[blob.ID] {
					return fmt.Errorf("blob %v is not in the pack %v", blob.ID, packID)
				}
				err := handleBlobFn(blob.BlobHandle, srcChunkStore[blob.ID], nil)
				if err != nil {
					return err
				}
			}
			return nil
		},
	}

	// test 1: rechunking without cache
	rechunker1 := NewRechunker(dstChunkerParam)
	rechunker1.reset()
	rechunker1.filesList = srcFilesList
	rtest.OK(t, rechunker1.buildIndex(false, func(t restic.BlobType, id restic.ID) []restic.PackedBlob {
		pb := restic.PackedBlob{}
		pb.ID = id
		pb.Type = t
		pb.UncompressedLength = uint(len(srcChunkStore[id]))
		pb.PackID = srcBlobToPack[id]

		return []restic.PackedBlob{pb}
	}))
	rechunker1.rechunkReady = true

	saveBlobLock1 := sync.Mutex{}
	rechunkTestRepo1 := &TestRechunkerRepo{
		saveBlob: func(buf []byte) (newID restic.ID, known bool, size int, err error) {
			newID = restic.Hash(buf)
			saveBlobLock1.Lock()
			rechunkStore1.Insert(newID)
			saveBlobLock1.Unlock()
			return
		},
	}
	rtest.OK(t, rechunker1.RechunkData(ctx, srcRepo, rechunkTestRepo1, nil))

	// test 2: rechunking with cache
	rechunker2 := NewRechunker(dstChunkerParam)
	rechunker2.reset()
	rechunker2.filesList = srcFilesList
	rtest.OK(t, rechunker2.buildIndex(true, func(t restic.BlobType, id restic.ID) []restic.PackedBlob {
		pb := restic.PackedBlob{}
		pb.ID = id
		pb.Type = t
		pb.UncompressedLength = uint(len(srcChunkStore[id]))
		pb.PackID = srcBlobToPack[id]

		return []restic.PackedBlob{pb}
	}))
	rechunker2.usePackCache = true
	rechunker2.rechunkReady = true

	saveBlobLock2 := sync.Mutex{}
	rechunkTestRepo2 := &TestRechunkerRepo{
		saveBlob: func(buf []byte) (newID restic.ID, known bool, size int, err error) {
			newID = restic.Hash(buf)
			saveBlobLock2.Lock()
			rechunkStore2.Insert(newID)
			saveBlobLock2.Unlock()
			return
		},
	}
	rtest.OK(t, rechunker2.RechunkData(ctx, srcRepo, rechunkTestRepo2, nil))

	// compare test result (by rechunker) vs dstWantsChunkedFiles (ordinary chunking)
	testResult1 := rechunker1.rechunkMap
	testResult2 := rechunker2.rechunkMap
	for name, srcBlobs := range srcFileIndex {
		hashval := hashOfIDs(srcBlobs)
		wants := hashOfIDs(dstWantsFileIndex[name])
		if hashOfIDs(testResult1[hashval]) != wants {
			t.Errorf("rechunk test 1 mismatch for file '%v'", name)
		}
		if hashOfIDs(testResult2[hashval]) != wants {
			t.Errorf("rechunk test 2 mismatch for file '%v'", name)
		}
	}

	// check if all blobs are stored
	for blobID := range dstWantsChunkStore {
		if !rechunkStore1.Has(blobID) {
			t.Errorf("blob missing for test 1: %v", blobID.Str())
		}
		if !rechunkStore2.Has(blobID) {
			t.Errorf("blob missing for test 2: %v", blobID.Str())
		}
	}
}

type BlobIDsPair struct {
	srcBlobIDs restic.IDs
	dstBlobIDs restic.IDs
}

func generateBlobIDsPair(nSrc, nDst uint) BlobIDsPair {
	srcIDs := make(restic.IDs, 0, nSrc)
	dstIDs := make(restic.IDs, 0, nDst)
	for range nSrc {
		srcIDs = append(srcIDs, restic.NewRandomID())
	}
	for range nDst {
		dstIDs = append(dstIDs, restic.NewRandomID())
	}

	return BlobIDsPair{srcBlobIDs: srcIDs, dstBlobIDs: dstIDs}
}

type TreeMap map[restic.ID][]byte
type TestTree map[string]interface{}
type TestContentNode struct {
	Type    data.NodeType
	Size    uint64
	Content restic.IDs
}

func (t TreeMap) LoadBlob(_ context.Context, _ restic.BlobType, id restic.ID, _ []byte) ([]byte, error) {
	buf, ok := t[id]
	if !ok {
		return nil, fmt.Errorf("blob does not exist")
	}
	return buf, nil
}

func (t TreeMap) SaveBlob(_ context.Context, _ restic.BlobType, buf []byte, _ restic.ID, _ bool) (newID restic.ID, known bool, size int, err error) {
	id := restic.Hash(buf)

	_, ok := t[id]
	if ok {
		return id, false, 0, nil
	}

	t[id] = append([]byte{}, buf...)
	return id, true, len(buf), nil
}

func BuildTreeMap(tree TestTree) (m TreeMap, root restic.ID) {
	m = TreeMap{}
	id := buildTreeMap(tree, m)
	return m, id
}

func buildTreeMap(tree TestTree, m TreeMap) restic.ID {
	tb := data.NewTreeJSONBuilder()
	var names []string
	for name := range tree {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		item := tree[name]
		switch elem := item.(type) {
		case TestTree:
			id := buildTreeMap(elem, m)
			err := tb.AddNode(&data.Node{
				Name:    name,
				Subtree: &id,
				Type:    data.NodeTypeDir,
			})
			if err != nil {
				panic(err)
			}
		case TestContentNode:
			err := tb.AddNode(&data.Node{
				Name:    name,
				Type:    elem.Type,
				Size:    elem.Size,
				Content: elem.Content,
			})
			if err != nil {
				panic(err)
			}
		default:
			panic(fmt.Sprintf("invalid type %T", elem))
		}
	}

	buf, err := tb.Finalize()
	if err != nil {
		panic(err)
	}

	id := restic.Hash(buf)

	if _, ok := m[id]; !ok {
		m[id] = buf
	}

	return id
}

func TestRechunkerRewriteTree(t *testing.T) {
	blobIDsMap := map[string]BlobIDsPair{
		"a":        generateBlobIDsPair(1, 1),
		"subdir/a": generateBlobIDsPair(30, 31),
		"x":        generateBlobIDsPair(42, 41),
		"0":        generateBlobIDsPair(0, 0),
	}
	rechunkBlobsMap := map[hashType]restic.IDs{}
	for _, v := range blobIDsMap {
		rechunkBlobsMap[hashOfIDs(v.srcBlobIDs)] = v.dstBlobIDs
	}

	tree := TestTree{
		"zerofile": TestContentNode{
			Type:    data.NodeTypeFile,
			Size:    0,
			Content: restic.IDs{},
		},
		"a": TestContentNode{
			Type:    data.NodeTypeFile,
			Size:    1,
			Content: blobIDsMap["a"].srcBlobIDs,
		},
		"subdir": TestTree{
			"a": TestContentNode{
				Type:    data.NodeTypeFile,
				Size:    3,
				Content: blobIDsMap["subdir/a"].srcBlobIDs,
			},
			"x": TestContentNode{
				Type:    data.NodeTypeFile,
				Size:    2,
				Content: blobIDsMap["x"].srcBlobIDs,
			},
			"subdir": TestTree{
				"dup_x": TestContentNode{
					Type:    data.NodeTypeFile,
					Size:    2,
					Content: blobIDsMap["x"].srcBlobIDs,
				},
				"nonregularfile": TestContentNode{
					Type: data.NodeTypeSymlink,
				},
			},
		},
	}
	wants := TestTree{
		"zerofile": TestContentNode{
			Type:    data.NodeTypeFile,
			Size:    0,
			Content: restic.IDs{},
		},
		"a": TestContentNode{
			Type:    data.NodeTypeFile,
			Size:    1,
			Content: blobIDsMap["a"].dstBlobIDs,
		},
		"subdir": TestTree{
			"a": TestContentNode{
				Type:    data.NodeTypeFile,
				Size:    3,
				Content: blobIDsMap["subdir/a"].dstBlobIDs,
			},
			"x": TestContentNode{
				Type:    data.NodeTypeFile,
				Size:    2,
				Content: blobIDsMap["x"].dstBlobIDs,
			},
			"subdir": TestTree{
				"dup_x": TestContentNode{
					Type:    data.NodeTypeFile,
					Size:    2,
					Content: blobIDsMap["x"].dstBlobIDs,
				},
				"nonregularfile": TestContentNode{
					Type: data.NodeTypeSymlink,
				},
			},
		},
	}

	srcRepo, srcRoot := BuildTreeMap(tree)
	_, wantsRoot := BuildTreeMap(wants)

	testsRepo := TreeMap{}
	rechunker := NewRechunker(0)
	rechunker.reset()
	rechunker.rechunkMap = rechunkBlobsMap
	testsRoot, err := rechunker.RewriteTree(context.TODO(), srcRepo, testsRepo, srcRoot)
	if err != nil {
		t.Error(err)
	}
	if wantsRoot != testsRoot {
		t.Errorf("tree mismatch. wants: %v, tests: %v", wantsRoot, testsRoot)
	}
}
