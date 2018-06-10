package main

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
)

var cmdFind = &cobra.Command{
	Use:   "find [flags] PATTERN",
	Short: "Find a file or directory",
	Long: `
The "find" command searches for files or directories in snapshots stored in the
repo. `,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runFind(findOptions, globalOptions, args)
	},
}

// FindOptions bundles all options for the find command.
type FindOptions struct {
	Oldest          string
	Newest          string
	Snapshots       []string
	CaseInsensitive bool
	ListLong        bool
	Host            string
	Paths           []string
	Tags            restic.TagLists
}

var findOptions FindOptions

func init() {
	cmdRoot.AddCommand(cmdFind)

	f := cmdFind.Flags()
	f.StringVarP(&findOptions.Oldest, "oldest", "O", "", "oldest modification date/time")
	f.StringVarP(&findOptions.Newest, "newest", "N", "", "newest modification date/time")
	f.StringArrayVarP(&findOptions.Snapshots, "snapshot", "s", nil, "snapshot `id` to search in (can be given multiple times)")
	f.BoolVarP(&findOptions.CaseInsensitive, "ignore-case", "i", false, "ignore case for pattern")
	f.BoolVarP(&findOptions.ListLong, "long", "l", false, "use a long listing format showing size and mode")

	f.StringVarP(&findOptions.Host, "host", "H", "", "only consider snapshots for this `host`, when no snapshot ID is given")
	f.Var(&findOptions.Tags, "tag", "only consider snapshots which include this `taglist`, when no snapshot-ID is given")
	f.StringArrayVar(&findOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path`, when no snapshot-ID is given")
}

type findPattern struct {
	oldest, newest time.Time
	pattern        string
	ignoreCase     bool
}

var timeFormats = []string{
	"2006-01-02",
	"2006-01-02 15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05 MST",
	"02.01.2006",
	"02.01.2006 15:04",
	"02.01.2006 15:04:05",
	"02.01.2006 15:04:05 -0700",
	"02.01.2006 15:04:05 MST",
	"Mon Jan 2 15:04:05 -0700 MST 2006",
}

func parseTime(str string) (time.Time, error) {
	for _, fmt := range timeFormats {
		if t, err := time.ParseInLocation(fmt, str, time.Local); err == nil {
			return t, nil
		}
	}

	return time.Time{}, errors.Fatalf("unable to parse time: %q", str)
}

type statefulOutput struct {
	ListLong bool
	JSON     bool
	inuse    bool
	newsn    *restic.Snapshot
	oldsn    *restic.Snapshot
	hits     int
}

func (s *statefulOutput) PrintJSON(path string, node *restic.Node) {
	type findNode restic.Node
	b, err := json.Marshal(struct {
		// Add these attributes
		Path        string `json:"path,omitempty"`
		Permissions string `json:"permissions,omitempty"`

		*findNode

		// Make the following attributes disappear
		Name               byte `json:"name,omitempty"`
		Inode              byte `json:"inode,omitempty"`
		ExtendedAttributes byte `json:"extended_attributes,omitempty"`
		Device             byte `json:"device,omitempty"`
		Content            byte `json:"content,omitempty"`
		Subtree            byte `json:"subtree,omitempty"`
	}{
		Path:        path,
		Permissions: node.Mode.String(),
		findNode:    (*findNode)(node),
	})
	if err != nil {
		Warnf("Marshall failed: %v\n", err)
		return
	}
	if !s.inuse {
		Printf("[")
		s.inuse = true
	}
	if s.newsn != s.oldsn {
		if s.oldsn != nil {
			Printf("],\"hits\":%d,\"snapshot\":%q},", s.hits, s.oldsn.ID())
		}
		Printf(`{"matches":[`)
		s.oldsn = s.newsn
		s.hits = 0
	}
	if s.hits > 0 {
		Printf(",")
	}
	Printf(string(b))
	s.hits++
}

func (s *statefulOutput) PrintNormal(path string, node *restic.Node) {
	if s.newsn != s.oldsn {
		if s.oldsn != nil {
			Verbosef("\n")
		}
		s.oldsn = s.newsn
		Verbosef("Found matching entries in snapshot %s\n", s.oldsn.ID().Str())
	}
	Printf(formatNode(path, node, s.ListLong) + "\n")
}

func (s *statefulOutput) Print(path string, node *restic.Node) {
	if s.JSON {
		s.PrintJSON(path, node)
	} else {
		s.PrintNormal(path, node)
	}
}

func (s *statefulOutput) Finish() {
	if s.JSON {
		// do some finishing up
		if s.oldsn != nil {
			Printf("],\"hits\":%d,\"snapshot\":%q}", s.hits, s.oldsn.ID())
		}
		if s.inuse {
			Printf("]\n")
		} else {
			Printf("[]\n")
		}
		return
	}
}

// Finder bundles information needed to find a file or directory.
type Finder struct {
	repo        restic.Repository
	pat         findPattern
	out         statefulOutput
	ignoreTrees restic.IDSet
}

func (f *Finder) findInSnapshot(ctx context.Context, sn *restic.Snapshot) error {
	debug.Log("searching in snapshot %s\n  for entries within [%s %s]", sn.ID(), f.pat.oldest, f.pat.newest)

	if sn.Tree == nil {
		return errors.Errorf("snapshot %v has no tree", sn.ID().Str())
	}

	f.out.newsn = sn
	return walker.Walk(ctx, f.repo, *sn.Tree, f.ignoreTrees, func(nodepath string, node *restic.Node, err error) (bool, error) {
		if err != nil {
			return false, err
		}

		if node == nil {
			return false, nil
		}

		name := node.Name
		if f.pat.ignoreCase {
			name = strings.ToLower(name)
		}

		foundMatch, err := filter.Match(f.pat.pattern, nodepath)
		if err != nil {
			return false, err
		}

		var (
			ignoreIfNoMatch = true
			errIfNoMatch    error
		)
		if node.Type == "dir" {
			childMayMatch, err := filter.ChildMatch(f.pat.pattern, nodepath)
			if err != nil {
				return false, err
			}

			if !childMayMatch {
				ignoreIfNoMatch = true
				errIfNoMatch = walker.SkipNode
			} else {
				ignoreIfNoMatch = false
			}
		}

		if !foundMatch {
			return ignoreIfNoMatch, errIfNoMatch
		}

		if !f.pat.oldest.IsZero() && node.ModTime.Before(f.pat.oldest) {
			debug.Log("    ModTime is older than %s\n", f.pat.oldest)
			return ignoreIfNoMatch, errIfNoMatch
		}

		if !f.pat.newest.IsZero() && node.ModTime.After(f.pat.newest) {
			debug.Log("    ModTime is newer than %s\n", f.pat.newest)
			return ignoreIfNoMatch, errIfNoMatch
		}

		debug.Log("    found match\n")
		f.out.Print(nodepath, node)
		return false, nil
	})
}

func runFind(opts FindOptions, gopts GlobalOptions, args []string) error {
	if len(args) != 1 {
		return errors.Fatal("wrong number of arguments")
	}

	var err error
	pat := findPattern{pattern: args[0]}
	if opts.CaseInsensitive {
		pat.pattern = strings.ToLower(pat.pattern)
		pat.ignoreCase = true
	}

	if opts.Oldest != "" {
		if pat.oldest, err = parseTime(opts.Oldest); err != nil {
			return err
		}
	}

	if opts.Newest != "" {
		if pat.newest, err = parseTime(opts.Newest); err != nil {
			return err
		}
	}

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		lock, err := lockRepo(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	if err = repo.LoadIndex(gopts.ctx); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()

	f := &Finder{
		repo:        repo,
		pat:         pat,
		out:         statefulOutput{ListLong: opts.ListLong, JSON: globalOptions.JSON},
		ignoreTrees: restic.NewIDSet(),
	}
	for sn := range FindFilteredSnapshots(ctx, repo, opts.Host, opts.Tags, opts.Paths, opts.Snapshots) {
		if err = f.findInSnapshot(ctx, sn); err != nil {
			return err
		}
	}
	f.out.Finish()

	return nil
}
