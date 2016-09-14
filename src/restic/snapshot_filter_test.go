package restic_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"restic"
	"sort"
	"testing"
	"time"
)

func parseTimeUTC(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		panic(err)
	}

	return t.UTC()
}

var testFilterSnapshots = restic.Snapshots{
	{Hostname: "foo", Username: "testuser", Time: parseTimeUTC("2016-01-01 01:02:03"), Paths: []string{"/usr", "/bin"}, Tags: []string{"foo"}},
	{Hostname: "bar", Username: "testuser", Time: parseTimeUTC("2016-01-01 01:03:03"), Paths: []string{"/usr", "/bin"}, Tags: []string{"foo"}},
	{Hostname: "foo", Username: "testuser", Time: parseTimeUTC("2016-01-03 07:02:03"), Paths: []string{"/usr", "/bin"}, Tags: []string{"foo"}},
	{Hostname: "bar", Username: "testuser", Time: parseTimeUTC("2016-01-01 07:08:03"), Paths: []string{"/usr", "/bin"}, Tags: []string{"foo"}},
	{Hostname: "foo", Username: "testuser", Time: parseTimeUTC("2016-01-04 10:23:03"), Paths: []string{"/usr", "/bin"}, Tags: []string{"foo"}},
	{Hostname: "foo", Username: "testuser", Time: parseTimeUTC("2016-01-04 11:23:03"), Paths: []string{"/usr", "/bin"}, Tags: []string{"foo"}},
	{Hostname: "foo", Username: "testuser", Time: parseTimeUTC("2016-01-04 12:23:03"), Paths: []string{"/usr", "/bin"}, Tags: []string{"foo"}},
	{Hostname: "foo", Username: "testuser", Time: parseTimeUTC("2016-01-04 12:24:03"), Paths: []string{"/usr", "/bin"}, Tags: []string{"test"}},
	{Hostname: "foo", Username: "testuser", Time: parseTimeUTC("2016-01-04 12:28:03"), Paths: []string{"/usr", "/bin"}, Tags: []string{"test"}},
	{Hostname: "foo", Username: "testuser", Time: parseTimeUTC("2016-01-04 12:30:03"), Paths: []string{"/usr", "/bin"}, Tags: []string{"test", "foo", "bar"}},
	{Hostname: "foo", Username: "testuser", Time: parseTimeUTC("2016-01-04 16:23:03"), Paths: []string{"/usr", "/bin"}, Tags: []string{"test", "test2"}},
	{Hostname: "foo", Username: "testuser", Time: parseTimeUTC("2016-01-05 09:02:03"), Paths: []string{"/usr", "/bin"}, Tags: []string{"foo"}},
	{Hostname: "foo", Username: "testuser", Time: parseTimeUTC("2016-01-06 08:02:03"), Paths: []string{"/usr", "/bin"}, Tags: []string{"fox"}},
	{Hostname: "foo", Username: "testuser", Time: parseTimeUTC("2016-01-07 10:02:03"), Paths: []string{"/usr", "/bin"}, Tags: []string{"fox"}},
	{Hostname: "foo", Username: "root", Time: parseTimeUTC("2016-01-08 20:02:03"), Paths: []string{"/usr", "/sbin"}, Tags: []string{"foo"}},
	{Hostname: "foo", Username: "root", Time: parseTimeUTC("2016-01-09 21:02:03"), Paths: []string{"/usr", "/sbin"}, Tags: []string{"fox"}},
	{Hostname: "bar", Username: "root", Time: parseTimeUTC("2016-01-12 21:02:03"), Paths: []string{"/usr", "/sbin"}, Tags: []string{"foo"}},
	{Hostname: "foo", Username: "testuser", Time: parseTimeUTC("2016-01-12 21:08:03"), Paths: []string{"/usr", "/bin"}, Tags: []string{"bar"}},
	{Hostname: "foo", Username: "testuser", Time: parseTimeUTC("2016-01-18 12:02:03"), Paths: []string{"/usr", "/bin"}, Tags: []string{"bar"}},
}

