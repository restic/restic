package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

// findPerSnapshotPrinter is a statefulOutput printer that attributes every
// matched path to the snapshot whose header most recently preceded it. The
// normal-mode PrintPattern path emits a "Found matching entries in snapshot
// <id> from <time>" header via P before each snapshot's matches, so tracking P
// calls recovers the per-snapshot grouping that a flat line sink loses.
type findPerSnapshotPrinter struct {
	mu      sync.Mutex
	order   []string
	current string
	sets    map[string]map[string]bool
}

func newFindPerSnapshotPrinter() *findPerSnapshotPrinter {
	return &findPerSnapshotPrinter{sets: make(map[string]map[string]bool)}
}

func (p *findPerSnapshotPrinter) S(format string, args ...interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == "" {
		return
	}
	line := fmt.Sprintf(format, args...)
	if p.sets[p.current] == nil {
		p.sets[p.current] = make(map[string]bool)
	}
	p.sets[p.current][line] = true
}

func (p *findPerSnapshotPrinter) P(format string, args ...interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	line := fmt.Sprintf(format, args...)
	if line == "" {
		return
	}
	var id string
	if _, err := fmt.Sscanf(line, "Found matching entries in snapshot %s from", &id); err != nil || id == "" {
		return
	}
	p.current = id
	if _, ok := p.sets[id]; !ok {
		p.sets[id] = make(map[string]bool)
		p.order = append(p.order, id)
	}
}

func (p *findPerSnapshotPrinter) E(string, ...interface{}) {}

func (p *findPerSnapshotPrinter) Order() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.order)
}

func (p *findPerSnapshotPrinter) Sets() map[string]map[string]bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]map[string]bool, len(p.sets))
	for k, v := range p.sets {
		cp := make(map[string]bool, len(v))
		for path := range v {
			cp[path] = true
		}
		out[k] = cp
	}
	return out
}

// runPatternSearchPerSnapshot runs the inverted pattern search over the given
// snapshots (in the supplied order) and returns, per snapshot, the set of
// matched paths plus the order in which snapshots first appear in the output.
// Snapshots with no matches do not appear.
func runPatternSearchPerSnapshot(t testing.TB, repo restic.Repository, snapshots []*data.Snapshot, patterns ...string) ([]string, map[string]map[string]bool) {
	t.Helper()
	printer := newFindPerSnapshotPrinter()
	finder := &Finder{
		repo:    repo,
		pat:     findPattern{pattern: patterns},
		out:     statefulOutput{printer: printer, stdout: io.Discard},
		printer: printer,
	}
	rtest.OK(t, finder.findPatternInverted(context.Background(), snapshots))
	return printer.Order(), printer.Sets()
}

// buildSharedSubtreeSnapshots builds a four-snapshot fixture exercising
// cross-snapshot subtree sharing, divergence, and a no-match snapshot:
//
//	sharedDocsTree  -> needle.txt            (shared by sn1, sn2 at /docs)
//	extraTree       -> needle.txt            (sn2 only, at /extra)
//	altDocsTree     -> needle.txt            (sn3 only, at /other)
//	rootEmpty       -> readme.txt            (sn4, no needle -> no matches)
//
//	sn1: /docs -> sharedDocsTree
//	sn2: /docs -> sharedDocsTree, /extra -> extraTree
//	sn3: /other -> altDocsTree
//	sn4: /readme.txt
//
// Pattern "needle.txt" yields:
//
//	sn1: {/docs/needle.txt}
//	sn2: {/docs/needle.txt, /extra/needle.txt}
//	sn3: {/other/needle.txt}
//	sn4: {} (does not appear in output)
func buildSharedSubtreeSnapshots(t testing.TB, repo restic.Repository) []*data.Snapshot {
	var (
		sharedDocsTree restic.ID
		extraTree      restic.ID
		altDocsTree    restic.ID
		rootShared     restic.ID
		rootExtra      restic.ID
		rootAlt        restic.ID
		rootEmpty      restic.ID
	)

	err := repo.WithBlobUploader(context.TODO(), func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		sharedDocsTree = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "needle.txt", Type: data.NodeTypeFile, Mode: 0644, Size: 1},
		})
		extraTree = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "needle.txt", Type: data.NodeTypeFile, Mode: 0644, Size: 2},
		})
		altDocsTree = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "needle.txt", Type: data.NodeTypeFile, Mode: 0644, Size: 3},
		})
		rootShared = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "docs", Type: data.NodeTypeDir, Mode: 0755, Subtree: &sharedDocsTree},
		})
		rootExtra = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "docs", Type: data.NodeTypeDir, Mode: 0755, Subtree: &sharedDocsTree},
			{Name: "extra", Type: data.NodeTypeDir, Mode: 0755, Subtree: &extraTree},
		})
		rootAlt = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "other", Type: data.NodeTypeDir, Mode: 0755, Subtree: &altDocsTree},
		})
		rootEmpty = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "readme.txt", Type: data.NodeTypeFile, Mode: 0644, Size: 1},
		})
		return nil
	})
	rtest.OK(t, err)

	base := time.Unix(1700000000, 0)
	return []*data.Snapshot{
		saveSnapshotWithTree(t, repo, rootShared, base),
		saveSnapshotWithTree(t, repo, rootExtra, base.Add(time.Second)),
		saveSnapshotWithTree(t, repo, rootAlt, base.Add(2*time.Second)),
		saveSnapshotWithTree(t, repo, rootEmpty, base.Add(3*time.Second)),
	}
}

