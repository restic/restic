//go:build !windows
// +build !windows

package archiver

import (
	"os"
	"path/filepath"
	"testing"
)

// getTargetPath gets the target path from the target and the name
func getTargetPath(target string, name string) (targetPath string) {
	return filepath.Join(target, name)
}

// writeFile writes the content to the file at the targetPath
func writeFile(_ testing.TB, targetPath string, content string) (err error) {
	return os.WriteFile(targetPath, []byte(content), 0644)
}
