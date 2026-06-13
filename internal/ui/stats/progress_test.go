package stats

import (
	"testing"
	"time"

	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui"
)

func TestStatsProgress(t *testing.T) {
	term := &ui.MockTerminal{}

	progress := newProgress(term, true, 2)
	progress.printProgress(0*time.Second, false)
	rtest.Equals(t, []string{"[0:00] 0.00%  0 / 2 snapshots, 0 B"}, term.Output)

	progress.ProcessSnapshot()
	progress.Update(1, 2, 3)
	progress.printProgress(5*time.Second, false)
	// Output differs from the previous one because the progress is based on the number of processed snapshots,
	// 1/2 snapshots means processing the snapshot 1 currently
	rtest.Equals(t, []string{"[0:05] 0.00%  1 / 2 snapshots, 1 files, 2 blobs, 3 B"}, term.Output)

	progress.ProcessSnapshot()
	progress.printProgress(10*time.Second, false)
	rtest.Equals(t, []string{"[0:10] 50.00%  2 / 2 snapshots, 0 B"}, term.Output)

	progress.Update(4, 5, 6)
	progress.printProgress(15*time.Second, false)
	rtest.Equals(t, []string{"[0:15] 50.00%  2 / 2 snapshots, 4 files, 5 blobs, 6 B"}, term.Output)

	progress.printProgress(20*time.Second, true)
	rtest.Equals(t, []string{"[0:20] 100.00%  2 / 2 snapshots, 4 files, 5 blobs, 6 B"}, term.Output)
}

func TestStatsProgressJSON(t *testing.T) {
	term := &ui.MockTerminal{}

	progress := newProgress(term, false, 2)
	progress.printProgress(0*time.Second, false)
	// JSON output is not available yet, so just make sure to not break normal json output
	rtest.Equals(t, nil, term.Output)
}
