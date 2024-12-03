//go:build !windows
// +build !windows

package archiver

import (
	"os"

	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

// preProcessTargets performs preprocessing of the targets before the loop.
// It is a no-op on non-windows OS as we do not need to do an
// extra iteration on the targets before the loop.
// We process each target inside the loop.
func preProcessTargets(_ fs.FS, _ *[]string) {
	// no-op
}

// processTarget processes each target in the loop.
// In case of non-windows OS it uses the passed filesys to clean the target.
func processTarget(filesys fs.FS, target string) string {
	return filesys.Clean(target)
}

// preProcessPaths processes paths before looping.
func (arch *Archiver) preProcessPaths(_ string, names []string) (paths []string) {
	// In case of non-windows OS this is no-op as we process the paths within the loop
	// and avoid the extra looping before hand.
	return names
}

// processPath processes the path in the loop.
func (arch *Archiver) processPath(dir string, name string) (path string) {
	//In case of non-windows OS we prepare the path in the loop.
	return arch.FS.Join(dir, name)
}

// getNameFromPathname gets the name from pathname.
// In case for non-windows the pathname is same as the name.
func getNameFromPathname(pathname string) (name string) {
	return pathname
}

// processTargets is no-op for non-windows OS
func (arch *Archiver) processTargets(_ string, _ string, _ string, fiMain os.FileInfo) (fi os.FileInfo, shouldReturn bool, fn futureNode, excluded bool, err error) {
	return fiMain, false, futureNode{}, false, nil
}

// incrementNewFiles increments the new files count
func (c *ChangeStats) incrementNewFiles(_ *restic.Node) {
	c.New++
}

// incrementNewFiles increments the unchanged files count
func (c *ChangeStats) incrementUnchangedFiles(_ *restic.Node) {
	c.Unchanged++
}

// incrementNewFiles increments the changed files count
func (c *ChangeStats) incrementChangedFiles(_ *restic.Node) {
	c.Changed++
}