var filterTests = []restic.SnapshotFilter{
	{Hostname: "foo"},
	{Username: "root"},
	{Hostname: "foo", Username: "root"},
	{Paths: []string{"/usr", "/bin"}},
	{Hostname: "bar", Paths: []string{"/usr", "/bin"}},
	{Hostname: "foo", Username: "root", Paths: []string{"/usr", "/sbin"}},
	{Tags: []string{"foo"}},
	{Tags: []string{"fox"}, Username: "root"},
	{Tags: []string{"foo", "test"}},
	{Tags: []string{"foo", "test2"}},
}

func TestFilterSnapshots(t *testing.T) {
	sort.Sort(testFilterSnapshots)

	for i, f := range filterTests {
		res := restic.FilterSnapshots(testFilterSnapshots, f)

		goldenFilename := filepath.Join("testdata", fmt.Sprintf("filter_snapshots_%d", i))

		if *updateGoldenFiles {
			buf, err := json.MarshalIndent(res, "", "  ")
			if err != nil {
				t.Fatalf("error marshaling result: %v", err)
			}

			if err = ioutil.WriteFile(goldenFilename, buf, 0644); err != nil {
				t.Fatalf("unable to update golden file: %v", err)
			}
		}

		buf, err := ioutil.ReadFile(goldenFilename)
		if err != nil {
			t.Errorf("error loading golden file %v: %v", goldenFilename, err)
			continue
		}

		var want restic.Snapshots
		err = json.Unmarshal(buf, &want)

		if !reflect.DeepEqual(res, want) {
			t.Errorf("test %v: wrong result, want:\n  %#v\ngot:\n  %#v", i, want, res)
			continue
		}
	}
}