func setEquals(want, got map[string]bool) bool {
	if len(want) != len(got) {
		return false
	}
	for k := range want {
		if !got[k] {
			return false
		}
	}
	return true
}

// TestFindPatternEquivalence is the golden harness for the inverted pattern
// search. It pins the contract that the live findPatternInverted path must
// preserve: which snapshots appear in the output, the order in which they
// appear, and the set of matched paths attributed to each snapshot (set
// equality, not intra-snapshot order).
func TestFindPatternEquivalence(t *testing.T) {
	repo := repository.TestRepository(t)
	snapshots := buildSharedSubtreeSnapshots(t, repo)

	countingRepo := &findCountingRepository{Repository: repo}
	order, sets := runPatternSearchPerSnapshot(t, countingRepo, snapshots, "needle.txt")

	// Every snapshot with matches appears, in input order; the no-match
	// snapshot (snapshots[3]) is absent.
	wantOrder := []string{
		snapshots[0].ID().Str(),
		snapshots[1].ID().Str(),
		snapshots[2].ID().Str(),
	}
	rtest.Equals(t, wantOrder, order, "snapshot output order/grouping mismatch")

	// Per-snapshot match sets: shared subtree attributed to every sharing
	// snapshot, the diverged subtree only to its own snapshot, and the
	// unique extra subtree only to the snapshot that contains it.
	wantSets := map[string]map[string]bool{
		snapshots[0].ID().Str(): {"/docs/needle.txt": true},
		snapshots[1].ID().Str(): {"/docs/needle.txt": true, "/extra/needle.txt": true},
		snapshots[2].ID().Str(): {"/other/needle.txt": true},
	}
	for id, want := range wantSets {
		rtest.Assert(t, setEquals(want, sets[id]),
			"snapshot %s: want matches %v, got %v", id, want, sets[id])
	}

	// The no-match snapshot produced no output group.
	_, present := sets[snapshots[3].ID().Str()]
	rtest.Assert(t, !present, "no-match snapshot must not appear in output: got %v", sets[snapshots[3].ID().Str()])
}

// recordingPrinter records the exact sequence of P and S calls (with their
// formatted arguments) so normal-mode pattern output can be asserted at the
// statefulOutput contract level: snapshot header boundaries and the ordered
// match lines.
type recordingPrinter struct {
	mu   sync.Mutex
	logs []string
}

func (p *recordingPrinter) P(format string, args ...interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logs = append(p.logs, "P\x00"+fmt.Sprintf(format, args...))
}

func (p *recordingPrinter) S(format string, args ...interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logs = append(p.logs, "S\x00"+fmt.Sprintf(format, args...))
}

func (p *recordingPrinter) E(string, ...interface{}) {}

func (p *recordingPrinter) Logs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.logs)
}

