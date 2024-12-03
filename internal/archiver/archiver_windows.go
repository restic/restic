package archiver

import (
	"path/filepath"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

// preProcessTargets performs preprocessing of the targets before the loop.
// For Windows, it cleans each target and it also adds ads stream for each
// target to the targets array.
// We read the ADS from each file and add them as independent Nodes with
// the full ADS name as the name of the file.
// During restore the ADS files are restored using the ADS name and that
// automatically attaches them as ADS to the main file.
func preProcessTargets(filesys fs.FS, targets *[]string) {
	for _, target := range *targets {
		if target != "" && filesys.VolumeName(target) == target {
			// special case to allow users to also specify a volume name "C:" instead of a path "C:\"
			target = target + filesys.Separator()
		} else {
			target = filesys.Clean(target)
		}
		addADSStreams(target, targets)
	}
}

// processTarget processes each target in the loop.
// In case of windows the clean up of target is already done
// in preProcessTargets before the loop, hence this is no-op.
func processTarget(_ fs.FS, target string) string {
	return target
}

// getNameFromPathname gets the name from pathname.
// In case for windows the pathname is the full path, so it need to get the base name.
func getNameFromPathname(pathname string) (name string) {
	return filepath.Base(pathname)
}

// preProcessPaths processes paths before looping.
func (arch *Archiver) preProcessPaths(dir string, names []string) (paths []string) {
	// In case of windows we want to add the ADS paths as well before sorting.
	return arch.getPathsIncludingADS(dir, names)
}

// processPath processes the path in the loop.
func (arch *Archiver) processPath(_ string, name string) (path string) {
	// In case of windows we have already prepared the paths before the loop.
	// Hence this is a no-op.
	return name
}

// getPathsIncludingADS iterates all passed path names and adds the ads
// contained in those paths before returning all full paths including ads
func (arch *Archiver) getPathsIncludingADS(dir string, names []string) []string {
	paths := make([]string, 0, len(names))

	for _, name := range names {
		pathname := arch.FS.Join(dir, name)
		paths = append(paths, pathname)
		addADSStreams(pathname, &paths)
	}
	return paths
}

// addADSStreams gets the ads streams if any in the pathname passed and adds them to the passed paths
func addADSStreams(pathname string, paths *[]string) {
	success, adsStreams, err := restic.GetADStreamNames(pathname)
	if success {
		streamCount := len(adsStreams)
		if streamCount > 0 {
			debug.Log("ADS Streams for file: %s, streams: %v", pathname, adsStreams)
			for i := 0; i < streamCount; i++ {
				adsStream := adsStreams[i]
				adsPath := pathname + adsStream
				*paths = append(*paths, adsPath)
			}
		}
	} else if err != nil {
		debug.Log("No ADS found for path: %s, err: %v", pathname, err)
	}
}

// processTargets in windows performs Lstat for the ADS files since the file info would not be available for them yet.
func (arch *Archiver) processTargets(target string, targetMain string, abstarget string, fiMain fs.ExtendedFileInfo) (fi *fs.ExtendedFileInfo, shouldReturn bool, fn futureNode, excluded bool, err error) {
	if target != targetMain {
		//If this is an ADS file we need to Lstat again for the file info.
		fi, err = arch.FS.Lstat(target)
		if err != nil {
			debug.Log("lstat() for %v returned error: %v", target, err)
			err = arch.error(abstarget, err)
			if err != nil {
				return nil, true, futureNode{}, false, errors.WithStack(err)
			}
			//If this is an ads file, shouldReturn should be true because we want to
			// skip the remaining processing of the file.
			return nil, true, futureNode{}, true, nil
		}
	} else {
		fi = &fiMain
	}
	return fi, false, futureNode{}, false, nil
}

// incrementNewFiles increments the new files count
func (c *ChangeStats) incrementNewFiles(node *restic.Node) {
	if node.IsMainFile() {
		c.New++
	}
}

// incrementNewFiles increments the unchanged files count
func (c *ChangeStats) incrementUnchangedFiles(node *restic.Node) {
	if node.IsMainFile() {
		c.Unchanged++
	}
}

// incrementNewFiles increments the changed files count
func (c *ChangeStats) incrementChangedFiles(node *restic.Node) {
	if node.IsMainFile() {
		c.Changed++
	}
}
