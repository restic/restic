package patchedfilepath

import (
	"path/filepath"
	"restic/patched"
)

// Abstraction Layer to go's filepath package
// One reason is to able to patch around issues with windows long pathnames

func Walk(root string, walkFn filepath.WalkFunc) error {
	return filepath.Walk(patched.Fixpath(root), walkFn)
}
