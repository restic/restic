package restorer

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/restic/restic/internal/data"
)

type restoreSymlink struct {
	node             *data.Node
	target, location string
	active, done     bool
	skipped          bool
	// endpoint is empty if the target could not be resolved.
	endpoint string
	err      error
}

type symlinkRestorer struct {
	res   *Restorer
	links map[string]*restoreSymlink
}

// restore creates dependencies before the link itself, while preserving the
// normal overwrite checks and progress reporting. Errors are returned at the
// link's own position in the second tree traversal.
func (s *symlinkRestorer) restore(ctx context.Context, link *restoreSymlink) {
	if link.done || link.active {
		return
	}
	link.active = true
	defer func() {
		link.active = false
		link.done = true
	}()
	if link.err = ctx.Err(); link.err != nil {
		return
	}

	link.skipped = true
	_, link.err = s.res.withOverwriteCheck(ctx, link.node, link.target, link.location, false, nil, func(_ bool, _ *fileState) error {
		link.skipped = false
		link.endpoint = s.resolve(ctx, filepath.Dir(link.target), link.node.LinkTarget, make(map[string]bool))
		if err := ctx.Err(); err != nil {
			return err
		}
		return s.res.restoreNodeTo(link.node, link.target, link.location)
	})
	if link.err != nil {
		link.endpoint = ""
	}
}

// resolve walks components instead of cleaning the entire target: a symlink in
// an intermediate component can introduce another selected dependency, and must
// be resolved before processing a following ".." component.
func (s *symlinkRestorer) resolve(ctx context.Context, base, target string, seen map[string]bool) string {
	// Only ordinary relative targets require dependency ordering here.
	if filepath.VolumeName(target) != "" || (len(target) > 0 && os.IsPathSeparator(target[0])) {
		return ""
	}
	for _, part := range strings.FieldsFunc(target, func(r rune) bool {
		return r == '/' || r == rune(filepath.Separator)
	}) {
		if ctx.Err() != nil {
			return ""
		}
		if part == "." {
			continue
		}
		base = filepath.Join(base, part)
		if link, ok := s.links[toComparableFilename(base)]; ok {
			s.restore(ctx, link)
			if link.active || link.err != nil {
				return ""
			}
			if link.skipped {
				// Inspect the actual target only when this skipped link is needed
				// by another link. It may differ from the saved target.
				base = s.resolveExisting(ctx, link.target, seen)
			} else {
				base = link.endpoint
			}
		} else {
			base = s.resolveExisting(ctx, base, seen)
		}
		if base == "" {
			return ""
		}
	}
	return base
}

func (s *symlinkRestorer) resolveExisting(ctx context.Context, path string, seen map[string]bool) string {
	if ctx.Err() != nil {
		return ""
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return ""
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return path
	}
	key := toComparableFilename(path)
	// Bound traversal through existing links as well as detecting selected cycles.
	if seen[key] || len(seen) >= 255 {
		return ""
	}
	seen[key] = true
	defer delete(seen, key)
	target, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	return s.resolve(ctx, filepath.Dir(path), target, seen)
}
