package restic

import (
	"reflect"
	"sort"
	"time"
)

// Snapshots is a list of snapshots.
type Snapshots []*Snapshot

// Len returns the number of snapshots in sn.
func (sn Snapshots) Len() int {
	return len(sn)
}

// Less returns true iff the ith snapshot has been made after the jth.
func (sn Snapshots) Less(i, j int) bool {
	return sn[i].Time.After(sn[j].Time)
}

// Swap exchanges the two snapshots.
func (sn Snapshots) Swap(i, j int) {
	sn[i], sn[j] = sn[j], sn[i]
}

// ExpirePolicy configures which snapshots should be automatically removed.
type ExpirePolicy struct {
	Last    int      // keep the last n snapshots
	Hourly  int      // keep the last n hourly snapshots
	Daily   int      // keep the last n daily snapshots
	Weekly  int      // keep the last n weekly snapshots
	Monthly int      // keep the last n monthly snapshots
	Yearly  int      // keep the last n yearly snapshots
	Tags    []string // keep all snapshots with these tags
}

// Sum returns the maximum number of snapshots to be kept according to this
// policy.
func (e ExpirePolicy) Sum() int {
	return e.Last + e.Hourly + e.Daily + e.Weekly + e.Monthly + e.Yearly
}

// Empty returns true iff no policy has been configured (all values zero).
func (e ExpirePolicy) Empty() bool {
	if len(e.Tags) != 0 {
		return false
	}

	empty := ExpirePolicy{Tags: e.Tags}
	return reflect.DeepEqual(e, empty)
}

// ymdh returns an integer in the form YYYYMMDDHH.
func ymdh(d time.Time) int {
	return d.Year()*1000000 + int(d.Month())*10000 + d.Day()*100 + d.Hour()
}

// ymd returns an integer in the form YYYYMMDD.
func ymd(d time.Time) int {
	return d.Year()*10000 + int(d.Month())*100 + d.Day()
}

// yw returns an integer in the form YYYYWW, where WW is the week number.
func yw(d time.Time) int {
	year, week := d.ISOWeek()
	return year*100 + week
}

// ym returns an integer in the form YYYYMM.
func ym(d time.Time) int {
	return d.Year()*100 + int(d.Month())
}

// y returns the year of d.
func y(d time.Time) int {
	return d.Year()
}

var a int

// always retuns a unique number for d.
func always(d time.Time) int {
	a++
	return a
}

// ApplyPolicy returns the snapshots from list that are to be kept and removed
// according to the policy p. list is sorted in the process.
func ApplyPolicy(list Snapshots, p ExpirePolicy) (keep, remove Snapshots) {
	sort.Sort(list)

	if p.Empty() {
		return list, remove
	}

	if len(list) == 0 {
		return list, remove
	}

	var buckets = [6]struct {
		Count  int
		bucker func(d time.Time) int
		Last   int
	}{
		{p.Last, always, -1},
		{p.Hourly, ymdh, -1},
		{p.Daily, ymd, -1},
		{p.Weekly, yw, -1},
		{p.Monthly, ym, -1},
		{p.Yearly, y, -1},
	}

	for _, cur := range list {
		var keep_snap bool

		// Tags are handled specially as they are not counted.
		if len(p.Tags) > 0 {
			if cur.HasTags(p.Tags) {
				keep_snap = true
			}
		}
		// Now update the other buckets and see if they have some counts left.
		for i, b := range buckets {
			if b.Count > 0 {
				val := b.bucker(cur.Time)
				if val != b.Last {
					keep_snap = true
					buckets[i].Last = val
					buckets[i].Count--
				}
			}
		}

		if keep_snap {
			keep = append(keep, cur)
		} else {
			remove = append(remove, cur)
		}
	}

	return keep, remove
}
