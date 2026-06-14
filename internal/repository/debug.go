//go:build debug

package repository

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
	"golang.org/x/sync/errgroup"

	"github.com/restic/restic/internal/repository/crypto"
	"github.com/restic/restic/internal/repository/index"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
)

type packDumpEntry struct {
	Name  string         `json:"name"`
	Blobs []packDumpBlob `json:"blobs"`
}

type packDumpBlob struct {
	Type   restic.BlobType `json:"type"`
	Length uint            `json:"length"`
	ID     restic.ID       `json:"id"`
	Offset uint            `json:"offset"`
}

func writePackDumpJSON(wr io.Writer, item any) error {
	buf, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	_, err = wr.Write(append(buf, '\n'))
	return err
}

// DumpPacks lists each pack file and writes its header blob layout as JSON to wr.
func DumpPacks(ctx context.Context, repo *Repository, wr io.Writer, printer restic.Printer) error {
	var m sync.Mutex
	return restic.ParallelList(ctx, repo, restic.PackFile, repo.Connections(), func(ctx context.Context, id restic.ID, size int64) error {
		blobs, err := repo.listPack(ctx, id, size)
		if err != nil {
			printer.E("error for pack %v: %v", id.Str(), err)
			return nil
		}

		p := packDumpEntry{
			Name:  id.String(),
			Blobs: make([]packDumpBlob, len(blobs)),
		}
		for i, blob := range blobs {
			p.Blobs[i] = packDumpBlob{
				Type:   blob.Type,
				Length: blob.Length,
				ID:     blob.ID,
				Offset: blob.Offset,
			}
		}

		m.Lock()
		defer m.Unlock()
		return writePackDumpJSON(wr, p)
	})
}

// DumpIndexes loads each on-disk index file and writes its debug dump to wr.
func DumpIndexes(ctx context.Context, repo restic.ListerLoaderUnpacked, wr io.Writer, printer restic.Printer) error {
	return index.ForAllIndexes(ctx, repo, repo, func(id restic.ID, idx *index.Index, err error) error {
		printer.S("index_id: %v", id)
		if err != nil {
			return err
		}

		return idx.Dump(wr)
	})
}

// ExaminePackOptions configures debug examination of a pack file.
type ExaminePackOptions struct {
	TryRepair     bool
	RepairByte    bool
	ExtractPack   bool
	ReuploadBlobs bool
}

// ExaminePack loads and inspects a pack file and its index entries.
func ExaminePack(ctx context.Context, repo *Repository, id restic.ID, opts ExaminePackOptions, printer restic.Printer) error {
	printer.S("examine %v", id)

	buf, err := repo.LoadRaw(ctx, restic.PackFile, id)
	// also process damaged pack files
	if buf == nil {
		return err
	}
	printer.S("  file size is %v", len(buf))
	gotID := restic.Hash(buf)
	if !id.Equal(gotID) {
		printer.S("  wanted hash %v, got %v", id, gotID)
	} else {
		printer.S("  hash for file content matches")
	}

	printer.S("  ========================================")
	printer.S("  looking for info in the indexes")

	blobsLoaded := false
	// examine all data the indexes have for the pack file
	for b := range repo.listPacksFromIndex(ctx, restic.NewIDSet(id)) {
		if len(b.Blobs) == 0 {
			continue
		}

		checkPackSize(b.Blobs, len(buf), printer)

		err := loadBlobs(ctx, opts, repo, id, b.Blobs, printer)
		if err != nil {
			printer.E("error: %v", err)
		} else {
			blobsLoaded = true
		}
	}

	printer.S("  ========================================")
	printer.S("  inspect the pack itself")

	blobs, err := repo.listPack(ctx, id, int64(len(buf)))
	if err != nil {
		return fmt.Errorf("pack %v: %v", id.Str(), err)
	}
	checkPackSize(blobs, len(buf), printer)

	if !blobsLoaded {
		return loadBlobs(ctx, opts, repo, id, blobs, printer)
	}
	return nil
}

func checkPackSize(blobs pack.Blobs, fileSize int, printer restic.Printer) {
	// track current size and offset
	var size, offset uint64

	blobs.Sort()

	for _, pb := range blobs {
		printer.S("      %v blob %v, offset %-6d, raw length %-6d", pb.Type, pb.ID, pb.Offset, pb.Length)
		if offset != uint64(pb.Offset) {
			printer.S("      hole in file, want offset %v, got %v", offset, pb.Offset)
		}
		offset = uint64(pb.Offset + pb.Length)
		size += uint64(pb.Length)
	}
	size += uint64(pack.CalculateHeaderSize(blobs))

	if uint64(fileSize) != size {
		printer.S("      file sizes do not match: computed %v, file size is %v", size, fileSize)
	} else {
		printer.S("      file sizes match")
	}
}