// TestFindPatternOutputGolden pins the byte-level output contract of the
// inverted pattern search for both the normal and JSON formats. It complements
// TestFindPatternEquivalence (which pins the per-snapshot match SET) by locking
// per-snapshot grouping and output order, within-snapshot match order (sorted by
// path), hit counts, and attribution, so any drift in format, sort, or
// attribution is caught. The fixture is buildSharedSubtreeSnapshots: sn0 and sn1
// share /docs/needle.txt, sn1 also has /extra/needle.txt, sn2 has
// /other/needle.txt, and sn3 has no match.
func TestFindPatternOutputGolden(t *testing.T) {
	repo := repository.TestRepository(t)
	snapshots := buildSharedSubtreeSnapshots(t, repo)
	patterns := []string{"needle.txt"}

	// JSON mode: statefulOutput writes serialized bytes to stdout. Parse them
	// and assert structure, snapshot order, hit counts, and per-snapshot match
	// paths in their emitted (path-sorted) order.
	var jsonOut bytes.Buffer
	jsonPrinter := &recordingPrinter{}
	jsonFinder := &Finder{
		repo:    repo,
		pat:     findPattern{pattern: patterns},
		out:     statefulOutput{JSON: true, printer: jsonPrinter, stdout: &jsonOut},
		printer: jsonPrinter,
	}
	rtest.OK(t, jsonFinder.findPatternInverted(context.Background(), snapshots))

	type goldenMatch struct {
		Path string `json:"path"`
	}
	type goldenSnap struct {
		Matches  []goldenMatch `json:"matches"`
		Hits     int           `json:"hits"`
		Snapshot string        `json:"snapshot"`
	}
	var got []goldenSnap
	rtest.OK(t, json.Unmarshal(jsonOut.Bytes(), &got))

	wantSnapshotIDs := []string{
		snapshots[0].ID().String(),
		snapshots[1].ID().String(),
		snapshots[2].ID().String(),
	}
	rtest.Equals(t, 3, len(got), "snapshots with matches appear in output order; no-match snapshot absent")
	for i, gs := range got {
		rtest.Equals(t, wantSnapshotIDs[i], gs.Snapshot)
	}
	rtest.Equals(t, []goldenMatch{{Path: "/docs/needle.txt"}}, got[0].Matches, "sn0 matches")
	rtest.Equals(t, 1, got[0].Hits, "sn0 hits")
	rtest.Equals(t, []goldenMatch{{Path: "/docs/needle.txt"}, {Path: "/extra/needle.txt"}}, got[1].Matches,
		"sn1 matches in path-sorted order")
	rtest.Equals(t, 2, got[1].Hits, "sn1 hits")
	rtest.Equals(t, []goldenMatch{{Path: "/other/needle.txt"}}, got[2].Matches, "sn2 matches")
	rtest.Equals(t, 1, got[2].Hits, "sn2 hits")

	// Normal mode: headers and match lines reach the printer as P and S calls.
	// Assert the ordered match paths (which pin within-snapshot sort) and the
	// ordered snapshot headers (which pin grouping and attribution). Header
	// timestamps are locale-dependent, so only the snapshot id is checked.
	normPrinter := &recordingPrinter{}
	normFinder := &Finder{
		repo:    repo,
		pat:     findPattern{pattern: patterns},
		out:     statefulOutput{printer: normPrinter, stdout: io.Discard},
		printer: normPrinter,
	}
	rtest.OK(t, normFinder.findPatternInverted(context.Background(), snapshots))

	const headerPrefix = "Found matching entries in snapshot "
	var headerIDs, matchPaths []string
	var separators int
	for _, l := range normPrinter.Logs() {
		kind, body, _ := strings.Cut(l, "\x00")
		switch kind {
		case "P":
			if body == "" {
				separators++
				continue
			}
			rtest.Assert(t, strings.HasPrefix(body, headerPrefix), "unexpected header: %q", body)
			rest := strings.TrimPrefix(body, headerPrefix)
			id, _, _ := strings.Cut(rest, " from")
			headerIDs = append(headerIDs, id)
		case "S":
			matchPaths = append(matchPaths, body)
		}
	}

	wantHeaderIDs := []string{
		snapshots[0].ID().Str(),
		snapshots[1].ID().Str(),
		snapshots[2].ID().Str(),
	}
	rtest.Equals(t, wantHeaderIDs, headerIDs, "snapshot header order")
	rtest.Equals(t, 2, separators, "blank separator between each pair of snapshots")
	rtest.Equals(t,
		[]string{"/docs/needle.txt", "/docs/needle.txt", "/extra/needle.txt", "/other/needle.txt"},
		matchPaths, "match paths in emitted order across snapshots")
}

