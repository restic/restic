package archiver_test

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/checker"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"

	"github.com/restic/restic/internal/errors"

	"github.com/restic/chunker"
)

var testPol = chunker.Pol(0x3DA3358B4DC173)

type Rdr interface {
	io.ReadSeeker
	io.ReaderAt
}

func benchmarkChunkEncrypt(b testing.TB, buf, buf2 []byte, rd Rdr, key *crypto.Key) {
	rd.Seek(0, 0)
	ch := chunker.New(rd, testPol)
	nonce := crypto.NewRandomNonce()

	for {
		chunk, err := ch.Next(buf)

		if errors.Cause(err) == io.EOF {
			break
		}

		rtest.OK(b, err)

		rtest.Assert(b, uint(len(chunk.Data)) == chunk.Length,
			"invalid length: got %d, expected %d", len(chunk.Data), chunk.Length)

		_ = key.Seal(buf2[:0], nonce, chunk.Data, nil)
	}
}

func BenchmarkChunkEncrypt(b *testing.B) {
	repo, cleanup := repository.TestRepository(b)
	defer cleanup()

	data := rtest.Random(23, 10<<20) // 10MiB
	rd := bytes.NewReader(data)

	buf := make([]byte, chunker.MaxSize)
	buf2 := make([]byte, chunker.MaxSize)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		benchmarkChunkEncrypt(b, buf, buf2, rd, repo.Key())
	}
}

func benchmarkChunkEncryptP(b *testing.PB, buf []byte, rd Rdr, key *crypto.Key) {
	ch := chunker.New(rd, testPol)
	nonce := crypto.NewRandomNonce()

	for {
		chunk, err := ch.Next(buf)
		if errors.Cause(err) == io.EOF {
			break
		}

		_ = key.Seal(chunk.Data[:0], nonce, chunk.Data, nil)
	}
}

func BenchmarkChunkEncryptParallel(b *testing.B) {
	repo, cleanup := repository.TestRepository(b)
	defer cleanup()

	data := rtest.Random(23, 10<<20) // 10MiB

	buf := make([]byte, chunker.MaxSize)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rd := bytes.NewReader(data)
			benchmarkChunkEncryptP(pb, buf, rd, repo.Key())
		}
	})
}

func archiveDirectory(b testing.TB) {
	repo, cleanup := repository.TestRepository(b)
	defer cleanup()

	arch := archiver.New(repo)

	_, id, err := arch.Snapshot(context.TODO(), nil, []string{rtest.BenchArchiveDirectory}, nil, "localhost", nil, time.Now())
	rtest.OK(b, err)

	b.Logf("snapshot archived as %v", id)
}

func TestArchiveDirectory(t *testing.T) {
	if rtest.BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestArchiveDirectory")
	}

	archiveDirectory(t)
}

func BenchmarkArchiveDirectory(b *testing.B) {
	if rtest.BenchArchiveDirectory == "" {
		b.Skip("benchdir not set, skipping BenchmarkArchiveDirectory")
	}

	for i := 0; i < b.N; i++ {
		archiveDirectory(b)
	}
}

func countPacks(t testing.TB, repo restic.Repository, tpe restic.FileType) (n uint) {
	err := repo.Backend().List(context.TODO(), tpe, func(restic.FileInfo) error {
		n++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return n
}

func archiveWithDedup(t testing.TB) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	if rtest.BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestArchiverDedup")
	}

	var cnt struct {
		before, after, after2 struct {
			packs, dataBlobs, treeBlobs uint
		}
	}

	// archive a few files
	sn := archiver.TestSnapshot(t, repo, rtest.BenchArchiveDirectory, nil)
	t.Logf("archived snapshot %v", sn.ID().Str())

	// get archive stats
	cnt.before.packs = countPacks(t, repo, restic.DataFile)
	cnt.before.dataBlobs = repo.Index().Count(restic.DataBlob)
	cnt.before.treeBlobs = repo.Index().Count(restic.TreeBlob)
	t.Logf("packs %v, data blobs %v, tree blobs %v",
		cnt.before.packs, cnt.before.dataBlobs, cnt.before.treeBlobs)

	// archive the same files again, without parent snapshot
	sn2 := archiver.TestSnapshot(t, repo, rtest.BenchArchiveDirectory, nil)
	t.Logf("archived snapshot %v", sn2.ID().Str())

	// get archive stats again
	cnt.after.packs = countPacks(t, repo, restic.DataFile)
	cnt.after.dataBlobs = repo.Index().Count(restic.DataBlob)
	cnt.after.treeBlobs = repo.Index().Count(restic.TreeBlob)
	t.Logf("packs %v, data blobs %v, tree blobs %v",
		cnt.after.packs, cnt.after.dataBlobs, cnt.after.treeBlobs)

	// if there are more data blobs, something is wrong
	if cnt.after.dataBlobs > cnt.before.dataBlobs {
		t.Fatalf("TestArchiverDedup: too many data blobs in repository: before %d, after %d",
			cnt.before.dataBlobs, cnt.after.dataBlobs)
	}

	// archive the same files again, with a parent snapshot
	sn3 := archiver.TestSnapshot(t, repo, rtest.BenchArchiveDirectory, sn2.ID())
	t.Logf("archived snapshot %v, parent %v", sn3.ID().Str(), sn2.ID().Str())

	// get archive stats again
	cnt.after2.packs = countPacks(t, repo, restic.DataFile)
	cnt.after2.dataBlobs = repo.Index().Count(restic.DataBlob)
	cnt.after2.treeBlobs = repo.Index().Count(restic.TreeBlob)
	t.Logf("packs %v, data blobs %v, tree blobs %v",
		cnt.after2.packs, cnt.after2.dataBlobs, cnt.after2.treeBlobs)

	// if there are more data blobs, something is wrong
	if cnt.after2.dataBlobs > cnt.before.dataBlobs {
		t.Fatalf("TestArchiverDedup: too many data blobs in repository: before %d, after %d",
			cnt.before.dataBlobs, cnt.after2.dataBlobs)
	}
}

