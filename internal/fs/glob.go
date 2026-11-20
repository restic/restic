package fs

import (
	"errors"
	"os"
	"path"
	"slices"
	"strings"
	"syscall"
)

// globHasMeta reports whether s contains any of the characters recognized by
// path.Match.
func globHasMeta(s string) bool {
	return strings.ContainsAny(s, `*?[\`)
}

// Glob returns the paths in filesys which match the absolute pattern, using
// the syntax of path.Match. It is like filepath.Glob, but for file systems
// which use "/" as separator. Only directories which are needed to expand a
// path component containing wildcards are read, so a pattern without
// wildcards in the leading components does not read their parents.
func Glob(filesys FS, pattern string) ([]string, error) {
	if _, err := path.Match(pattern, ""); err != nil {
		return nil, err
	}
	if !path.IsAbs(pattern) {
		return nil, errors.New("pattern must be absolute")
	}

	matches := []string{"/"}
	listed := false
	for _, comp := range strings.Split(path.Clean(pattern)[1:], "/") {
		if comp == "" {
			continue
		}

		var next []string
		if !globHasMeta(comp) {
			for _, m := range matches {
				next = append(next, path.Join(m, comp))
			}
			listed = false
		} else {
			for _, m := range matches {
				names, err := Readdirnames(filesys, m, O_NOFOLLOW)
				if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
					continue
				}
				if err != nil {
					return nil, err
				}
				slices.Sort(names)
				for _, name := range names {
					if ok, _ := path.Match(comp, name); ok {
						next = append(next, path.Join(m, name))
					}
				}
			}
			listed = true
		}

		matches = next
		if len(matches) == 0 {
			return nil, nil
		}
	}

	if listed {
		return matches, nil
	}

	// the last component did not contain wildcards, check that it exists
	var result []string
	for _, m := range matches {
		_, err := filesys.Lstat(m)
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, nil
}