// buildPathSensitiveSnapshots builds a two-snapshot fixture in which the same
// subtree (leafTree, containing needle.txt) is mounted at two different paths
// in two different snapshots:
//
//	leafTree -> needle.txt                (identical subtree blob)
//	rootAlpha -> alpha -> leafTree        (sn1: /alpha/needle.txt)
//	rootBeta  -> beta  -> leafTree        (sn2: /beta/needle.txt)
//
// Both root trees reference the same leafTree by ID, so the only thing that
// distinguishes the two mounts is the path. A pattern that matches one path
// only ("alpha/needle.txt") therefore pins path sensitivity: the inverted
// walk must bucket children by path, not by treeID, or the two mounts collapse
// into one bucket and the match leaks across snapshots.
func buildPathSensitiveSnapshots(t testing.TB, repo restic.Repository) ([]*data.Snapshot, restic.ID) {
	var (
		leafTree  restic.ID
		rootAlpha restic.ID
		rootBeta  restic.ID
	)

	err := repo.WithBlobUploader(context.TODO(), func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		leafTree = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "needle.txt", Type: data.NodeTypeFile, Mode: 0644, Size: 1},
		})
		rootAlpha = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "alpha", Type: data.NodeTypeDir, Mode: 0755, Subtree: &leafTree},
		})
		rootBeta = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "beta", Type: data.NodeTypeDir, Mode: 0755, Subtree: &leafTree},
		})
		return nil
	})
	rtest.OK(t, err)

	base := time.Unix(1700000000, 0)
	snapshots := []*data.Snapshot{
		saveSnapshotWithTree(t, repo, rootAlpha, base),
		saveSnapshotWithTree(t, repo, rootBeta, base.Add(time.Second)),
	}
	return snapshots, leafTree
}

// TestFindPatternInvertedPathSensitive pins the path-sensitivity contract of
// the inverted walk: an identical subtree mounted at two different paths must
// be processed independently, so a pattern that matches one path only is
// attributed to the snapshot containing that path and never to the snapshot
// containing the other path.
//
// Mutation check: if processGroup keyed its child-bucket merge by treeID only
// (dropping the path), the two mounts of leafTree would collapse into a single
// bucket. The walk would then load leafTree once and attribute whatever
// nodePath that single bucket computed to both snapshots — so either sn2 would
// wrongly receive /alpha/needle.txt, or sn1 would lose its /alpha/needle.txt
// match (or receive /beta/needle.txt, which the pattern does not match). Both
// outcomes violate the assertions below. The LoadCount assertion additionally
// fails: the correct walk loads leafTree once per distinct path (2), whereas a
// treeID-only merge loads it once.
func TestFindPatternInvertedPathSensitive(t *testing.T) {
	repo := repository.TestRepository(t)
	snapshots, leafTree := buildPathSensitiveSnapshots(t, repo)

	countingRepo := &findCountingRepository{Repository: repo}
	order, sets := runPatternSearchPerSnapshot(t, countingRepo, snapshots, "alpha/needle.txt")

	// sn1 contains /alpha; only it matches. sn2 contains /beta; it must not
	// appear in the output at all, and must carry no matches.
	wantOrder := []string{snapshots[0].ID().Str()}
	rtest.Equals(t, wantOrder, order, "only the /alpha snapshot should appear in output")

	wantSets := map[string]map[string]bool{
		snapshots[0].ID().Str(): {"/alpha/needle.txt": true},
	}
	for id, want := range wantSets {
		rtest.Assert(t, setEquals(want, sets[id]),
			"snapshot %s: want matches %v, got %v", id, want, sets[id])
	}

	_, present := sets[snapshots[1].ID().Str()]
	rtest.Assert(t, !present, "the /beta snapshot must not appear in output: got %v", sets[snapshots[1].ID().Str()])

	// leafTree is mounted at two distinct paths, so the path-keyed walk loads
	// it once per path. A treeID-only merge would load it once.
	rtest.Equals(t, 2, countingRepo.LoadCount(leafTree),
		fmt.Sprintf("leafTree must be loaded once per distinct path (2), got %d", countingRepo.LoadCount(leafTree)))
}

// buildPrunableGroupSnapshots builds an N-snapshot fixture in which every
// snapshot shares the same root tree, so the whole run is a single group
// spanning all N snapshots. The shared /test subtree holds two children:
//
//	/test/a -> aTree (directory subtree holding filler.txt)
//	/test/b -> b file
//
// Against the ABSOLUTE pattern /test/b, /test/a cannot match (ChildMatch is
// false there), so aTree is pruned once for the entire group and never loaded,
// while /test/b is matched and attributed to every snapshot.
func buildPrunableGroupSnapshots(t testing.TB, repo restic.Repository, n int) ([]*data.Snapshot, restic.ID) {
	var (
		aTree    restic.ID
		testTree restic.ID
		rootTree restic.ID
	)

	err := repo.WithBlobUploader(context.TODO(), func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		aTree = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "filler.txt", Type: data.NodeTypeFile, Mode: 0644, Size: 1},
		})
		testTree = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "a", Type: data.NodeTypeDir, Mode: 0755, Subtree: &aTree},
			{Name: "b", Type: data.NodeTypeFile, Mode: 0644, Size: 1},
		})
		rootTree = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "test", Type: data.NodeTypeDir, Mode: 0755, Subtree: &testTree},
		})
		return nil
	})
	rtest.OK(t, err)

	base := time.Unix(1700000000, 0)
	snapshots := make([]*data.Snapshot, n)
	for i := range snapshots {
		snapshots[i] = saveSnapshotWithTree(t, repo, rootTree, base.Add(time.Duration(i)*time.Second))
	}
	return snapshots, aTree
}

