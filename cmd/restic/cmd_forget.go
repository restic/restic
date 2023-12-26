package main

import (
	"context"
	"encoding/json"
	"io"
	"strconv"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/spf13/cobra"
)

var cmdForget = &cobra.Command{
	Use:   "forget [flags] [snapshot ID] [...]",
	Short: "Remove snapshots from the repository",
	Long: `
The "forget" command removes snapshots according to a policy. All snapshots are
first divided into groups according to "--group-by", and after that the policy
specified by the "--keep-*" options is applied to each group individually.

Please note that this command really only deletes the snapshot object in the
repository, which is a reference to data stored there. In order to remove the
unreferenced data after "forget" was run successfully, see the "prune" command.

Please also read the documentation for "forget" to learn about some important
security considerations.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runForget(cmd.Context(), forgetOptions, globalOptions, args)
	},
}

type ForgetPolicyCount int

var ErrNegativePolicyCount = errors.New("negative values not allowed, use 'unlimited' instead")

func (c *ForgetPolicyCount) Set(s string) error {
	switch s {
	case "unlimited":
		*c = -1
	default:
		val, err := strconv.ParseInt(s, 10, 0)
		if err != nil {
			return err
		}
		if val < 0 {
			return ErrNegativePolicyCount
		}
		*c = ForgetPolicyCount(val)
	}

	return nil
}

func (c *ForgetPolicyCount) String() string {
	switch *c {
	case -1:
		return "unlimited"
	default:
		return strconv.FormatInt(int64(*c), 10)
	}
}

func (c *ForgetPolicyCount) Type() string {
	return "n"
}

// ForgetOptions collects all options for the forget command.
type ForgetOptions struct {
	Last          ForgetPolicyCount
	Hourly        ForgetPolicyCount
	Daily         ForgetPolicyCount
	Weekly        ForgetPolicyCount
	Monthly       ForgetPolicyCount
	Yearly        ForgetPolicyCount
	Within        restic.Duration
	WithinHourly  restic.Duration
	WithinDaily   restic.Duration
	WithinWeekly  restic.Duration
	WithinMonthly restic.Duration
	WithinYearly  restic.Duration
	KeepTags      restic.TagLists

	restic.SnapshotFilter
	Compact bool

	// Grouping
	GroupBy restic.SnapshotGroupByOptions
	DryRun  bool
	Prune   bool
}

var forgetOptions ForgetOptions

func init() {
	cmdRoot.AddCommand(cmdForget)

	f := cmdForget.Flags()
	f.VarP(&forgetOptions.Last, "keep-last", "l", "keep the last `n` snapshots (use 'unlimited' to keep all snapshots)")
	f.VarP(&forgetOptions.Hourly, "keep-hourly", "H", "keep the last `n` hourly snapshots (use 'unlimited' to keep all hourly snapshots)")
	f.VarP(&forgetOptions.Daily, "keep-daily", "d", "keep the last `n` daily snapshots (use 'unlimited' to keep all daily snapshots)")
	f.VarP(&forgetOptions.Weekly, "keep-weekly", "w", "keep the last `n` weekly snapshots (use 'unlimited' to keep all weekly snapshots)")
	f.VarP(&forgetOptions.Monthly, "keep-monthly", "m", "keep the last `n` monthly snapshots (use 'unlimited' to keep all monthly snapshots)")
	f.VarP(&forgetOptions.Yearly, "keep-yearly", "y", "keep the last `n` yearly snapshots (use 'unlimited' to keep all yearly snapshots)")
	f.VarP(&forgetOptions.Within, "keep-within", "", "keep snapshots that are newer than `duration` (eg. 1y5m7d2h) relative to the latest snapshot")
	f.VarP(&forgetOptions.WithinHourly, "keep-within-hourly", "", "keep hourly snapshots that are newer than `duration` (eg. 1y5m7d2h) relative to the latest snapshot")
	f.VarP(&forgetOptions.WithinDaily, "keep-within-daily", "", "keep daily snapshots that are newer than `duration` (eg. 1y5m7d2h) relative to the latest snapshot")
	f.VarP(&forgetOptions.WithinWeekly, "keep-within-weekly", "", "keep weekly snapshots that are newer than `duration` (eg. 1y5m7d2h) relative to the latest snapshot")
	f.VarP(&forgetOptions.WithinMonthly, "keep-within-monthly", "", "keep monthly snapshots that are newer than `duration` (eg. 1y5m7d2h) relative to the latest snapshot")
	f.VarP(&forgetOptions.WithinYearly, "keep-within-yearly", "", "keep yearly snapshots that are newer than `duration` (eg. 1y5m7d2h) relative to the latest snapshot")
	f.Var(&forgetOptions.KeepTags, "keep-tag", "keep snapshots with this `taglist` (can be specified multiple times)")

	initMultiSnapshotFilter(f, &forgetOptions.SnapshotFilter, false)
	f.StringArrayVar(&forgetOptions.Hosts, "hostname", nil, "only consider snapshots with the given `hostname` (can be specified multiple times)")
	err := f.MarkDeprecated("hostname", "use --host")
	if err != nil {
		// MarkDeprecated only returns an error when the flag is not found
		panic(err)
	}

	f.BoolVarP(&forgetOptions.Compact, "compact", "c", false, "use compact output format")
	forgetOptions.GroupBy = restic.SnapshotGroupByOptions{Host: true, Path: true}
	f.VarP(&forgetOptions.GroupBy, "group-by", "g", "`group` snapshots by host, paths and/or tags, separated by comma (disable grouping with '')")
	f.BoolVarP(&forgetOptions.DryRun, "dry-run", "n", false, "do not delete anything, just print what would be done")
	f.BoolVar(&forgetOptions.Prune, "prune", false, "automatically run the 'prune' command if snapshots have been removed")

	f.SortFlags = false
	addPruneOptions(cmdForget)
}

func verifyForgetOptions(opts *ForgetOptions) error {
	if opts.Last < -1 || opts.Hourly < -1 || opts.Daily < -1 || opts.Weekly < -1 ||
		opts.Monthly < -1 || opts.Yearly < -1 {
		return errors.Fatal("negative values other than -1 are not allowed for --keep-*")
	}

	for _, d := range []restic.Duration{opts.Within, opts.WithinHourly, opts.WithinDaily,
		opts.WithinMonthly, opts.WithinWeekly, opts.WithinYearly} {
		if d.Hours < 0 || d.Days < 0 || d.Months < 0 || d.Years < 0 {
			return errors.Fatal("durations containing negative values are not allowed for --keep-within*")
		}
	}

	return nil
}

func runForget(ctx context.Context, opts ForgetOptions, gopts GlobalOptions, args []string) error {
	err := verifyForgetOptions(&opts)
	if err != nil {
		return err
	}

	err = verifyPruneOptions(&pruneOptions)
	if err != nil {
		return err
	}

	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	if gopts.NoLock && !opts.DryRun {
		return errors.Fatal("--no-lock is only applicable in combination with --dry-run for forget command")
	}

	if !opts.DryRun || !gopts.NoLock {
		var lock *restic.Lock
		lock, ctx, err = lockRepoExclusive(ctx, repo, gopts.RetryLock, gopts.JSON)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	var snapshots restic.Snapshots
	removeSnIDs := restic.NewIDSet()

	for sn := range FindFilteredSnapshots(ctx, repo, repo, &opts.SnapshotFilter, args) {
		snapshots = append(snapshots, sn)
	}

	var jsonGroups []*ForgetGroup

	if len(args) > 0 {
		// When explicit snapshots args are given, remove them immediately.
		for _, sn := range snapshots {
			removeSnIDs.Insert(*sn.ID())
		}
	} else {
		snapshotGroups, _, err := restic.GroupSnapshots(snapshots, opts.GroupBy)
		if err != nil {
			return err
		}

		policy := restic.ExpirePolicy{
			Last:          int(opts.Last),
			Hourly:        int(opts.Hourly),
			Daily:         int(opts.Daily),
			Weekly:        int(opts.Weekly),
			Monthly:       int(opts.Monthly),
			Yearly:        int(opts.Yearly),
			Within:        opts.Within,
			WithinHourly:  opts.WithinHourly,
			WithinDaily:   opts.WithinDaily,
			WithinWeekly:  opts.WithinWeekly,
			WithinMonthly: opts.WithinMonthly,
			WithinYearly:  opts.WithinYearly,
			Tags:          opts.KeepTags,
		}

		if policy.Empty() && len(args) == 0 {
			if !gopts.JSON {
				Verbosef("no policy was specified, no snapshots will be removed\n")
			}
		}

		if !policy.Empty() {
			if !gopts.JSON {
				Verbosef("Applying Policy: %v\n", policy)
			}

			for k, snapshotGroup := range snapshotGroups {
				if gopts.Verbose >= 1 && !gopts.JSON {
					err = PrintSnapshotGroupHeader(globalOptions.stdout, k)
					if err != nil {
						return err
					}
				}

				var key restic.SnapshotGroupKey
				if json.Unmarshal([]byte(k), &key) != nil {
					return err
				}

				var fg ForgetGroup
				fg.Tags = key.Tags
				fg.Host = key.Hostname
				fg.Paths = key.Paths

				keep, remove, reasons := restic.ApplyPolicy(snapshotGroup, policy)

				if len(keep) != 0 && !gopts.Quiet && !gopts.JSON {
					Printf("keep %d snapshots:\n", len(keep))
					PrintSnapshots(globalOptions.stdout, keep, reasons, opts.Compact)
					Printf("\n")
				}
				addJSONSnapshots(&fg.Keep, keep)

				if len(remove) != 0 && !gopts.Quiet && !gopts.JSON {
					Printf("remove %d snapshots:\n", len(remove))
					PrintSnapshots(globalOptions.stdout, remove, nil, opts.Compact)
					Printf("\n")
				}
				addJSONSnapshots(&fg.Remove, remove)

				fg.Reasons = reasons

				jsonGroups = append(jsonGroups, &fg)

				for _, sn := range remove {
					removeSnIDs.Insert(*sn.ID())
				}
			}
		}
	}

	if len(removeSnIDs) > 0 {
		if !opts.DryRun {
			err := DeleteFilesChecked(ctx, gopts, repo, removeSnIDs, restic.SnapshotFile)
			if err != nil {
				return err
			}
		} else {
			if !gopts.JSON {
				Printf("Would have removed the following snapshots:\n%v\n\n", removeSnIDs)
			}
		}
	}

	if gopts.JSON && len(jsonGroups) > 0 {
		err = printJSONForget(globalOptions.stdout, jsonGroups)
		if err != nil {
			return err
		}
	}

	if len(removeSnIDs) > 0 && opts.Prune {
		if !gopts.JSON {
			if opts.DryRun {
				Verbosef("%d snapshots would be removed, running prune dry run\n", len(removeSnIDs))
			} else {
				Verbosef("%d snapshots have been removed, running prune\n", len(removeSnIDs))
			}
		}
		pruneOptions.DryRun = opts.DryRun
		return runPruneWithRepo(ctx, pruneOptions, gopts, repo, removeSnIDs)
	}

	return nil
}

// ForgetGroup helps to print what is forgotten in JSON.
type ForgetGroup struct {
	Tags    []string            `json:"tags"`
	Host    string              `json:"host"`
	Paths   []string            `json:"paths"`
	Keep    []Snapshot          `json:"keep"`
	Remove  []Snapshot          `json:"remove"`
	Reasons []restic.KeepReason `json:"reasons"`
}

func addJSONSnapshots(js *[]Snapshot, list restic.Snapshots) {
	for _, sn := range list {
		k := Snapshot{
			Snapshot: sn,
			ID:       sn.ID(),
			ShortID:  sn.ID().Str(),
		}
		*js = append(*js, k)
	}
}

func printJSONForget(stdout io.Writer, forgets []*ForgetGroup) error {
	return json.NewEncoder(stdout).Encode(forgets)
}
