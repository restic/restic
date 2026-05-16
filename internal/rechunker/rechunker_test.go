package rechunker

import (
	"context"
	"fmt"
	"testing"

	"github.com/restic/chunker"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/walker"
)

// prepareData prepares random data for rechunker test.
func prepareData(t *testing.T) string {
	tempdir := rtest.TempDir(t)
	data := map[int][]byte{
		1: rtest.Random(1, 10_000),
		2: rtest.Random(2, 10_000_000),
		3: rtest.Random(3, 100_000_000),
	}
	repo := archiver.TestDir{
		"zero":  archiver.TestFile{Content: ""},
		"one":   archiver.TestFile{Content: string(data[1])},
		"two":   archiver.TestFile{Content: string(data[2])},
		"three": archiver.TestFile{Content: string(data[3])},
		"dir1": archiver.TestDir{
			"dir2": archiver.TestDir{
				"dup_1": archiver.TestFile{Content: string(data[1])},
				"dup_3": archiver.TestFile{Content: string(data[3])},
			},
		},
	}
	archiver.TestCreateFiles(t, tempdir, repo)

	return tempdir
}

func gatherNodesByPath(t *testing.T, repo restic.BlobLoader, root restic.ID) map[string]*data.Node {
	t.Helper()

	result := map[string]*data.Node{}
	err := walker.Walk(t.Context(), repo, root, walker.WalkVisitor{
		ProcessNode: func(parentTreeID restic.ID, path string, node *data.Node, nodeErr error) (err error) {
			if node != nil {
				result[path] = node
			}
			return nodeErr
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	return result
}

func buildRechunkMapByMatchingPath(t *testing.T, srcNodes, dstNodes map[string]*data.Node) map[restic.ID]restic.IDs {
	t.Helper()

	rechunkMap := map[restic.ID]restic.IDs{}

	for k, v := range srcNodes {
		if v.Type != data.NodeTypeFile {
			continue
		}
		if _, ok := dstNodes[k]; !ok {
			t.Fatalf("%v expected in dstNodes, but not found", k)
		}
		rechunkMap[HashOfIDs(v.Content)] = dstNodes[k].Content
	}

	return rechunkMap
}

func TestRechunker(t *testing.T) {
	// generate reandom polynomials
	srcChunkerParam, _ := chunker.RandomPolynomial()
	dstChunkerParam, _ := chunker.RandomPolynomial()

	// prepare test data
	tempdir := prepareData(t)

	// prepare repositories
	srcRepo := TestRepositoryWithPol(t, srcChunkerParam)
	dstWantsRepo := TestRepositoryWithPol(t, dstChunkerParam)
	dstTestsRepo := TestRepositoryWithPol(t, dstChunkerParam)

	srcSn := archiver.TestSnapshot(t, srcRepo, tempdir, nil)
	dstWantsSn := archiver.TestSnapshot(t, dstWantsRepo, tempdir, nil)

	srcNodes := gatherNodesByPath(t, srcRepo, *srcSn.Tree)
	dstWantsNodes := gatherNodesByPath(t, dstWantsRepo, *dstWantsSn.Tree)
	wantedRechunkMap := buildRechunkMapByMatchingPath(t, srcNodes, dstWantsNodes)

	// run rechunk copy
	rechunker := NewRechunker(Config{
		CacheSize: 4 * (1 << 30),
		Pol:       dstChunkerParam,
	})

	t.Run("Plan running", func(t *testing.T) {
		err := rechunker.Plan(t.Context(), srcRepo, restic.IDs{*srcSn.Tree})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Rechunk running", func(t *testing.T) {
		err := rechunker.Rechunk(t.Context(), srcRepo, dstTestsRepo, nil)
		if err != nil {
			t.Fatal(err)
		}
	})

	var testsTree restic.ID
	t.Run("RewriteTrees running", func(t *testing.T) {
		newID, err := rechunker.RewriteTrees(t.Context(), srcRepo, dstTestsRepo, restic.IDs{*srcSn.Tree})
		if err != nil {
			t.Fatal(err)
		}
		testsTree = newID[0]
	})

	// compare dstTestsRepo (rechunker result) vs dstWantsRepo (reference result)
	// 1) check if all expected data blobs are stored
	t.Run("data blob verification", func(t *testing.T) {
		inCtx, stop := context.WithCancelCause(t.Context())
		err := dstWantsRepo.ListBlobs(inCtx, func(pb restic.PackedBlob) {
			if pb.Type == restic.DataBlob {
				_, found := dstTestsRepo.LookupBlobSize(restic.DataBlob, pb.ID)
				if !found {
					stop(fmt.Errorf("blob %v expected but not found", pb.ID.Str()))
				}
			}
		})
		if err != nil {
			t.Error(err)
		}
	})

	// 2) check if rechunk is done correctly by comparing rechunkMap
	t.Run("rechunk mapping verification", func(t *testing.T) {
		testedRechunkMap := rechunker.rechunkMap
		for k, v := range wantedRechunkMap {
			wants := HashOfIDs(v)
			tests := HashOfIDs(testedRechunkMap[k])
			if wants != tests {
				t.Errorf("rechunk result for src file %v does not match: %v expected, but got %v", k.Str(), wants.Str(), tests.Str())
			}
		}
	})

	// 3) check if tree is rewritten correctly by comparing tree nodes
	t.Run("tree verification", func(t *testing.T) {
		testsNodes := gatherNodesByPath(t, dstTestsRepo, testsTree)

		// (i) compare Content field with dstWantsNodes
		for path, node := range dstWantsNodes {
			if node.Type != data.NodeTypeFile {
				continue
			}
			if _, ok := testsNodes[path]; !ok {
				t.Errorf("node for path %v does not exist", path)
				continue
			}
			wants := HashOfIDs(node.Content)
			tests := HashOfIDs(testsNodes[path].Content)
			if wants != tests {
				t.Errorf("node content for path %v does not match: %v expected, but got %v", path, wants.Str(), tests.Str())
			}
		}

		// (ii) compare remaining fields with srcNodes
		for path, wantsNode := range srcNodes {
			testsNode, ok := testsNodes[path]
			if !ok {
				t.Errorf("node for path %v does not exist", path)
				continue
			}
			// copy nodes and clear rewritten fields for comparison
			wants, tests := *wantsNode, *testsNode
			wants.Content, tests.Content = nil, nil
			wants.Subtree, tests.Subtree = nil, nil
			if !wants.Equals(tests) {
				t.Errorf("node fields for path %v does not match", path)
			}
		}
	})
}