// TestFindPatternInvertedPrune pins the absolute-pattern pruning contract of
// the inverted walk: an absolute pattern prunes a sibling subtree once for the
// whole snapshot group — the pruned subtree is never loaded — while the
// matched path is attributed to every snapshot in the group.
//
// Mutation check: if the childMayMatch prune in processGroup were disabled
// (the `if !childMayMatch { continue }` guard removed), /test/a would be
// bucketed and recursed into, loading aTree. LoadCount(aTree) would then be 1
// (one group), failing the LoadCount == 0 assertion. The match-attribution
// assertions additionally fail if the prune were accidentally applied to /test
// itself (the matched path /test/b would disappear).
func TestFindPatternInvertedPrune(t *testing.T) {
	repo := repository.TestRepository(t)
	const numSnapshots = 3
	snapshots, aTree := buildPrunableGroupSnapshots(t, repo, numSnapshots)

	countingRepo := &findCountingRepository{Repository: repo}
	order, sets := runPatternSearchPerSnapshot(t, countingRepo, snapshots, "/test/b")

	// /test/b is matched once for the shared /test subtree and attributed to
	// every snapshot in the group, in input order.
	wantOrder := make([]string, numSnapshots)
	wantSets := make(map[string]map[string]bool, numSnapshots)
	for i, sn := range snapshots {
		wantOrder[i] = sn.ID().Str()
		wantSets[sn.ID().Str()] = map[string]bool{"/test/b": true}
	}
	rtest.Equals(t, wantOrder, order, "every snapshot in the group must appear in output order")
	for id, want := range wantSets {
		rtest.Assert(t, setEquals(want, sets[id]),
			"snapshot %s: want matches %v, got %v", id, want, sets[id])
	}

	// The absolute pattern /test/b cannot match anything under /test/a, so
	// childMayMatch is false there and aTree is pruned once for the whole
	// group — never loaded. This is the only externally observable signal of
	// pruning (pruning never changes which paths are reported).
	rtest.Equals(t, 0, countingRepo.LoadCount(aTree),
		fmt.Sprintf("pruned subtree aTree must never be loaded, got LoadCount %d", countingRepo.LoadCount(aTree)))
}

// buildSharedSubtreeLoadFixture builds a three-snapshot fixture in which a
// subtree (sharedTree, holding needle.txt) is shared by two snapshots at the same
// path /shared, while a third snapshot holds an independent subtree at /other:
//
//	sharedTree -> needle.txt             (shared by sn1, sn2 at /shared)
//	otherTree  -> needle.txt            (sn3 only, at /other)
//
//	rootShared -> shared -> sharedTree
//	rootExtra  -> shared -> sharedTree
//	rootOther  -> other  -> otherTree
//
// It returns the snapshots plus sharedTree's ID so a test can target the shared
// subtree for an injected load failure.
func buildSharedSubtreeLoadFixture(t testing.TB, repo restic.Repository) ([]*data.Snapshot, restic.ID) {
	var (
		sharedTree restic.ID
		otherTree  restic.ID
		rootShared restic.ID
		rootExtra  restic.ID
		rootOther  restic.ID
	)

	err := repo.WithBlobUploader(context.TODO(), func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		sharedTree = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "needle.txt", Type: data.NodeTypeFile, Mode: 0644, Size: 1},
		})
		otherTree = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "needle.txt", Type: data.NodeTypeFile, Mode: 0644, Size: 2},
		})
		rootShared = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "shared", Type: data.NodeTypeDir, Mode: 0755, Subtree: &sharedTree},
		})
		rootExtra = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "shared", Type: data.NodeTypeDir, Mode: 0755, Subtree: &sharedTree},
		})
		rootOther = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "other", Type: data.NodeTypeDir, Mode: 0755, Subtree: &otherTree},
		})
		return nil
	})
	rtest.OK(t, err)

	base := time.Unix(1700000000, 0)
	snapshots := []*data.Snapshot{
		saveSnapshotWithTree(t, repo, rootShared, base),
		saveSnapshotWithTree(t, repo, rootExtra, base.Add(time.Second)),
		saveSnapshotWithTree(t, repo, rootOther, base.Add(2*time.Second)),
	}
	return snapshots, sharedTree
}