var testExpireSnapshots = restic.Snapshots{
	{Time: parseTimeUTC("2014-09-01 10:20:30")},
	{Time: parseTimeUTC("2014-09-02 10:20:30")},
	{Time: parseTimeUTC("2014-09-05 10:20:30")},
	{Time: parseTimeUTC("2014-09-06 10:20:30")},
	{Time: parseTimeUTC("2014-09-08 10:20:30")},
	{Time: parseTimeUTC("2014-09-09 10:20:30")},
	{Time: parseTimeUTC("2014-09-10 10:20:30")},
	{Time: parseTimeUTC("2014-09-11 10:20:30")},
	{Time: parseTimeUTC("2014-09-20 10:20:30")},
	{Time: parseTimeUTC("2014-09-22 10:20:30")},
	{Time: parseTimeUTC("2014-08-08 10:20:30")},
	{Time: parseTimeUTC("2014-08-10 10:20:30")},
	{Time: parseTimeUTC("2014-08-12 10:20:30")},
	{Time: parseTimeUTC("2014-08-13 10:20:30")},
	{Time: parseTimeUTC("2014-08-13 10:20:30")},
	{Time: parseTimeUTC("2014-08-15 10:20:30")},
	{Time: parseTimeUTC("2014-08-18 10:20:30")},
	{Time: parseTimeUTC("2014-08-20 10:20:30")},
	{Time: parseTimeUTC("2014-08-21 10:20:30")},
	{Time: parseTimeUTC("2014-08-22 10:20:30")},
	{Time: parseTimeUTC("2014-10-01 10:20:30"), Tags: []string{"foo"}},
	{Time: parseTimeUTC("2014-10-02 10:20:30"), Tags: []string{"foo"}},
	{Time: parseTimeUTC("2014-10-05 10:20:30"), Tags: []string{"foo"}},
	{Time: parseTimeUTC("2014-10-06 10:20:30"), Tags: []string{"foo"}},
	{Time: parseTimeUTC("2014-10-08 10:20:30"), Tags: []string{"foo"}},
	{Time: parseTimeUTC("2014-10-09 10:20:30"), Tags: []string{"foo"}},
	{Time: parseTimeUTC("2014-10-10 10:20:30"), Tags: []string{"foo"}},
	{Time: parseTimeUTC("2014-10-11 10:20:30"), Tags: []string{"foo"}},
	{Time: parseTimeUTC("2014-10-20 10:20:30"), Tags: []string{"foo"}},
	{Time: parseTimeUTC("2014-10-22 10:20:30"), Tags: []string{"foo"}},
	{Time: parseTimeUTC("2014-11-08 10:20:30"), Tags: []string{"foo"}},
	{Time: parseTimeUTC("2014-11-10 10:20:30"), Tags: []string{"foo"}},
	{Time: parseTimeUTC("2014-11-12 10:20:30"), Tags: []string{"foo"}},
	{Time: parseTimeUTC("2014-11-13 10:20:30"), Tags: []string{"foo"}},
	{Time: parseTimeUTC("2014-11-13 10:20:30")},
	{Time: parseTimeUTC("2014-11-15 10:20:30")},
	{Time: parseTimeUTC("2014-11-18 10:20:30")},
	{Time: parseTimeUTC("2014-11-20 10:20:30")},
	{Time: parseTimeUTC("2014-11-21 10:20:30")},
	{Time: parseTimeUTC("2014-11-22 10:20:30")},
	{Time: parseTimeUTC("2015-09-01 10:20:30")},
	{Time: parseTimeUTC("2015-09-02 10:20:30")},
	{Time: parseTimeUTC("2015-09-05 10:20:30")},
	{Time: parseTimeUTC("2015-09-06 10:20:30")},
	{Time: parseTimeUTC("2015-09-08 10:20:30")},
	{Time: parseTimeUTC("2015-09-09 10:20:30")},
	{Time: parseTimeUTC("2015-09-10 10:20:30")},
	{Time: parseTimeUTC("2015-09-11 10:20:30")},
	{Time: parseTimeUTC("2015-09-20 10:20:30")},
	{Time: parseTimeUTC("2015-09-22 10:20:30")},
	{Time: parseTimeUTC("2015-08-08 10:20:30")},
	{Time: parseTimeUTC("2015-08-10 10:20:30")},
	{Time: parseTimeUTC("2015-08-12 10:20:30")},
	{Time: parseTimeUTC("2015-08-13 10:20:30")},
	{Time: parseTimeUTC("2015-08-13 10:20:30")},
	{Time: parseTimeUTC("2015-08-15 10:20:30")},
	{Time: parseTimeUTC("2015-08-18 10:20:30")},
	{Time: parseTimeUTC("2015-08-20 10:20:30")},
	{Time: parseTimeUTC("2015-08-21 10:20:30")},
	{Time: parseTimeUTC("2015-08-22 10:20:30")},
	{Time: parseTimeUTC("2015-10-01 10:20:30")},
	{Time: parseTimeUTC("2015-10-02 10:20:30")},
	{Time: parseTimeUTC("2015-10-05 10:20:30")},
	{Time: parseTimeUTC("2015-10-06 10:20:30")},
	{Time: parseTimeUTC("2015-10-08 10:20:30")},
	{Time: parseTimeUTC("2015-10-09 10:20:30")},
	{Time: parseTimeUTC("2015-10-10 10:20:30")},
	{Time: parseTimeUTC("2015-10-11 10:20:30")},
	{Time: parseTimeUTC("2015-10-20 10:20:30")},
	{Time: parseTimeUTC("2015-10-22 10:20:30")},
	{Time: parseTimeUTC("2015-11-08 10:20:30")},
	{Time: parseTimeUTC("2015-11-10 10:20:30")},
	{Time: parseTimeUTC("2015-11-12 10:20:30")},
	{Time: parseTimeUTC("2015-11-13 10:20:30")},
	{Time: parseTimeUTC("2015-11-13 10:20:30")},
	{Time: parseTimeUTC("2015-11-15 10:20:30")},
	{Time: parseTimeUTC("2015-11-18 10:20:30")},
	{Time: parseTimeUTC("2015-11-20 10:20:30")},
	{Time: parseTimeUTC("2015-11-21 10:20:30")},
	{Time: parseTimeUTC("2015-11-22 10:20:30")},
	{Time: parseTimeUTC("2016-01-01 01:02:03")},
	{Time: parseTimeUTC("2016-01-01 01:03:03")},
	{Time: parseTimeUTC("2016-01-01 07:08:03")},
	{Time: parseTimeUTC("2016-01-03 07:02:03")},
	{Time: parseTimeUTC("2016-01-04 10:23:03")},
	{Time: parseTimeUTC("2016-01-04 11:23:03")},
	{Time: parseTimeUTC("2016-01-04 12:23:03")},
	{Time: parseTimeUTC("2016-01-04 12:24:03")},
	{Time: parseTimeUTC("2016-01-04 12:28:03")},
	{Time: parseTimeUTC("2016-01-04 12:30:03")},
	{Time: parseTimeUTC("2016-01-04 16:23:03")},
	{Time: parseTimeUTC("2016-01-05 09:02:03")},
	{Time: parseTimeUTC("2016-01-06 08:02:03")},
	{Time: parseTimeUTC("2016-01-07 10:02:03")},
	{Time: parseTimeUTC("2016-01-08 20:02:03")},
	{Time: parseTimeUTC("2016-01-09 21:02:03")},
	{Time: parseTimeUTC("2016-01-12 21:02:03")},
	{Time: parseTimeUTC("2016-01-12 21:08:03")},
	{Time: parseTimeUTC("2016-01-18 12:02:03")},
}

