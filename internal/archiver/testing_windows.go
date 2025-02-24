package archiver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/fs"
)

// getTargetPath gets the target path from the target and the name
func getTargetPath(target string, name string) (targetPath string) {
	if name[0] == ':' {
		// If the first char of the name is :, append the name to the targetPath.
		// This switch is useful for cases like creating directories having ads attributes attached.
		// Without this, if we put the directory ads creation at top level, eg. "dir" and "dir:dirstream1:$DATA",
		// since they can be created in any order it could first create an empty file called "dir" with the ads
		// stream and then the dir creation fails.
		targetPath = target + name
	} else {
		targetPath = filepath.Join(target, name)
	}
	return targetPath
}

// writeFile writes the content to the file at the targetPath
func writeFile(t testing.TB, targetPath string, content string) (err error) {
	//For windows, create file only if it doesn't exist. Otherwise ads streams may get overwritten.
	f, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_TRUNC, 0644)

	if os.IsNotExist(err) {
		f, err = os.OpenFile(targetPath, os.O_WRONLY|fs.O_CREATE|os.O_TRUNC, 0644)
	}

	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Write([]byte(content))
	if err1 := f.Close(); err1 != nil && err == nil {
		err = err1
	}
	return err
}
