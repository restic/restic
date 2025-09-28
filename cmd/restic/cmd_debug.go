//go:build debug
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
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/repository/index"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
)

func registerDebugCommand(cmd *cobra.Command, globalOptions *global.Options) {
	cmd.AddCommand(
		newDebugCommand(globalOptions),
	)
}

func newDebugCommand(globalOptions *global.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "debug",
		Short:             "Debug commands",
		GroupID:           cmdGroupDefault,
		DisableAutoGenTag: true,
	}
	cmd.AddCommand(newDebugDumpCommand(globalOptions))
	cmd.AddCommand(newDebugExamineCommand(globalOptions))
	return cmd
}

func newDebugDumpCommand(globalOptions *global.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dump [indexes|snapshots|all|packs]",
		Short: "Dump data structures",
		Long: `
The "dump" command dumps data structures from the repository as JSON objects. It
is used for debugging purposes only.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
`,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDebugDump(cmd.Context(), *globalOptions, args, globalOptions.Term)
		},
	}
	return cmd
}

func newDebugExamineCommand(globalOptions *global.Options) *cobra.Command {
	var opts DebugExamineOptions

	cmd := &cobra.Command{
		Use:               "examine pack-ID...",
		Short:             "Examine a pack file",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDebugExamine(cmd.Context(), *globalOptions, opts, args, globalOptions.Term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

type DebugExamineOptions struct {
	TryRepair     bool
	RepairByte    bool
	ExtractPack   bool
	ReuploadBlobs bool
}

func (opts *DebugExamineOptions) AddFlags(f *pflag.FlagSet) {
	f.BoolVar(&opts.ExtractPack, "extract-pack", false, "write blobs to the current directory")
	f.BoolVar(&opts.ReuploadBlobs, "reupload-blobs", false, "reupload blobs to the repository")
	f.BoolVar(&opts.TryRepair, "try-repair", false, "try to repair broken blobs with single bit flips")
	f.BoolVar(&opts.RepairByte, "repair-byte", false, "try to repair broken blobs by trying bytes")
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
	return data.ForAllSnapshots(ctx, repo, repo, nil, func(id restic.ID, snapshot *data.Snapshot, err error) error {
		if err != nil {
			return err
		}

		if _, err := fmt.Fprintf(wr, "snapshot_id: %v\n", id); err != nil {
			return err
		}

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

func printPacks(ctx context.Context, repo *repository.Repository, wr io.Writer, printer progress.Printer) error {

	var m sync.Mutex
	return restic.ParallelList(ctx, repo, restic.PackFile, repo.Connections(), func(ctx context.Context, id restic.ID, size int64) error {
		blobs, _, err := repo.ListPack(ctx, id, size)
		if err != nil {
			printer.E("error for pack %v: %v", id.Str(), err)
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

		m.Lock()
		defer m.Unlock()
		return prettyPrintJSON(wr, p)
	})
}

func dumpIndexes(ctx context.Context, repo restic.ListerLoaderUnpacked, wr io.Writer, printer progress.Printer) error {
	return index.ForAllIndexes(ctx, repo, repo, func(id restic.ID, idx *index.Index, err error) error {
		printer.S("index_id: %v", id)
		if err != nil {
			return err
		}

		return idx.Dump(wr)
	})
}

func runDebugDump(ctx context.Context, gopts global.Options, args []string, term ui.Terminal) error {
	printer := ui.NewProgressPrinter(false, gopts.Verbosity, term)

	if len(args) != 1 {
		return errors.Fatal("type not specified")
	}

	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock, printer)
	if err != nil {
		return err
	}
	defer unlock()

	tpe := args[0]

	switch tpe {
	case "indexes":
		return dumpIndexes(ctx, repo, gopts.Term.OutputWriter(), printer)
	case "snapshots":
		return debugPrintSnapshots(ctx, repo, gopts.Term.OutputWriter())
	case "packs":
		return printPacks(ctx, repo, gopts.Term.OutputWriter(), printer)
	case "all":
		printer.S("snapshots:")
		err := debugPrintSnapshots(ctx, repo, gopts.Term.OutputWriter())
		if err != nil {
			return err
		}

		printer.S("indexes:")
		err = dumpIndexes(ctx, repo, gopts.Term.OutputWriter(), printer)
		if err != nil {
			return err
		}

		return nil
	default:
		return errors.Fatalf("no such type %q", tpe)
	}
}

func tryRepairWithBitflip(key *crypto.Key, input []byte, bytewise bool, printer progress.Printer) []byte {
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

func loadBlobs(ctx context.Context, opts DebugExamineOptions, repo restic.Repository, packID restic.ID, list []restic.Blob, printer progress.Printer) error {
	dec, err := zstd.NewReader(nil)
	if err != nil {
		panic(err)
	}

	pack, err := repo.LoadRaw(ctx, restic.PackFile, packID)
	// allow processing broken pack files
	if pack == nil {
		return err
	}

	wg, ctx := errgroup.WithContext(ctx)

	if opts.ReuploadBlobs {
		repo.StartPackUploader(ctx, wg)
	}

	wg.Go(func() error {
		for _, blob := range list {
			printer.S("      loading blob %v at %v (length %v)", blob.ID, blob.Offset, blob.Length)
			if int(blob.Offset+blob.Length) > len(pack) {
				printer.E("skipping truncated blob")
				continue
			}
			buf := pack[blob.Offset : blob.Offset+blob.Length]
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
				_, _, _, err := repo.SaveBlob(ctx, blob.Type, plaintext, id, true)
				if err != nil {
					return err
				}
				printer.S("         uploaded %v %v", blob.Type, id)
			}
		}

		if opts.ReuploadBlobs {
			return repo.Flush(ctx)
		}
		return nil
	})

	return wg.Wait()
}

func storePlainBlob(id restic.ID, prefix string, plain []byte, printer progress.Printer) error {
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

func runDebugExamine(ctx context.Context, gopts global.Options, opts DebugExamineOptions, args []string, term ui.Terminal) error {
	printer := ui.NewProgressPrinter(false, gopts.Verbosity, term)

	if opts.ExtractPack && gopts.NoLock {
		return fmt.Errorf("--extract-pack and --no-lock are mutually exclusive")
	}

	ctx, repo, unlock, err := openWithAppendLock(ctx, gopts, gopts.NoLock, printer)
	if err != nil {
		return err
	}
	defer unlock()

	ids := make([]restic.ID, 0)
	for _, name := range args {
		id, err := restic.ParseID(name)
		if err != nil {
			id, err = restic.Find(ctx, repo, restic.PackFile, name)
			if err != nil {
				printer.E("error: %v", err)
				continue
			}
		}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return errors.Fatal("no pack files to examine")
	}

	err = repo.LoadIndex(ctx, printer)
	if err != nil {
		return err
	}

	for _, id := range ids {
		err := examinePack(ctx, opts, repo, id, printer)
		if err != nil {
			printer.E("error: %v", err)
		}
		if err == context.Canceled {
			break
		}
	}
	return nil
}

func examinePack(ctx context.Context, opts DebugExamineOptions, repo restic.Repository, id restic.ID, printer progress.Printer) error {
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
	for b := range repo.ListPacksFromIndex(ctx, restic.NewIDSet(id)) {
		blobs := b.Blobs
		if len(blobs) == 0 {
			continue
		}

		checkPackSize(blobs, len(buf), printer)

		err = loadBlobs(ctx, opts, repo, id, blobs, printer)
		if err != nil {
			printer.E("error: %v", err)
		} else {
			blobsLoaded = true
		}
	}

	printer.S("  ========================================")
	printer.S("  inspect the pack itself")

	blobs, _, err := repo.ListPack(ctx, id, int64(len(buf)))
	if err != nil {
		return fmt.Errorf("pack %v: %v", id.Str(), err)
	}
	checkPackSize(blobs, len(buf), printer)

	if !blobsLoaded {
		return loadBlobs(ctx, opts, repo, id, blobs, printer)
	}
	return nil
}

func checkPackSize(blobs []restic.Blob, fileSize int, printer progress.Printer) {
	// track current size and offset
	var size, offset uint64

	sort.Slice(blobs, func(i, j int) bool {
		return blobs[i].Offset < blobs[j].Offset
	})

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