// TestFindPatternInvertedSoftLoadFailure pins the soft tree-load failure
// contract of the inverted walk: a failed load of a subtree shared by a group
// is logged and skipped, so the snapshots sharing that subtree miss its
// matches for this run, while sibling groups and other snapshots proceed
// unaffected. The walk does not abort, and no match is attributed to a snapshot
// that does not contain the failed subtree.
//
// Mutation check: if processGroup returned the load error instead of
// soft-skipping (the `continue` after the debug.Log/printer.S turned into
// `return err`), the whole walk would abort immediately. The sibling /other
// group would never be processed, so sn3 would never appear in the output and
// would lose its /other/needle.txt match — failing the no-abort and
// sibling-unaffected assertions below.
func TestFindPatternInvertedSoftLoadFailure(t *testing.T) {
	repo := repository.TestRepository(t)
	snapshots, sharedTree := buildSharedSubtreeLoadFixture(t, repo)

	// Inject one transient LoadBlob failure for the shared subtree. Because the
	// inverted walk loads sharedTree exactly once for the sn1+sn2 group, a
	// single failure is enough for both sharing snapshots to miss it; the load is
	// never retried within this run.
	failRepo := &findFailOnceRepository{Repository: repo, failID: sharedTree, failsLeft: 1}
	order, sets := runPatternSearchPerSnapshot(t, failRepo, snapshots, "needle.txt")

	// sn3 (the /other snapshot, not sharing the failed subtree) is unaffected:
	// it still appears in the output carrying its /other/needle.txt match.
	wantOrder := []string{snapshots[2].ID().Str()}
	rtest.Equals(t, wantOrder, order, "the unaffected sibling snapshot must still appear in output")

	wantOther := map[string]bool{"/other/needle.txt": true}
	rtest.Assert(t, setEquals(wantOther, sets[snapshots[2].ID().Str()]),
		"snapshot %s: sibling group must keep its match, want %v, got %v",
		snapshots[2].ID().Str(), wantOther, sets[snapshots[2].ID().Str()])

	// sn1 and sn2 share the failed sharedTree at /shared. They miss /shared for
	// this run, so neither appears in the output (they have no other matches)
	// and neither is attributed /other/needle.txt.
	for _, i := range []int{0, 1} {
		id := snapshots[i].ID().Str()
		_, present := sets[id]
		rtest.Assert(t, !present,
			"snapshot %s must not appear (its shared subtree failed to load), got %v", id, sets[id])
	}

	// Explicitly assert no match was attributed to a non-containing snapshot:
	// /shared/needle.txt was never loaded, so it must be absent from every set,
	// and /other/needle.txt must appear only for sn3.
	for id, s := range sets {
		rtest.Assert(t, !s["/shared/needle.txt"],
			"snapshot %s must not carry /shared/needle.txt (its subtree load failed): got %v", id, s)
	}
}

// findTestPrinter is a minimal statefulOutput printer that captures every S
// line. It is shared by the pattern behavior tests and the find benchmarks.
type findTestPrinter struct {
	mu    sync.Mutex
	lines []string
}

func (p *findTestPrinter) S(format string, args ...interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lines = append(p.lines, fmt.Sprintf(format, args...))
}

func (p *findTestPrinter) P(string, ...interface{}) {}
func (p *findTestPrinter) E(string, ...interface{}) {}

func (p *findTestPrinter) Lines() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.lines)
}

type findCountingRepository struct {
	restic.Repository
	mu        sync.Mutex
	treeLoads int
	perTree   map[restic.ID]int
}

func (r *findCountingRepository) LoadBlob(ctx context.Context, h restic.BlobHandle, buf []byte) ([]byte, error) {
	if h.Type == restic.TreeBlob {
		r.mu.Lock()
		r.treeLoads++
		if r.perTree == nil {
			r.perTree = make(map[restic.ID]int)
		}
		r.perTree[h.ID]++
		r.mu.Unlock()
	}
	return r.Repository.LoadBlob(ctx, h, buf)
}

func (r *findCountingRepository) TreeLoads() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.treeLoads
}

// LoadCount reports how many times the given tree blob was loaded. A count of
// zero proves the subtree was never visited, which is the only externally
// observable signal that a subtree was pruned (pruning never changes which
// paths are reported, so it cannot be detected from match output alone).
func (r *findCountingRepository) LoadCount(id restic.ID) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.perTree[id]
}

// findFailOnceRepository injects a bounded number of transient failures when a
// specific tree blob is loaded, then lets subsequent loads succeed. It models a
// backend hiccup (timeout, temporarily unavailable pack) that clears on a later
// attempt, which is what exercises the soft-skip branch of the inverted walk.
type findFailOnceRepository struct {
	restic.Repository
	mu        sync.Mutex
	failID    restic.ID
	failsLeft int
}

