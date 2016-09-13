package archiver_test

import (
	"bytes"
	"io"
	"testing"
	"time"

	"restic"
	"restic/archiver"
	"restic/checker"
	"restic/crypto"
	"restic/repository"
	. "restic/test"

	"restic/errors"

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

	for {
		chunk, err := ch.Next(buf)

		if errors.Cause(err) == io.EOF {
			break
		}

		OK(b, err)

		// reduce length of buf
		Assert(b, uint(len(chunk.Data)) == chunk.Length,
			"invalid length: got %d, expected %d", len(chunk.Data), chunk.Length)

		_, err = crypto.Encrypt(key, buf2, chunk.Data)
		OK(b, err)
	}
}

func BenchmarkChunkEncrypt(b *testing.B) {
	repo, cleanup := repository.TestRepository(b)
	defer cleanup()

	data := Random(23, 10<<20) // 10MiB
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

	for {
		chunk, err := ch.Next(buf)
		if errors.Cause(err) == io.EOF {
			break
		}

		// reduce length of chunkBuf
		crypto.Encrypt(key, chunk.Data, chunk.Data)
	}
}

func BenchmarkChunkEncryptParallel(b *testing.B) {
	repo, cleanup := repository.TestRepository(b)
	defer cleanup()

	data := Random(23, 10<<20) // 10MiB

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

	_, id, err := arch.Snapshot(nil, []string{BenchArchiveDirectory}, nil, nil)
	OK(b, err)

	b.Logf("snapshot archived as %v", id)
}

func TestArchiveDirectory(t *testing.T) {
	if BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestArchiveDirectory")
	}

	archiveDirectory(t)
}

func BenchmarkArchiveDirectory(b *testing.B) {
	if BenchArchiveDirectory == "" {
		b.Skip("benchdir not set, skipping BenchmarkArchiveDirectory")
	}

	for i := 0; i < b.N; i++ {
		archiveDirectory(b)
	}
}

func countPacks(repo restic.Repository, t restic.FileType) (n uint) {
	for _ = range repo.Backend().List(t, nil) {
		n++
	}

	return n
}

func archiveWithDedup(t testing.TB) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	if BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestArchiverDedup")
	}

	var cnt struct {
		before, after, after2 struct {
			packs, dataBlobs, treeBlobs uint
		}
	}

	// archive a few files
	sn := archiver.TestSnapshot(t, repo, BenchArchiveDirectory, nil)
	t.Logf("archived snapshot %v", sn.ID().Str())

	// get archive stats
	cnt.before.packs = countPacks(repo, restic.DataFile)
	cnt.before.dataBlobs = repo.Index().Count(restic.DataBlob)
	cnt.before.treeBlobs = repo.Index().Count(restic.TreeBlob)
	t.Logf("packs %v, data blobs %v, tree blobs %v",
		cnt.before.packs, cnt.before.dataBlobs, cnt.before.treeBlobs)

	// archive the same files again, without parent snapshot
	sn2 := archiver.TestSnapshot(t, repo, BenchArchiveDirectory, nil)
	t.Logf("archived snapshot %v", sn2.ID().Str())

	// get archive stats again
	cnt.after.packs = countPacks(repo, restic.DataFile)
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
	sn3 := archiver.TestSnapshot(t, repo, BenchArchiveDirectory, sn2.ID())
	t.Logf("archived snapshot %v, parent %v", sn3.ID().Str(), sn2.ID().Str())

	// get archive stats again
	cnt.after2.packs = countPacks(repo, restic.DataFile)
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

	// interweaved processing of subsequent chunks
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
				err := arch.Save(restic.DataBlob, c.Data, id)
				<-barrier
				errChan <- err
			}(c, errChan)
		}
	}

	for _, errChan := range errChannels {
		OK(t, <-errChan)
	}

	OK(t, repo.Flush())
	OK(t, repo.SaveIndex())

	chkr := createAndInitChecker(t, repo)
	assertNoUnreferencedPacks(t, chkr)
}

func getRandomData(seed int, size int) []chunker.Chunk {
	buf := Random(seed, size)
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

	hints, errs := chkr.LoadIndex()
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %v: %v", len(errs), errs)
	}

	if len(hints) > 0 {
		t.Errorf("expected no hints, got %v: %v", len(hints), hints)
	}

	return chkr
}

func assertNoUnreferencedPacks(t *testing.T, chkr *checker.Checker) {
	done := make(chan struct{})
	defer close(done)

	errChan := make(chan error)
	go chkr.Packs(errChan, done)

	for err := range errChan {
		OK(t, err)
	}
}
