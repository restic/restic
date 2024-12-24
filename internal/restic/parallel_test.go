package restic

import (
	"context"
	"testing"
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/ui/progress"
)

type mockRemoverUnpacked struct {
	removeUnpacked func(ctx context.Context, t FileType, id ID) error
}

func (m *mockRemoverUnpacked) Connections() uint {
	return 2
}

func (m *mockRemoverUnpacked) RemoveUnpacked(ctx context.Context, t FileType, id ID) error {
	return m.removeUnpacked(ctx, t, id)
}

func NewTestID(i byte) ID {
	return Hash([]byte{i})
}

func TestParallelRemove(t *testing.T) {
	ctx := context.Background()

	fileType := SnapshotFile // this can be any FileType

	tests := []struct {
		name            string
		removeUnpacked  func(ctx context.Context, t FileType, id ID) error
		fileList        IDSet
		wantRemoved     IDSet
		wantReportIDSet IDSet
		wantBarCount    int
	}{
		{
			name: "remove files",
			removeUnpacked: func(ctx context.Context, t FileType, id ID) error {
				return nil
			},
			fileList:        NewIDSet(NewTestID(1), NewTestID(2), NewTestID(3)),
			wantRemoved:     NewIDSet(NewTestID(1), NewTestID(2), NewTestID(3)),
			wantReportIDSet: NewIDSet(NewTestID(1), NewTestID(2), NewTestID(3)),
			wantBarCount:    3,
		},
		{
			name: "remove files with error",
			removeUnpacked: func(ctx context.Context, t FileType, id ID) error {
				return errors.New("error")
			},
			fileList:        NewIDSet(NewTestID(1), NewTestID(2), NewTestID(3)),
			wantRemoved:     NewIDSet(),
			wantReportIDSet: NewIDSet(),
			wantBarCount:    0,
		},
		{
			name: "fail 2 files",
			removeUnpacked: func(ctx context.Context, t FileType, id ID) error {
				if id == NewTestID(2) {
					return errors.New("error")
				}
				if id == NewTestID(3) {
					return errors.New("error")
				}
				return nil
			},
			fileList:        NewIDSet(NewTestID(1), NewTestID(2), NewTestID(3), NewTestID(4)),
			wantRemoved:     NewIDSet(NewTestID(1), NewTestID(4)),
			wantReportIDSet: NewIDSet(NewTestID(1), NewTestID(4)),
			wantBarCount:    2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &mockRemoverUnpacked{removeUnpacked: test.removeUnpacked}
			bar := progress.NewCounter(time.Millisecond, 0, func(value uint64, total uint64, runtime time.Duration, final bool) {})
			reportIDSet := NewIDSet()
			report := func(id ID, err error) error {
				if err == nil {
					bar.Add(1)
					reportIDSet.Insert(id)
				}
				return nil
			}
			_ = ParallelRemove(ctx, repo, test.fileList, fileType, report)
			barCount, _ := bar.Get()
			if barCount != uint64(test.wantBarCount) {
				t.Errorf("ParallelRemove() barCount = %d, want %d", barCount, test.wantBarCount)
			}
			if !reportIDSet.Equals(test.wantReportIDSet) {
				t.Errorf("ParallelRemove() reportIDSet = %v, want %v", reportIDSet, test.wantReportIDSet)
			}
		})
	}
}