func TestArchiveDedup(t *testing.T) {
	archiveWithDedup(t)
}

// Saves several identical chunks concurrently and later checks that there are no
// unreferenced packs in the repository. See also #292 and #358.
func TestParallelSaveWithDuplication(t *testing.T) {
	for seed := 0; seed < 10; seed++ {
		testParallelSaveWithDuplication(t, seed)
	}
}

func testParallelSaveWithDuplication(t *testing.T, seed int) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	dataSizeMb := 128
	duplication := 7

	arch := archiver.New(repo)
	chunks := getRandomData(seed, dataSizeMb*1024*1024)

	errChannels := [](<-chan error){}

	// interwoven processing of subsequent chunks
	maxParallel := 2*duplication - 1
	barrier := make(chan struct{}, maxParallel)

	for _, c := range chunks {
		for dupIdx := 0; dupIdx < duplication; dupIdx++ {
			errChan := make(chan error)
			errChannels = append(errChannels, errChan)

			go func(c chunker.Chunk, errChan chan<- error) {
				barrier <- struct{}{}

				id := restic.Hash(c.Data)
				time.Sleep(time.Duration(id[0]))
				err := arch.Save(context.TODO(), restic.DataBlob, c.Data, id)
				<-barrier
				errChan <- err
			}(c, errChan)
		}
	}

	for _, errChan := range errChannels {
		rtest.OK(t, <-errChan)
	}

	rtest.OK(t, repo.Flush(context.Background()))
	rtest.OK(t, repo.SaveIndex(context.TODO()))

	chkr := createAndInitChecker(t, repo)
	assertNoUnreferencedPacks(t, chkr)
}

func getRandomData(seed int, size int) []chunker.Chunk {
	buf := rtest.Random(seed, size)
	var chunks []chunker.Chunk
	chunker := chunker.New(bytes.NewReader(buf), testPol)

	for {
		c, err := chunker.Next(nil)
		if errors.Cause(err) == io.EOF {
			break
		}
		chunks = append(chunks, c)
	}

	return chunks
}

func createAndInitChecker(t *testing.T, repo restic.Repository) *checker.Checker {
	chkr := checker.New(repo)

	hints, errs := chkr.LoadIndex(context.TODO())
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %v: %v", len(errs), errs)
	}

	if len(hints) > 0 {
		t.Errorf("expected no hints, got %v: %v", len(hints), hints)
	}

	return chkr
}

func assertNoUnreferencedPacks(t *testing.T, chkr *checker.Checker) {
	errChan := make(chan error)
	go chkr.Packs(context.TODO(), errChan)

	for err := range errChan {
		rtest.OK(t, err)
	}
}

func TestArchiveEmptySnapshot(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	arch := archiver.New(repo)

	sn, id, err := arch.Snapshot(context.TODO(), nil, []string{"file-does-not-exist-123123213123", "file2-does-not-exist-too-123123123"}, nil, "localhost", nil, time.Now())
	if err == nil {
		t.Errorf("expected error for empty snapshot, got nil")
	}

	if !id.IsNull() {
		t.Errorf("expected null ID for empty snapshot, got %v", id.Str())
	}

	if sn != nil {
		t.Errorf("expected null snapshot for empty snapshot, got %v", sn)
	}
}

func chdir(t testing.TB, target string) (cleanup func()) {
	curdir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("chdir to %v", target)
	err = os.Chdir(target)
	if err != nil {
		t.Fatal(err)
	}

	return func() {
		t.Logf("chdir back to %v", curdir)
		err := os.Chdir(curdir)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestArchiveNameCollision(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	dir, cleanup := rtest.TempDir(t)
	defer cleanup()

	root := filepath.Join(dir, "root")
	rtest.OK(t, os.MkdirAll(root, 0755))

	rtest.OK(t, ioutil.WriteFile(filepath.Join(dir, "testfile"), []byte("testfile1"), 0644))
	rtest.OK(t, ioutil.WriteFile(filepath.Join(dir, "root", "testfile"), []byte("testfile2"), 0644))

	defer chdir(t, root)()

	arch := archiver.New(repo)

	sn, id, err := arch.Snapshot(context.TODO(), nil, []string{"testfile", filepath.Join("..", "testfile")}, nil, "localhost", nil, time.Now())
	rtest.OK(t, err)

	t.Logf("snapshot archived as %v", id)

	tree, err := repo.LoadTree(context.TODO(), *sn.Tree)
	rtest.OK(t, err)

	if len(tree.Nodes) != 2 {
		t.Fatalf("tree has %d nodes, wanted 2: %v", len(tree.Nodes), tree.Nodes)
	}
}
