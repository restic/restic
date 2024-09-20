package restic

import (
	"context"
	"time"

	"github.com/restic/restic/internal/restic"
)

type Snapshot struct {
	paths, tags []string
	hostname    string
	time        time.Time
}

type Filter struct {
	ctx                context.Context
	be                 restic.Lister
	loader             restic.LoaderUnpacked
	hosts, snapshotIDs []string
	tags               restic.TagList
	paths              []restic.TagList
	cb                 restic.SnapshotFindCb
}

func New(s *Snapshot) (*restic.Snapshot, error) {
	return restic.NewSnapshot(s.paths, s.tags, s.hostname, s.time)
}

func Find(f *Filter) {
	restic.FindFilteredSnapshots(f.ctx, f.be, f.loader, f.hosts, f.paths, f.tags, f.snapshotIDs, f.cb)
}
