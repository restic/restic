package fuse

import (
	"fmt"
	"time"

	"github.com/restic/restic/internal/restic"
	"golang.org/x/net/context"
)

type SnapshotManager struct {
	ctx                context.Context
	repo               restic.Repository
	config             Config
	lastSnapshotUpdate time.Time
	snapshots          restic.Snapshots
	snapshotByName     map[string]*restic.Snapshot
	snapshotNameLatest string
}

func NewSnapshotManager(
	ctx context.Context, repo restic.Repository, config Config,
) *SnapshotManager {
	return &SnapshotManager{ctx: ctx, repo: repo, config: config}
}

// update snapshots if repository has changed
func (self *SnapshotManager) updateSnapshots() error {
	if time.Since(self.lastSnapshotUpdate) < minSnapshotsReloadTime {
		return nil
	}

	snapshots, err := restic.FindFilteredSnapshots(
		self.ctx, self.repo,
		self.config.Hosts, self.config.Tags, self.config.Paths,
	)
	if err != nil {
		return err
	}

	if len(self.snapshots) != len(snapshots) {
		self.repo.LoadIndex(self.ctx)
		self.snapshots = snapshots
	}

	self.updateSnapshotNames("", "", self.config.SnapshotTemplate)

	self.lastSnapshotUpdate = time.Now()

	return nil
}

// read snapshot timestamps from the current repository-state.
func (self *SnapshotManager) updateSnapshotNames(tag, host, template string) {

	var latestTime time.Time

	self.snapshotNameLatest = ""
	self.snapshotByName = make(map[string]*restic.Snapshot, len(self.snapshots))
	self.snapshotNameLatest = ""

	for _, sn := range self.snapshots {

		tagMatches := tag == "" || isElem(tag, sn.Tags)
		hostMatches := host == "" || host == sn.Hostname

		if tagMatches && hostMatches {

			name := sn.Time.Format(template)

			if self.snapshotNameLatest == "" || !sn.Time.Before(latestTime) {
				latestTime = sn.Time
				self.snapshotNameLatest = name
			}

			for i := 1; ; i++ {
				if _, ok := self.snapshotByName[name]; !ok {
					break
				}

				name = fmt.Sprintf("%s-%d", sn.Time.Format(template), i)
			}

			self.snapshotByName[name] = sn
		}
	}
}