var expireTests = []restic.ExpirePolicy{
	{},
	{Last: 10},
	{Last: 15},
	{Last: 99},
	{Last: 200},
	{Hourly: 20},
	{Daily: 3},
	{Daily: 10},
	{Daily: 30},
	{Last: 5, Daily: 5},
	{Last: 2, Daily: 10},
	{Weekly: 2},
	{Weekly: 4},
	{Daily: 3, Weekly: 4},
	{Monthly: 6},
	{Daily: 2, Weekly: 2, Monthly: 6},
	{Yearly: 10},
	{Daily: 7, Weekly: 2, Monthly: 3, Yearly: 10},
	{Tags: []string{"foo"}},
}

func TestApplyPolicy(t *testing.T) {
	for i, p := range expireTests {
		keep, remove := restic.ApplyPolicy(testExpireSnapshots, p)

		t.Logf("test %d: returned keep %v, remove %v (of %v) expired snapshots for policy %v",
			i, len(keep), len(remove), len(testExpireSnapshots), p)

		if len(keep)+len(remove) != len(testExpireSnapshots) {
			t.Errorf("test %d: len(keep)+len(remove) = %d != len(testExpireSnapshots) = %d",
				i, len(keep)+len(remove), len(testExpireSnapshots))
		}

		if p.Sum() > 0 && len(keep) > p.Sum() {
			t.Errorf("not enough snapshots removed: policy allows %v snapshots to remain, but ended up with %v",
				p.Sum(), len(keep))
		}

		for _, sn := range keep {
			t.Logf("test %d:     keep snapshot at %v %s\n", i, sn.Time, sn.Tags)
		}
		for _, sn := range remove {
			t.Logf("test %d:   forget snapshot at %v %s\n", i, sn.Time, sn.Tags)
		}

		goldenFilename := filepath.Join("testdata", fmt.Sprintf("policy_keep_snapshots_%d", i))

		if *updateGoldenFiles {
			buf, err := json.MarshalIndent(keep, "", "  ")
			if err != nil {
				t.Fatalf("error marshaling result: %v", err)
			}

			if err = ioutil.WriteFile(goldenFilename, buf, 0644); err != nil {
				t.Fatalf("unable to update golden file: %v", err)
			}
		}

		buf, err := ioutil.ReadFile(goldenFilename)
		if err != nil {
			t.Errorf("error loading golden file %v: %v", goldenFilename, err)
			continue
		}

		var want restic.Snapshots
		err = json.Unmarshal(buf, &want)

		if !reflect.DeepEqual(keep, want) {
			t.Errorf("test %v: wrong result, want:\n  %v\ngot:\n  %v", i, want, keep)
			continue
		}
	}
}