func tryRepairWithBitflip(key *crypto.Key, input []byte, bytewise bool, printer restic.Printer) []byte {
	if bytewise {
		printer.S("        trying to repair blob by finding a broken byte")
	} else {
		printer.S("        trying to repair blob with single bit flip")
	}

	ch := make(chan int)
	var wg errgroup.Group
	done := make(chan struct{})
	var fixed []byte
	var found bool

	workers := runtime.GOMAXPROCS(0)
	printer.S("         spinning up %d worker functions", runtime.GOMAXPROCS(0))
	for i := 0; i < workers; i++ {
		wg.Go(func() error {
			// make a local copy of the buffer
			buf := make([]byte, len(input))
			copy(buf, input)

			testFlip := func(idx int, pattern byte) bool {
				// flip bits
				buf[idx] ^= pattern

				nonce, plaintext := buf[:key.NonceSize()], buf[key.NonceSize():]
				plaintext, err := key.Open(plaintext[:0], nonce, plaintext, nil)
				if err == nil {
					printer.S("")
					printer.S("        blob could be repaired by XORing byte %v with 0x%02x", idx, pattern)
					printer.S("        hash is %v", restic.Hash(plaintext))
					close(done)
					found = true
					fixed = plaintext
					return true
				}

				// flip bits back
				buf[idx] ^= pattern
				return false
			}

			for i := range ch {
				if bytewise {
					for j := 0; j < 255; j++ {
						if testFlip(i, byte(j)) {
							return nil
						}
					}
				} else {
					for j := 0; j < 7; j++ {
						// flip each bit once
						if testFlip(i, (1 << uint(j))) {
							return nil
						}
					}
				}
			}
			return nil
		})
	}

	wg.Go(func() error {
		defer close(ch)

		start := time.Now()
		info := time.Now()
		for i := range input {
			select {
			case ch <- i:
			case <-done:
				printer.S("     done after %v", time.Since(start))
				return nil
			}

			if time.Since(info) > time.Second {
				secs := time.Since(start).Seconds()
				gps := float64(i) / secs
				remaining := len(input) - i
				eta := time.Duration(float64(remaining)/gps) * time.Second

				printer.S("\r%d byte of %d done (%.2f%%), %.0f byte per second, ETA %v",
					i, len(input), float32(i)/float32(len(input))*100, gps, eta)
				info = time.Now()
			}
		}
		return nil
	})
	err := wg.Wait()
	if err != nil {
		panic("all go routines can only return nil")
	}

	if !found {
		printer.S("\n        blob could not be repaired")
	}
	return fixed
}

func decryptUnsigned(k *crypto.Key, buf []byte) []byte {
	// strip signature at the end
	l := len(buf)
	nonce, ct := buf[:16], buf[16:l-16]
	out := make([]byte, len(ct))

	c, err := aes.NewCipher(k.EncryptionKey[:])
	if err != nil {
		panic(fmt.Sprintf("unable to create cipher: %v", err))
	}
	e := cipher.NewCTR(c, nonce)
	e.XORKeyStream(out, ct)

	return out
}

func loadBlobs(ctx context.Context, opts ExaminePackOptions, repo *Repository, packID restic.ID, list pack.Blobs, printer restic.Printer) error {
	dec, err := zstd.NewReader(nil)
	if err != nil {
		panic(err)
	}

	packData, err := repo.LoadRaw(ctx, restic.PackFile, packID)
	// allow processing broken pack files
	if packData == nil {
		return err
	}

	err = repo.WithBlobUploader(ctx, func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		for _, blob := range list {
			printer.S("      loading blob %v at %v (length %v)", blob.ID, blob.Offset, blob.Length)
			if int(blob.Offset+blob.Length) > len(packData) {
				printer.E("skipping truncated blob")
				continue
			}
			buf := packData[blob.Offset : blob.Offset+blob.Length]
			key := repo.Key()

			nonce, plaintext := buf[:key.NonceSize()], buf[key.NonceSize():]
			plaintext, err = key.Open(plaintext[:0], nonce, plaintext, nil)
			outputPrefix := ""
			filePrefix := ""
			if err != nil {
				printer.E("error decrypting blob: %v", err)
				if opts.TryRepair || opts.RepairByte {
					plaintext = tryRepairWithBitflip(key, buf, opts.RepairByte, printer)
				}
				if plaintext != nil {
					outputPrefix = "repaired "
					filePrefix = "repaired-"
				} else {
					plaintext = decryptUnsigned(key, buf)
					err = storePlainBlob(blob.ID, "damaged-", plaintext, printer)
					if err != nil {
						return err
					}
					continue
				}
			}

			if blob.IsCompressed() {
				decompressed, err := dec.DecodeAll(plaintext, nil)
				if err != nil {
					printer.S("         failed to decompress blob %v", blob.ID)
				}
				if decompressed != nil {
					plaintext = decompressed
				}
			}

			id := restic.Hash(plaintext)
			var prefix string
			if !id.Equal(blob.ID) {
				printer.S("         successfully %vdecrypted blob (length %v), hash is %v, ID does not match, wanted %v", outputPrefix, len(plaintext), id, blob.ID)
				prefix = "wrong-hash-"
			} else {
				printer.S("         successfully %vdecrypted blob (length %v), hash is %v, ID matches", outputPrefix, len(plaintext), id)
				prefix = "correct-"
			}
			if opts.ExtractPack {
				err = storePlainBlob(id, filePrefix+prefix, plaintext, printer)
				if err != nil {
					return err
				}
			}
			if opts.ReuploadBlobs {
				_, _, _, err := uploader.SaveBlob(ctx, blob.Type, plaintext, id, true)
				if err != nil {
					return err
				}
				printer.S("         uploaded %v %v", blob.Type, id)
			}
		}
		return nil
	})
	return err
}

func storePlainBlob(id restic.ID, prefix string, plain []byte, printer restic.Printer) error {
	filename := fmt.Sprintf("%s%s.bin", prefix, id)
	f, err := os.Create(filename)
	if err != nil {
		return err
	}

	_, err = f.Write(plain)
	if err != nil {
		_ = f.Close()
		return err
	}

	err = f.Close()
	if err != nil {
		return err
	}

	printer.S("decrypt of blob %v stored at %v", id, filename)
	return nil
}
