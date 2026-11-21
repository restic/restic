package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/global"
	rtest "github.com/restic/restic/internal/test"
)

func TestCheckFutureSnapshots(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	testRunInit(t, env.gopts)
	rtest.OK(t, os.WriteFile(filepath.Join(env.testdata, "file"), []byte("data"), 0600))
	for _, backup := range []struct {
		host string
		time time.Time
	}{
		{"past", time.Now().Add(-24 * time.Hour)},
		{"present", time.Now()},
		{"future", time.Now().Add(24 * time.Hour)},
	} {
		testRunBackup(t, env.testdata, []string{"file"}, BackupOptions{
			Host: backup.host, TimeStamp: backup.time.Format(global.TimeFormat),
		}, env.gopts)
	}
	newest, snapshots := testRunSnapshots(t, env.gopts)
	var pastID string
	for id, sn := range snapshots {
		if sn.Hostname == "past" {
			pastID = id.String()
		}
	}

	for _, jsonOutput := range []bool{false, true} {
		for _, tc := range []struct {
			name, host string
			args       []string
			warning    bool
		}{
			{name: "all", warning: true},
			{name: "past", host: "past"},
			{name: "present", host: "present"},
			{name: "future", host: "future", warning: true},
			{name: "past ID", args: []string{pastID}},
			{name: "future ID", args: []string{newest.ID.String()}, warning: true},
		} {
			t.Run(fmt.Sprintf("%s/json=%t", tc.name, jsonOutput), func(t *testing.T) {
				gopts := env.gopts
				gopts.JSON = jsonOutput
				var summary checkSummary
				stdout, stderr, err := withCaptureStdoutStderr(t, gopts, func(ctx context.Context, gopts global.Options) error {
					if gopts.JSON {
						cmd := newCheckCommand(&gopts)
						args := append([]string{}, tc.args...)
						if tc.host != "" {
							args = append(args, "--host", tc.host)
						}
						cmd.SetArgs(args)
						return cmd.ExecuteContext(ctx)
					}
					opts := CheckOptions{}
					if tc.host != "" {
						opts.SnapshotFilter = data.SnapshotFilter{Hosts: []string{tc.host}}
					}
					var err error
					summary, err = runCheck(ctx, opts, gopts, tc.args, gopts.Term)
					return err
				})
				rtest.OK(t, err)
				if jsonOutput {
					rtest.OK(t, json.Unmarshal(stdout.Bytes(), &summary))
					rtest.Equals(t, "summary", summary.MessageType)
				}
				rtest.Equals(t, 0, summary.NumErrors)
				rtest.Assert(t, len(summary.BrokenPacks) == 0, "unexpected damaged packs")
				rtest.Equals(t, tc.warning, strings.Contains(stderr.String(), "has a date in the future"))
				if jsonOutput && tc.warning {
					var warning checkError
					rtest.OK(t, json.Unmarshal(stderr.Bytes(), &warning))
					rtest.Equals(t, "warning", warning.MessageType)
				}
			})
		}
	}

	// A bad snapshot must still fail even when another snapshot only has a future date.
	rtest.OK(t, os.WriteFile(filepath.Join(env.repo, "snapshots", strings.Repeat("a", 64)), []byte("damaged"), 0600))
	_, stderr, err := testRunCheckOutput(t, env.gopts, false)
	rtest.Assert(t, err != nil, "damaged snapshot should fail check")
	rtest.Assert(t, strings.Contains(stderr, "has a date in the future"), "missing warning: %s", stderr)
	rtest.Assert(t, strings.Contains(stderr, "snapshot error"), "missing snapshot error: %s", stderr)
}