func (r *findFailOnceRepository) LoadBlob(ctx context.Context, h restic.BlobHandle, buf []byte) ([]byte, error) {
	r.mu.Lock()
	if h.Type == restic.TreeBlob && h.ID == r.failID && r.failsLeft > 0 {
		r.failsLeft--
		r.mu.Unlock()
		return nil, fmt.Errorf("injected transient load failure for tree %v", h.ID)
	}
	r.mu.Unlock()
	return r.Repository.LoadBlob(ctx, h, buf)
}

func newFindPatternTestFinder(repo restic.Repository, printer *findTestPrinter, patterns ...string) *Finder {
	return &Finder{
		repo: repo,
		pat: findPattern{
			pattern: patterns,
		},
		out: statefulOutput{
			printer: printer,
			stdout:  io.Discard,
		},
		printer: printer,
	}
}

func saveSnapshotWithTree(t testing.TB, repo restic.Repository, treeID restic.ID, at time.Time) *data.Snapshot {
	sn, err := data.NewSnapshot([]string{"/test"}, nil, "test-host", at)
	rtest.OK(t, err)

	sn.Tree = &treeID
	id, err := data.SaveSnapshot(context.TODO(), repo, sn)
	rtest.OK(t, err)
	data.TestSetSnapshotID(t, sn, id)
	return sn
}

// buildPrunableTree creates a tree shaped as:
//
//	/test/a/deep.txt   sibling an absolute "/test/b" pattern must prune
//	/test/b/leaf.txt   the directory the pattern targets
//
// It returns the root tree ID plus the two leaf subtree IDs so tests can assert
// which subtrees were or were not loaded. The two leaf subtrees hold different
// content so they receive distinct, individually observable tree IDs.
// runPatternSearchPerSnapshotWithPat is like runPatternSearchPerSnapshot but
// lets the caller supply the full findPattern (oldest/newest/ignoreCase) so
// tests can pin per-node filtering applied at match time across a group.
func runPatternSearchPerSnapshotWithPat(t testing.TB, repo restic.Repository, snapshots []*data.Snapshot, pat findPattern) ([]string, map[string]map[string]bool) {
	t.Helper()
	printer := newFindPerSnapshotPrinter()
	finder := &Finder{
		repo:    repo,
		pat:     pat,
		out:     statefulOutput{printer: printer, stdout: io.Discard},
		printer: printer,
	}
	rtest.OK(t, finder.findPatternInverted(context.Background(), snapshots))
	return printer.Order(), printer.Sets()
}

// buildSharedGroupFixture builds an N-snapshot fixture in which every snapshot
// shares the same root tree, so the whole run is a single group spanning all N
// snapshots. The shared tree holds two files at the root that BOTH match the
// pattern "needle.txt" case-insensitively and differ only in casing and
// modification time:
//
//	Needle.TXT   mixed-case name, ModTime == within (kept: name match via
//	             --ignore-case, and inside --oldest/--newest)
//	needle.txt   lowercase name, ModTime == before oldest (name matches the
//	             pattern directly, so only matchFindNodeTimeRange drops it)
//
// Returning a single shared root tree guarantees that the inverted walk
// processes every snapshot through one processGroup call, so any per-node
// filter (ignore-case, time range) is applied once and attributed to every
// snapshot in the group. Because needle.txt matches the pattern by name, the
// time-range check is the sole gate keeping it out of the output: dropping
// matchFindNodeTimeRange would let it leak. Dropping the ignore-case
// lowercasing would remove /Needle.TXT, the only in-range match.
func buildSharedGroupFixture(t testing.TB, repo restic.Repository, n int, within, before time.Time) []*data.Snapshot {
	var root restic.ID
	err := repo.WithBlobUploader(context.TODO(), func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		root = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "Needle.TXT", Type: data.NodeTypeFile, Mode: 0644, Size: 1, ModTime: within},
			{Name: "needle.txt", Type: data.NodeTypeFile, Mode: 0644, Size: 1, ModTime: before},
		})
		return nil
	})
	rtest.OK(t, err)

	base := time.Unix(1700000000, 0)
	snapshots := make([]*data.Snapshot, n)
	for i := range snapshots {
		snapshots[i] = saveSnapshotWithTree(t, repo, root, base.Add(time.Duration(i)*time.Second))
	}
	return snapshots
}

