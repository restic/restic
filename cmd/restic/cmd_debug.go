// +build debug

package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/pack"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

var cmdDebug = &cobra.Command{
	Use:   "debug",
	Short: "Debug commands",
}

var cmdDebugDump = &cobra.Command{
	Use:   "dump [indexes|snapshots|all|packs]",
	Short: "Dump data structures",
	Long: `
The "dump" command dumps data structures from the repository as JSON objects. It
is used for debugging purposes only.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDebugDump(globalOptions, args)
	},
}

var tryRepair bool
var repairByte bool
var extractPack bool

func init() {
	cmdRoot.AddCommand(cmdDebug)
	cmdDebug.AddCommand(cmdDebugDump)
	cmdDebug.AddCommand(cmdDebugExamine)
	cmdDebugExamine.Flags().BoolVar(&extractPack, "extract-pack", false, "write blobs to the current directory")
	cmdDebugExamine.Flags().BoolVar(&tryRepair, "try-repair", false, "try to repair broken blobs with single bit flips")
	cmdDebugExamine.Flags().BoolVar(&repairByte, "repair-byte", false, "try to repair broken blobs by trying bytes")
}

func prettyPrintJSON(wr io.Writer, item interface{}) error {
	buf, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}

	_, err = wr.Write(append(buf, '\n'))
	return err
}

func debugPrintSnapshots(ctx context.Context, repo *repository.Repository, wr io.Writer) error {
	return restic.ForAllSnapshots(ctx, repo, nil, func(id restic.ID, snapshot *restic.Snapshot, err error) error {
		if err != nil {
			return err
		}

		fmt.Fprintf(wr, "snapshot_id: %v\n", id)

		return prettyPrintJSON(wr, snapshot)
	})
}

// Pack is the struct used in printPacks.
type Pack struct {
	Name string `json:"name"`

	Blobs []Blob `json:"blobs"`
}

// Blob is the struct used in printPacks.
type Blob struct {
	Type   restic.BlobType `json:"type"`
	Length uint            `json:"length"`
	ID     restic.ID       `json:"id"`
	Offset uint            `json:"offset"`
}

func printPacks(ctx context.Context, repo *repository.Repository, wr io.Writer) error {

	return repo.List(ctx, restic.PackFile, func(id restic.ID, size int64) error {
		h := restic.Handle{Type: restic.PackFile, Name: id.String()}

		blobs, _, err := pack.List(repo.Key(), restic.ReaderAt(ctx, repo.Backend(), h), size)
		if err != nil {
			Warnf("error for pack %v: %v\n", id.Str(), err)
			return nil
		}

		p := Pack{
			Name:  id.String(),
			Blobs: make([]Blob, len(blobs)),
		}
		for i, blob := range blobs {
			p.Blobs[i] = Blob{
				Type:   blob.Type,
				Length: blob.Length,
				ID:     blob.ID,
				Offset: blob.Offset,
			}
		}

		return prettyPrintJSON(wr, p)
	})
}

func dumpIndexes(ctx context.Context, repo restic.Repository, wr io.Writer) error {
	return repository.ForAllIndexes(ctx, repo, func(id restic.ID, idx *repository.Index, oldFormat bool, err error) error {
		Printf("index_id: %v\n", id)
		if err != nil {
			return err
		}

		return idx.Dump(wr)
	})
}

func runDebugDump(gopts GlobalOptions, args []string) error {
	if len(args) != 1 {
		return errors.Fatal("type not specified")
	}

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		lock, err := lockRepo(gopts.ctx, repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	tpe := args[0]

	switch tpe {
	case "indexes":
		return dumpIndexes(gopts.ctx, repo, gopts.stdout)
	case "snapshots":
		return debugPrintSnapshots(gopts.ctx, repo, gopts.stdout)
	case "packs":
		return printPacks(gopts.ctx, repo, gopts.stdout)
	case "all":
		Printf("snapshots:\n")
		err := debugPrintSnapshots(gopts.ctx, repo, gopts.stdout)
		if err != nil {
			return err
		}

		Printf("\nindexes:\n")
		err = dumpIndexes(gopts.ctx, repo, gopts.stdout)
		if err != nil {
			return err
		}

		return nil
	default:
		return errors.Fatalf("no such type %q", tpe)
	}
}

var cmdDebugExamine = &cobra.Command{
	Use:               "examine pack-ID...",
	Short:             "Examine a pack file",
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDebugExamine(globalOptions, args)
	},
}

func tryRepairWithBitflip(ctx context.Context, key *crypto.Key, input []byte, bytewise bool) []byte {
	if bytewise {
		Printf("        trying to repair blob by finding a broken byte\n")
	} else {
		Printf("        trying to repair blob with single bit flip\n")
	}

	ch := make(chan int)
	var wg errgroup.Group
	done := make(chan struct{})
	var fixed []byte
	var found bool

	workers := runtime.GOMAXPROCS(0)
	Printf("         spinning up %d worker functions\n", runtime.GOMAXPROCS(0))
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
					Printf("\n")
					Printf("        blob could be repaired by XORing byte %v with 0x%02x\n", idx, pattern)
					Printf("        hash is %v\n", restic.Hash(plaintext))
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
				Printf("     done after %v\n", time.Since(start))
				return nil
			}

			if time.Since(info) > time.Second {
				secs := time.Since(start).Seconds()
				gps := float64(i) / secs
				remaining := len(input) - i
				eta := time.Duration(float64(remaining)/gps) * time.Second

				Printf("\r%d byte of %d done (%.2f%%), %.0f byte per second, ETA %v",
					i, len(input), float32(i)/float32(len(input))*100, gps, eta)
				info = time.Now()
			}
		}
		return nil
	})
	err := wg.Wait()
	if err != nil {
		panic("all go rountines can only return nil")
	}

	if !found {
		Printf("\n        blob could not be repaired\n")
	}
	return fixed
}

func decryptUnsigned(ctx context.Context, k *crypto.Key, buf []byte) []byte {
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

func loadBlobs(ctx context.Context, repo restic.Repository, pack restic.ID, list []restic.Blob) error {
	be := repo.Backend()
	h := restic.Handle{
		Name: pack.String(),
		Type: restic.PackFile,
	}
	for _, blob := range list {
		Printf("      loading blob %v at %v (length %v)\n", blob.ID, blob.Offset, blob.Length)
		buf := make([]byte, blob.Length)
		err := be.Load(ctx, h, int(blob.Length), int64(blob.Offset), func(rd io.Reader) error {
			n, err := io.ReadFull(rd, buf)
			if err != nil {
				return fmt.Errorf("read error after %d bytes: %v", n, err)
			}
			return nil
		})
		if err != nil {
			Warnf("error read: %v\n", err)
			continue
		}

		key := repo.Key()

		nonce, plaintext := buf[:key.NonceSize()], buf[key.NonceSize():]
		plaintext, err = key.Open(plaintext[:0], nonce, plaintext, nil)
		if err != nil {
			Warnf("error decrypting blob: %v\n", err)
			var plain []byte
			if tryRepair || repairByte {
				plain = tryRepairWithBitflip(ctx, key, buf, repairByte)
			}
			var prefix string
			if plain != nil {
				id := restic.Hash(plain)
				if !id.Equal(blob.ID) {
					Printf("         repaired blob (length %v), hash is %v, ID does not match, wanted %v\n", len(plain), id, blob.ID)
					prefix = "repaired-wrong-hash-"
				} else {
					Printf("         successfully repaired blob (length %v), hash is %v, ID matches\n", len(plain), id)
					prefix = "repaired-"
				}
			} else {
				plain = decryptUnsigned(ctx, key, buf)
				prefix = "damaged-"
			}
			err = storePlainBlob(blob.ID, prefix, plain)
			if err != nil {
				return err
			}
			continue
		}

		id := restic.Hash(plaintext)
		var prefix string
		if !id.Equal(blob.ID) {
			Printf("         successfully decrypted blob (length %v), hash is %v, ID does not match, wanted %v\n", len(plaintext), id, blob.ID)
			prefix = "wrong-hash-"
		} else {
			Printf("         successfully decrypted blob (length %v), hash is %v, ID matches\n", len(plaintext), id)
			prefix = "correct-"
		}
		if extractPack {
			err = storePlainBlob(id, prefix, plaintext)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func storePlainBlob(id restic.ID, prefix string, plain []byte) error {
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

	Printf("decrypt of blob %v stored at %v\n", id, filename)
	return nil
}

func runDebugExamine(gopts GlobalOptions, args []string) error {
	ids := make([]restic.ID, 0)
	for _, name := range args {
		id, err := restic.ParseID(name)
		if err != nil {
			Warnf("error: %v\n", err)
			continue
		}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return errors.Fatal("no pack files to examine")
	}

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		lock, err := lockRepo(gopts.ctx, repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	err = repo.LoadIndex(gopts.ctx)
	if err != nil {
		return err
	}

	for _, id := range ids {
		err := examinePack(gopts.ctx, repo, id)
		if err != nil {
			Warnf("error: %v\n", err)
		}
		if err == context.Canceled {
			break
		}
	}
	return nil
}

func examinePack(ctx context.Context, repo restic.Repository, id restic.ID) error {
	Printf("examine %v\n", id)

	h := restic.Handle{
		Type: restic.PackFile,
		Name: id.String(),
	}
	fi, err := repo.Backend().Stat(ctx, h)
	if err != nil {
		return err
	}
	Printf("  file size is %v\n", fi.Size)

	buf, err := backend.LoadAll(ctx, nil, repo.Backend(), h)
	if err != nil {
		return err
	}
	gotID := restic.Hash(buf)
	if !id.Equal(gotID) {
		Printf("  wanted hash %v, got %v\n", id, gotID)
	} else {
		Printf("  hash for file content matches\n")
	}

	Printf("  ========================================\n")
	Printf("  looking for info in the indexes\n")

	blobsLoaded := false
	// examine all data the indexes have for the pack file
	for _, idx := range repo.Index().(*repository.MasterIndex).All() {
		idxIDs, err := idx.IDs()
		if err != nil {
			idxIDs = restic.IDs{}
		}

		blobs := idx.ListPack(id)
		if len(blobs) == 0 {
			continue
		}

		Printf("    index %v:\n", idxIDs)

		// convert list of blobs to []restic.Blob
		var list []restic.Blob
		for _, b := range blobs {
			list = append(list, b.Blob)
		}
		checkPackSize(list, fi.Size)

		err = loadBlobs(ctx, repo, id, list)
		if err != nil {
			Warnf("error: %v\n", err)
		} else {
			blobsLoaded = true
		}
	}

	Printf("  ========================================\n")
	Printf("  inspect the pack itself\n")

	blobs, _, err := pack.List(repo.Key(), restic.ReaderAt(ctx, repo.Backend(), h), fi.Size)
	if err != nil {
		return fmt.Errorf("pack %v: %v", id.Str(), err)
	}
	checkPackSize(blobs, fi.Size)

	if !blobsLoaded {
		return loadBlobs(ctx, repo, id, blobs)
	}
	return nil
}

func checkPackSize(blobs []restic.Blob, fileSize int64) {
	// track current size and offset
	var size, offset uint64

	sort.Slice(blobs, func(i, j int) bool {
		return blobs[i].Offset < blobs[j].Offset
	})

	for _, pb := range blobs {
		Printf("      %v blob %v, offset %-6d, raw length %-6d\n", pb.Type, pb.ID, pb.Offset, pb.Length)
		if offset != uint64(pb.Offset) {
			Printf("      hole in file, want offset %v, got %v\n", offset, pb.Offset)
		}
		offset += uint64(pb.Length)
		size += uint64(pb.Length)
	}

	// compute header size, per blob: 1 byte type, 4 byte length, 32 byte id
	size += uint64(restic.CiphertextLength(len(blobs) * (1 + 4 + 32)))
	// length in uint32 little endian
	size += 4

	if uint64(fileSize) != size {
		Printf("      file sizes do not match: computed %v from index, file size is %v\n", size, fileSize)
	} else {
		Printf("      file sizes match\n")
	}
}
