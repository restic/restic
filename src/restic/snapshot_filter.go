package restic

import (
	"fmt"
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

// SnapshotFilter configures criteria for filtering snapshots before an
// ExpirePolicy can be applied.
type SnapshotFilter struct {
	Hostname string
	Username string
	Paths    []string
	Tags     []string
}

// FilterSnapshots returns the snapshots from s which match the filter f.
func FilterSnapshots(s Snapshots, f SnapshotFilter) (result Snapshots) {
	for _, snap := range s {
		if f.Hostname != "" && f.Hostname != snap.Hostname {
			continue
		}

		if f.Username != "" && f.Username != snap.Username {
			continue
		}

		if f.Paths != nil && !reflect.DeepEqual(f.Paths, snap.Paths) {
			continue
		}

		if !snap.HasTags(f.Tags) {
			continue
		}

		result = append(result, snap)
	}

	return result
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

// filter is used to split a list of snapshots into those to keep and those to
// remove according to a policy.
type filter struct {
	Unprocessed Snapshots
	Remove      Snapshots
	Keep        Snapshots
}

func (f filter) String() string {
	return fmt.Sprintf("<filter %d todo, %d keep, %d remove>", len(f.Unprocessed), len(f.Keep), len(f.Remove))
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

// apply moves snapshots from Unprocess to either Keep or Remove. It sorts the
// snapshots into buckets according to the return of fn, and then moves the
// newest snapshot in each bucket to Keep and all others to Remove. When max
// snapshots were found, processing stops.
func (f *filter) apply(fn func(time.Time) int, max int) {
	if max == 0 || len(f.Unprocessed) == 0 {
		return
	}

	sameDay := Snapshots{}
	lastDay := fn(f.Unprocessed[0].Time)

	for len(f.Unprocessed) > 0 {
		cur := f.Unprocessed[0]

		day := fn(cur.Time)

		// if the snapshots are from a new day, forget all but the first (=last
		// in time) snapshot from the previous day.
		if day != lastDay {
			f.Keep = append(f.Keep, sameDay[0])
			for _, snapshot := range sameDay[1:] {
				f.Remove = append(f.Remove, snapshot)
			}

			sameDay = Snapshots{}
			lastDay = day
			max--

			if max == 0 {
				break
			}
		}

		// collect all snapshots for the current day
		sameDay = append(sameDay, cur)
		f.Unprocessed = f.Unprocessed[1:]
	}

	if len(sameDay) > 0 {
		f.Keep = append(f.Keep, sameDay[0])
		for _, snapshot := range sameDay[1:] {
			f.Remove = append(f.Remove, snapshot)
		}
	}
}

// keepTags marks the snapshots which have all tags as to be kept.
func (f *filter) keepTags(tags []string) {
	if len(tags) == 0 {
		return
	}

	unprocessed := f.Unprocessed[:0]
	for _, sn := range f.Unprocessed {
		if sn.HasTags(tags) {
			f.Keep = append(f.Keep, sn)
			continue
		}
		unprocessed = append(unprocessed, sn)
	}
	f.Unprocessed = unprocessed
}

// keepLast marks the last n snapshots as to be kept.
func (f *filter) keepLast(n int) {
	if n > len(f.Unprocessed) {
		n = len(f.Unprocessed)
	}

	f.Keep = append(f.Keep, f.Unprocessed[:n]...)
	f.Unprocessed = f.Unprocessed[n:]
}

// finish moves all remaining snapshots to remove.
func (f *filter) finish() {
	f.Remove = append(f.Remove, f.Unprocessed...)
}

// ApplyPolicy runs returns the snapshots from s that are to be deleted according
// to the policy p. s is sorted in the process.
func ApplyPolicy(list Snapshots, p ExpirePolicy) (keep, remove Snapshots) {
	sort.Sort(list)

	if p.Empty() {
		return list, remove
	}

	if len(list) == 0 {
		return list, remove
	}

	f := filter{
		Unprocessed: list,
		Remove:      Snapshots{},
		Keep:        Snapshots{},
	}

	f.keepTags(p.Tags)
	f.keepLast(p.Last)
	f.apply(ymdh, p.Hourly)
	f.apply(ymd, p.Daily)
	f.apply(yw, p.Weekly)
	f.apply(ym, p.Monthly)
	f.apply(y, p.Yearly)
	f.finish()

	return f.Keep, f.Remove
}