func buildPrunableTree(t testing.TB, repo restic.Repository) (root, aTree, bTree restic.ID) {
	err := repo.WithBlobUploader(context.TODO(), func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		aTree = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "deep.txt", Type: data.NodeTypeFile, Mode: 0644, Size: 1},
		})
		bTree = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "leaf.txt", Type: data.NodeTypeFile, Mode: 0644, Size: 2},
		})
		testTree := data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "a", Type: data.NodeTypeDir, Mode: 0755, Subtree: &aTree},
			{Name: "b", Type: data.NodeTypeDir, Mode: 0755, Subtree: &bTree},
		})
		root = data.TestSaveNodes(t, ctx, uploader, []*data.Node{
			{Name: "test", Type: data.NodeTypeDir, Mode: 0755, Subtree: &testTree},
		})
		return nil
	})
	rtest.OK(t, err)
	return root, aTree, bTree
}

// TestFindPatternInvertedFilters pins the per-node filtering contracts of the
// inverted walk across a multi-snapshot group: --ignore-case and
// --oldest/--newest (matchFindNodeTimeRange) must be honored identically for
// every snapshot in the group, because every snapshot shares one root tree and
// is therefore processed through a single processGroup call. The filters are
// applied at match time on the node, so a broken application (wrong field,
// skipped check, path lowercased for only some snapshots) surfaces as divergent
// per-snapshot match sets.
//
// Mutation checks:
//   - If matchFindPattern skipped the ignore-case lowercasing, the lowercase
//     pattern "needle.txt" would not match the mixed-case file /Needle.TXT; the
//     only remaining name match is /needle.txt, which is outside the time range,
//     so no snapshot would appear in the output.
//   - If matchFindNodeTimeRange were dropped, /needle.txt (name matches the
//     pattern, ModTime before --oldest) would leak into every snapshot's output
//     instead of being filtered out.
//   - If the time-range or ignore-case filter were applied to only one snapshot
//     in the group (rather than uniformly), the per-snapshot match sets would
//     diverge from the expected identical set.
func TestFindPatternInvertedFilters(t *testing.T) {
	const numSnapshots = 3

	// within is inside the --oldest/--newest window; before is older than
	// --oldest and must be filtered out by matchFindNodeTimeRange.
	within := time.Unix(1700001000, 0)
	before := time.Unix(1690000000, 0)
	oldest := time.Unix(1700000000, 0)
	newest := time.Unix(1700002000, 0)

	repo := repository.TestRepository(t)
	snapshots := buildSharedGroupFixture(t, repo, numSnapshots, within, before)

	// --ignore-case plus a lowercase pattern: the mixed-case /Needle.TXT is
	// lowercased to /needle.txt and matches; /needle.txt matches by name
	// directly. --oldest/--newest keeps /Needle.TXT (within range) and must drop
	// /needle.txt (before oldest). Both filters must apply identically to every
	// snapshot sharing the single root group.
	pat := findPattern{
		pattern:    []string{"needle.txt"},
		ignoreCase: true,
		oldest:     oldest,
		newest:     newest,
	}
	order, sets := runPatternSearchPerSnapshotWithPat(t, repo, snapshots, pat)

	wantOrder := make([]string, numSnapshots)
	wantSets := make(map[string]map[string]bool, numSnapshots)
	for i, sn := range snapshots {
		wantOrder[i] = sn.ID().Str()
		wantSets[sn.ID().Str()] = map[string]bool{"/Needle.TXT": true}
	}
	rtest.Equals(t, wantOrder, order, "every snapshot in the group must appear in output order")
	for id, want := range wantSets {
		rtest.Assert(t, setEquals(want, sets[id]),
			"snapshot %s: want matches %v, got %v", id, want, sets[id])
	}

	// /needle.txt matches the pattern by name but is outside the time range, so
	// matchFindNodeTimeRange must keep it out of every set.
	for id, s := range sets {
		rtest.Assert(t, !s["/needle.txt"],
			"snapshot %s must not carry /needle.txt (ModTime before --oldest): got %v", id, s)
	}

	// Without --ignore-case the same lowercase pattern must NOT match the
	// mixed-case file, so no snapshot appears. This pins that ignore-case is
	// actually the active knob: the match above was not an artifact of a
	// case-insensitive filesystem filepath.Match.
	strictPat := findPattern{
		pattern: []string{"needle.txt"},
		oldest:  oldest,
		newest:  newest,
	}
	strictOrder, _ := runPatternSearchPerSnapshotWithPat(t, repo, snapshots, strictPat)
	rtest.Assert(t, len(strictOrder) == 0,
		"without --ignore-case the lowercase pattern must match nothing, got order %v", strictOrder)
}
