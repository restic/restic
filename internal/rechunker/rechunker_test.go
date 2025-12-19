package rechunker

import (
	"bytes"
	"context"
	"errors"
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

// TestRechunkerRepo implements minimal repository interface for rechunker test.
type TestRechunkerRepo struct {
	loadBlob          func(id restic.ID, buf []byte) ([]byte, error)
	loadBlobsFromPack func(packID restic.ID, blobs []restic.Blob, handleBlobFn func(blob restic.BlobHandle, buf []byte, err error) error) error
	saveBlob          func(buf []byte) (newID restic.ID, known bool, size int, err error)
}

// methods to satisfy interfaces used in rechunker

func (r *TestRechunkerRepo) LoadBlob(ctx context.Context, t restic.BlobType, id restic.ID, buf []byte) ([]byte, error) {
	return r.loadBlob(id, buf)
}
func (r *TestRechunkerRepo) LoadBlobsFromPack(ctx context.Context, packID restic.ID, blobs []restic.Blob, handleBlobFn func(blob restic.BlobHandle, buf []byte, err error) error) error {
	return r.loadBlobsFromPack(packID, blobs, handleBlobFn)
}
func (r *TestRechunkerRepo) SaveBlob(ctx context.Context, t restic.BlobType, buf []byte, id restic.ID, storeDuplicate bool) (newID restic.ID, known bool, size int, err error) {
	return r.saveBlob(buf)
}
func (r *TestRechunkerRepo) SaveBlobAsync(ctx context.Context, tpe restic.BlobType, buf []byte, id restic.ID, storeDuplicate bool, cb func(newID restic.ID, known bool, sizeInRepo int, err error)) {
	// not used in rechunker; declared just to satisfy restic.BlobSaverWithAsync interface
}
func (r *TestRechunkerRepo) WithBlobUploader(ctx context.Context, fn func(ctx context.Context, uploader restic.BlobSaverWithAsync) error) error {
	return fn(ctx, r)
}
func (r *TestRechunkerRepo) Connections() uint {
	// arbitrarily chosen value
	return 5
}

// chunkFiles chunk `files` by `pol` and return fileIndex (map from path to blob IDs) and chunkStore (map from blob ID to blob data).
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

// simulatedPack assigns arbitrary pack to each blob in chunkStore.
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

// prepareData prepares random data for rechunker test.
func prepareData() map[string][]byte {
	files := map[string][]byte{
		"0": {},
		"1": rtest.Random(1, 10_000),
		"2": rtest.Random(4, 10_000_000),
		"3": rtest.Random(5, 100_000_000),
	}

	return files
}

func TestRechunker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// generate reandom polynomials
	srcChunkerParam, _ := chunker.RandomPolynomial()
	dstChunkerParam, _ := chunker.RandomPolynomial()

	// prepare test data
	files := prepareData()

	// prepare chunker and minimal repositories
	chnker := chunker.New(nil, 0)
	srcFileIndex, srcChunkStore := chunkFiles(chnker, srcChunkerParam, files)
	dstWantsFileIndex, dstWantsChunkStore := chunkFiles(chnker, dstChunkerParam, files)
	rechunkStore := restic.IDSet{}

	// build files list and virtual blobToPack mapping
	srcFilesList := []*ChunkedFile{}
	for _, file := range srcFileIndex {
		srcFilesList = append(srcFilesList, &ChunkedFile{file, HashOfIDs(file)})
	}
	srcBlobToPack := simulatedPack(srcChunkStore)

	// define src repo for rechunker test
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

	// create rechunker
	cfg := Config{
		CacheSize:          4096 * (1 << 20),
		SmallFileThreshold: 25,
		Pol:                dstChunkerParam,
	}
	rechunker := NewRechunker(cfg)

	// manually configure rechunker instead of running Plan(), because we are using mock repo
	var err error
	rechunker.filesList = srcFilesList
	rechunker.idx, rechunker.tracker, err = createIndex(srcFilesList, func(t restic.BlobType, id restic.ID) []restic.PackedBlob {
		pb := restic.PackedBlob{}
		pb.ID = id
		pb.Type = t
		pb.UncompressedLength = uint(len(srcChunkStore[id]))
		pb.PackID = srcBlobToPack[id]

		return []restic.PackedBlob{pb}
	}, cfg)
	if err != nil {
		panic(err)
	}

	rechunker.rechunkReady = true

	// define dst repo for rechunker test, and run Rechunk
	saveBlobLock := sync.Mutex{}
	rechunkTestRepo := &TestRechunkerRepo{
		saveBlob: func(buf []byte) (newID restic.ID, known bool, size int, err error) {
			newID = restic.Hash(buf)
			saveBlobLock.Lock()
			rechunkStore.Insert(newID)
			saveBlobLock.Unlock()
			return
		},
	}
	rtest.OK(t, rechunker.Rechunk(ctx, srcRepo, rechunkTestRepo, nil))

	// compare test result (by rechunker) vs dstWantsChunkedFiles (ordinary backup)
	testResult := rechunker.rechunkMap
	for name, srcBlobs := range srcFileIndex {
		hashval := HashOfIDs(srcBlobs)
		wants := HashOfIDs(dstWantsFileIndex[name])
		if HashOfIDs(testResult[hashval]) != wants {
			t.Errorf("blob mismatch for file '%v'", name)
		}
	}

	// check if all blobs are stored
	for blobID := range dstWantsChunkStore {
		if !rechunkStore.Has(blobID) {
			t.Errorf("blob missing: %v", blobID.Str())
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

// Type definitions for rewriteTree test.
// Reference: walker/rewriter_test.go and walker/walker_test.go (v0.18.0).

type TreeMap map[restic.ID][]byte
type TestTree map[string]interface{}
type TestContentNode struct {
	Type    data.NodeType
	Size    uint64
	Content restic.IDs
}

func (t TreeMap) LoadBlob(_ context.Context, tpe restic.BlobType, id restic.ID, _ []byte) ([]byte, error) {
	if tpe != restic.TreeBlob {
		return nil, errors.New("can only load trees")
	}
	tree, ok := t[id]
	if !ok {
		return nil, errors.New("tree not found")
	}
	return tree, nil
}

func (t TreeMap) SaveBlob(_ context.Context, tpe restic.BlobType, buf []byte, id restic.ID, _ bool) (newID restic.ID, known bool, size int, err error) {
	if tpe != restic.TreeBlob {
		return restic.ID{}, false, 0, errors.New("can only save trees")
	}

	if id.IsNull() {
		id = restic.Hash(buf)
	}
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

// prepareTree prepares sample tree for rewriteTree test.
func prepareTree() (srcTree TestTree, wantsTree TestTree, rechunkMap map[restic.ID]restic.IDs) {
	blobIDsMap := map[string]BlobIDsPair{
		"a":        generateBlobIDsPair(1, 1),
		"subdir/a": generateBlobIDsPair(30, 31),
		"x":        generateBlobIDsPair(42, 41),
		"0":        generateBlobIDsPair(0, 0),
	}
	rechunkMap = map[restic.ID]restic.IDs{}
	for _, v := range blobIDsMap {
		rechunkMap[HashOfIDs(v.srcBlobIDs)] = v.dstBlobIDs
	}

	srcTree = TestTree{
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
	wantsTree = TestTree{
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

	return srcTree, wantsTree, rechunkMap
}

func TestRechunkerRewriteTree(t *testing.T) {
	srcTree, wantsTree, rechunkMap := prepareTree()
	
	srcRepo, srcRoot := BuildTreeMap(srcTree)
	_, wantsRoot := BuildTreeMap(wantsTree)

	testsRepo := TreeMap{}
	rechunker := NewRechunker(Config{})
	rechunker.rechunkMap = rechunkMap
	testsRoot, err := rechunker.RewriteTree(context.TODO(), srcRepo, testsRepo, srcRoot)
	if err != nil {
		t.Error(err)
	}
	if wantsRoot != testsRoot {
		t.Errorf("tree mismatch. wants: %v, tests: %v", wantsRoot, testsRoot)
	}
}
