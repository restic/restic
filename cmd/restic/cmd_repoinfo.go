package main

import (
	"context"
	"encoding/binary"
	"sort"
	"strings"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/restic"
	"github.com/spf13/cobra"
)

var cmdRepoInfo = &cobra.Command{
	Use:   "repoinfo [stats]",
	Short: "Show info about the",
	Long: `
The "repoinfo" command displays general repository information and some
statistics. It does not walk any snapshots or tress. 

If you need specific information about a snapshot, consider using "repo stats".
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRepoInfo(repoInfoOptions, globalOptions)
	},
}

// RepoinfoOptions collects all options for the repoinfo command.
type RepoInfoOptions struct {
	ShowStats   bool
	ShowPercent bool
	ScanRepo    bool
	ScanIndex   bool
}

var repoInfoOptions RepoInfoOptions

func init() {
	cmdRoot.AddCommand(cmdRepoInfo)
	f := cmdRepoInfo.Flags()
	f.BoolVar(&repoInfoOptions.ShowPercent, "percentage", false, "show percentage")
	f.BoolVar(&repoInfoOptions.ShowStats, "stats", false, "show statistics")
	f.BoolVar(&repoInfoOptions.ScanRepo, "scan-repo", true, "scan repository")
	f.BoolVar(&repoInfoOptions.ScanIndex, "scan-index", true, "scan index")
}

func runRepoInfo(opts RepoInfoOptions, gopts GlobalOptions) error {
	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()

	if !opts.ScanRepo && !opts.ScanIndex {
		Println("neither --scan-repo nor --scan-index specified -> nothing to do")
		return nil
	}

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if opts.ScanIndex {
		if err = repo.LoadIndex(ctx); err != nil {
			return err
		}
	}

	if !gopts.NoLock {
		lock, err := lockRepo(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	return statsRepository(ctx, repo, opts)
}

func statsRepository(ctx context.Context, repo restic.Repository, opts RepoInfoOptions) error {
	be := repo.Backend()

	// get information from backend for all repo types
	scanRepo := []restic.FileType{restic.KeyFile, restic.LockFile, restic.SnapshotFile, restic.IndexFile, restic.PackFile}
	infoRepo := newRepoinfo("all files:")
	statRepo := newStatinfo("all files:")

	if opts.ScanRepo {
		Println("scanning repo..")
		for _, tpe := range scanRepo {
			title := tpe.String() + "s:"
			err := be.List(ctx, tpe, func(fi restic.FileInfo) error {
				size := uint64(fi.Size)
				switch tpe {
				case restic.KeyFile:
					//key files have 100% crypto part
					infoRepo.add(title, 0, size)
				case restic.PackFile:
					//pack files have crypto only in blobs and pack overhead
					infoRepo.add(title, size, 0)
				default:
					infoRepo.add(title, size-crypto.Extension, crypto.Extension)
				}
				// statistics present total file size
				statRepo.add(title, size)
				return nil
			})
			if err != nil {
				return err
			}
		}
		// print results
		Printf("\nRepository content:\n==================\n")
		infoRepo.print()

		if opts.ShowPercent {
			Printf("\npercentage:\n")
			infoRepo.printPercent()
		}

		if opts.ShowStats {
			Printf("\nstatistics - file size:\n")
			statRepo.print()
		}
	}

	infoPacks := newPackinfo("all blobs:")
	statIndex := newStatinfo("all blobs:")

	if opts.ScanIndex {
		Println("scanning index..")
		for pb := range repo.Index().Each(ctx) {
			title := pb.Type.String() + " blobs:"
			// use the raw size, i.e. without crypto overhead
			size := uint64(pb.Length) - crypto.Extension
			infoPacks.add(title, size, pb.PackID)
			statIndex.add(title, size)
		}

		Printf("\nIndex content:\n==============\n")
		infoPacks.print()

		if opts.ShowPercent {
			Printf("\npercentage:\n")
			infoPacks.printPercent()
		}

		if opts.ShowStats {
			Printf("\nstatistics - raw blobs:\n")
			statIndex.print()
		}
	}

	if !opts.ScanRepo || !opts.ScanIndex {
		// Totals are only available when both repo and index are scanned
		return nil
	}

	// totals
	totalSize := infoRepo.all.size + infoRepo.all.crypto

	totalPackSizeFromRepo := infoRepo.byString["pack files:"].size
	totalPackSizeFromIndex := infoPacks.all.size + infoPacks.all.header() + infoPacks.all.crypto()

	// overheads
	totalIdx := infoRepo.byString["index files:"].size
	totalSnp := infoRepo.byString["snapshot files:"].size
	totalLock := infoRepo.byString["lock files:"].size
	totalPackHdr := infoPacks.all.header()
	totalCrypto := infoRepo.all.crypto + infoPacks.all.crypto()
	totalOverhead := totalIdx + totalSnp + totalPackHdr + totalCrypto
	// totalDiff is the difference between Size from Repo and size from index - is calculated later
	totalDiff := uint64(0)

	if totalPackSizeFromIndex > totalPackSizeFromRepo {
		Printf("\nWarning: Calculated packsize is %s greater than the actual total size of pack files!\n",
			formatBytes(totalPackSizeFromIndex-totalPackSizeFromRepo))
		Printf("Please check your repo now - you may have lost data!\n")
		return nil
	}
	if totalPackSizeFromIndex < totalPackSizeFromRepo {
		Printf("\nCalculated packsize is %s smaller than the actual total size of pack files!\n",
			formatBytes(totalPackSizeFromRepo-totalPackSizeFromIndex))
		Printf("This means there are packs that contain blobs which are not referenced.\n\n")
		Printf("This is most likely due to abborted backup operations or abborted 'prune'.\n")
		Printf("It can also indicate that something is not correct with your repository.\n")
		Printf("Please run 'restic check'.\n")
		totalDiff = totalPackSizeFromRepo - totalPackSizeFromIndex
		// add difference to overhead
		totalOverhead += totalDiff
	}

	Printf("\nOverhead:\n=========\n")
	Printf("%-22s %11s (%7s)\n%-22s %11s (%7s)\n%-22s %11s (%7s)\n%-22s %11s (%7s)\n%-22s %11s (%7s)\n",
		"index:", formatBytes(totalIdx), formatPercent(totalIdx, totalSize),
		"snapshots:", formatBytes(totalSnp), formatPercent(totalSnp, totalSize),
		"locks:", formatBytes(totalLock), formatPercent(totalLock, totalSize),
		"pack header:", formatBytes(totalPackHdr), formatPercent(totalPackHdr, totalSize),
		"crypto:", formatBytes(totalCrypto), formatPercent(totalCrypto, totalSize))
	if totalDiff > 0 {
		Printf("%-22s %11s (%7s)\n",
			"unused blobs in packs:", formatBytes(totalDiff), formatPercent(totalDiff, totalSize))
	}
	Printf("%s\n", strings.Repeat("-", 44))
	Printf("%-22s %11s (%7s)\n",
		"total:", formatBytes(totalOverhead), formatPercent(totalOverhead, totalSize))

	Printf("\nTotal:\n======\n")

	Printf("%11d blobs\n%11d files\n%11s total repository size\n\n",
		infoPacks.all.count, infoRepo.all.count, formatBytes(totalSize))

	return nil
}

type ri struct {
	title  string
	count  uint64
	size   uint64
	crypto uint64
}

type repoinfo struct {
	byString map[string]ri
	all      ri
	allTitle string
}

func newRepoinfo(allTitle string) *repoinfo {
	return &repoinfo{allTitle: allTitle, byString: make(map[string]ri)}
}

func (info *repoinfo) print() {
	Printf("%-15s %11s | %11s | %11s | %11s\n",
		"", "count", "raw size", "crypto", "encr size")
	Printf("%s\n", strings.Repeat("-", 69))

	var sortedStrings []string
	for s := range info.byString {
		sortedStrings = append(sortedStrings, s)
	}
	sort.Strings(sortedStrings)

	for _, s := range sortedStrings {
		ri := info.byString[s]
		Printf("%-15s %11d | %11s | %11s | %11s\n",
			s, ri.count, formatBytes(ri.size),
			formatBytes(ri.crypto), formatBytes(ri.size+ri.crypto))
	}
	Printf("%s\n", strings.Repeat("-", 69))
	Printf("%-15s %11d | %11s | %11s | %11s\n\n",
		info.allTitle, info.all.count, formatBytes(info.all.size),
		formatBytes(info.all.crypto), formatBytes(info.all.size+info.all.crypto))
}

func (info *repoinfo) printPercent() {
	Printf("%-15s %11s | %11s | %11s | %11s\n",
		"", "count", "raw size", "crypto", "encr size")
	Printf("%s\n", strings.Repeat("-", 69))

	var sortedStrings []string
	for s := range info.byString {
		sortedStrings = append(sortedStrings, s)
	}
	sort.Strings(sortedStrings)

	for _, s := range sortedStrings {
		ri := info.byString[s]
		Printf("%-15s     %7s |     %7s |     %7s |     %7s\n",
			s, formatPercent(ri.count, info.all.count),
			formatPercent(ri.size, info.all.size),
			formatPercent(ri.crypto, info.all.crypto),
			formatPercent(ri.size+ri.crypto, info.all.size+info.all.crypto))
	}
	Printf("%s\n", strings.Repeat("-", 69))
}

func (info *repoinfo) add(s string, size uint64, crypto uint64) {
	ri := info.byString[s]
	ri.size += size
	ri.crypto += crypto
	ri.count++
	info.byString[s] = ri

	info.all.size += size
	info.all.crypto += crypto
	info.all.count++
}

type pi struct {
	title string
	count uint64
	size  uint64
	packs restic.IDSet
}

func (info *pi) crypto() uint64 {
	return (uint64(len(info.packs)) + info.count) * crypto.Extension
}

func (info *pi) header() uint64 {
	// this is identical to entrySize in /internal/pack/pack.go
	headerDataPerBlob := uint64(binary.Size(restic.BlobType(0)) + binary.Size(uint32(0)) + len(restic.ID{}))

	return 4*uint64(len(info.packs)) + info.count*headerDataPerBlob
}

func (info *pi) total() uint64 {
	return info.size + info.header() + info.crypto()
}

type packinfo struct {
	byString map[string]pi
	all      pi
	allTitle string
}

func newPackinfo(allTitle string) *packinfo {
	all := pi{packs: restic.NewIDSet()}
	return &packinfo{allTitle: allTitle, byString: make(map[string]pi), all: all}
}

func (info *packinfo) print() {
	Printf("%-12s %11s | %11s | %11s | %11s | %11s | %11s\n",
		"", "# packs", "# blobs", "raw blobs", "pack header", "crypto", "total")
	Printf("%s\n", strings.Repeat("-", 94))

	var sortedStrings []string
	for s := range info.byString {
		sortedStrings = append(sortedStrings, s)
	}
	sort.Strings(sortedStrings)

	for _, s := range sortedStrings {
		pi := info.byString[s]
		Printf("%-12s %11d | %11d | %11s | %11s | %11s | %11s\n",
			s, len(pi.packs), pi.count, formatBytes(pi.size),
			formatBytes(pi.header()), formatBytes(pi.crypto()),
			formatBytes(pi.total()))
	}
	Printf("%s\n", strings.Repeat("-", 94))
	Printf("%-12s %11d | %11d | %11s | %11s | %11s | %11s\n",
		info.all.title, len(info.all.packs), info.all.count, formatBytes(info.all.size),
		formatBytes(info.all.header()), formatBytes(info.all.crypto()),
		formatBytes(info.all.total()))
}

func (info *packinfo) printPercent() {
	Printf("%-12s %11s | %11s | %11s | %11s | %11s | %11s\n",
		"", "# packs", "# blobs", "raw blobs", "pack header", "crypto", "total")
	Printf("%s\n", strings.Repeat("-", 94))

	var sortedStrings []string
	for s := range info.byString {
		sortedStrings = append(sortedStrings, s)
	}
	sort.Strings(sortedStrings)

	for _, s := range sortedStrings {
		pi := info.byString[s]
		Printf("%-12s %11s | %11s | %11s | %11s | %11s | %11s\n",
			s, formatPercent(uint64(len(pi.packs)), uint64(len(info.all.packs))),
			formatPercent(pi.count, info.all.count), formatPercent(pi.size, info.all.size),
			formatPercent(pi.header(), info.all.header()),
			formatPercent(pi.crypto(), info.all.crypto()),
			formatPercent(pi.total(), info.all.total()))
	}
	Printf("%s\n", strings.Repeat("-", 94))
}

func (info *packinfo) add(s string, size uint64, packID restic.ID) {
	pi, ok := info.byString[s]
	if !ok {
		pi.packs = restic.NewIDSet()
	}
	pi.size += size
	pi.count++
	pi.packs.Insert(packID)
	info.byString[s] = pi

	info.all.size += size
	info.all.count++
	info.all.packs.Insert(packID)
}

type si struct {
	title     string
	count     uint64
	sizeMin   uint64
	sizeMax   uint64
	sizeTotal uint64
}

type statinfo struct {
	byString map[string]si
	all      si
	allTitle string
}

func newStatinfo(allTitle string) *statinfo {
	return &statinfo{allTitle: allTitle, byString: make(map[string]si)}
}

func (info *statinfo) print() {
	Printf("%-15s %11s | %11s | %11s\n",
		"", "min", "max", "avg")
	Printf("%s\n", strings.Repeat("-", 55))

	var avg uint64
	var sortedStrings []string
	for s := range info.byString {
		sortedStrings = append(sortedStrings, s)
	}
	sort.Strings(sortedStrings)

	for _, s := range sortedStrings {
		si := info.byString[s]
		if si.count == 0 {
			avg = 0
		} else {
			avg = si.sizeTotal / si.count
		}
		Printf("%-15s %11s | %11s | %11s\n",
			s, formatBytes(si.sizeMin), formatBytes(si.sizeMax), formatBytes(avg))
	}

	Printf("%s\n", strings.Repeat("-", 55))

	if info.all.count == 0 {
		avg = 0
	} else {
		avg = info.all.sizeTotal / info.all.count
	}
	Printf("%-15s %11s | %11s | %11s\n",
		info.allTitle, formatBytes(info.all.sizeMin), formatBytes(info.all.sizeMax), formatBytes(avg))
}

func (info *si) add(size uint64) {
	info.sizeTotal += size
	info.count++
	if info.sizeMin == 0 || info.sizeMin > size {
		info.sizeMin = size
	}
	if info.sizeMax < size {
		info.sizeMax = size
	}
}

func (info *statinfo) add(s string, size uint64) {
	si := info.byString[s]
	si.add(size)
	info.byString[s] = si
	info.all.add(size)
}
