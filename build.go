// +build ignore

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

var (
	verbose    bool
	keepGopath bool
)

const timeFormat = "2006-01-02 15:04:05"

// specialDir returns true if the file begins with a special character ('.' or '_').
func specialDir(name string) bool {
	if name == "." {
		return false
	}

	base := filepath.Base(name)
	return base[0] == '_' || base[0] == '.'
}

// updateGopath builds a valid GOPATH at dst, with all Go files in src/ copied
// to dst/prefix/, so calling
//
//   updateGopath("/tmp/gopath", "/home/u/restic", "github.com/restic/restic")
//
// with "/home/u/restic" containing the file "foo.go" yields the following tree
// at "/tmp/gopath":
//
//   /tmp/gopath
//   └── src
//       └── github.com
//           └── restic
//               └── restic
//                   └── foo.go
func updateGopath(dst, src, prefix string) error {
	return filepath.Walk(src, func(name string, fi os.FileInfo, err error) error {
		if specialDir(name) {
			if fi.IsDir() {
				return filepath.SkipDir
			}

			return nil
		}

		if fi.IsDir() {
			return nil
		}

		ext := path.Ext(name)
		if ext != ".go" && ext != ".s" {
			return nil
		}

		intermediatePath, err := filepath.Rel(src, name)
		if err != nil {
			return err
		}

		fileSrc := filepath.Join(src, intermediatePath)
		fileDst := filepath.Join(dst, "src", prefix, intermediatePath)

		return copyFile(fileDst, fileSrc)
	})
}

// copyFile creates dst from src, preserving file attributes and timestamps.
func copyFile(dst, src string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}

	fsrc, err := os.Open(src)
	if err != nil {
		return err
	}

	if err = os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		fmt.Printf("MkdirAll(%v)\n", filepath.Dir(dst))
		return err
	}

	fdst, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err = io.Copy(fdst, fsrc); err != nil {
		return err
	}

	if err == nil {
		err = fsrc.Close()
	}

	if err == nil {
		err = fdst.Close()
	}

	if err == nil {
		err = os.Chmod(dst, fi.Mode())
	}

	if err == nil {
		err = os.Chtimes(dst, fi.ModTime(), fi.ModTime())
	}

	return nil
}

// die prints the message with fmt.Fprintf() to stderr and exits with an error
// code.
func die(message string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, message, args...)
	os.Exit(1)
}

func showUsage(output io.Writer) {
	fmt.Fprintf(output, "USAGE: go run build.go OPTIONS\n")
	fmt.Fprintf(output, "\n")
	fmt.Fprintf(output, "OPTIONS:\n")
	fmt.Fprintf(output, "  -v     --verbose   output more messages\n")
}

func verbosePrintf(message string, args ...interface{}) {
	if !verbose {
		return
	}

	fmt.Printf(message, args...)
}

// cleanEnv returns a clean environment with GOPATH and GOBIN removed (if
// present).
func cleanEnv() (env []string) {
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, "GOPATH=") || strings.HasPrefix(v, "GOBIN=") {
			continue
		}

		env = append(env, v)
	}

	return env
}

// build runs "go build args..." with GOPATH set to gopath.
func build(gopath string, args ...string) error {
	args = append([]string{"build"}, args...)
	cmd := exec.Command("go", args...)
	cmd.Env = append(cleanEnv(), "GOPATH="+gopath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	verbosePrintf("go %s\n", args)

	return cmd.Run()
}

// getVersion returns a version string, either from the file VERSION in the
// current directory or from git.
func getVersion() string {
	v, err := ioutil.ReadFile("VERSION")
	version := strings.TrimSpace(string(v))
	if err == nil {
		verbosePrintf("version from file 'VERSION' is %s\n", version)
		return version
	}

	return gitVersion()
}

// gitVersion returns a version string that identifies the currently checked
// out git commit.
func gitVersion() string {
	cmd := exec.Command("git", "describe",
		"--long", "--tags", "--dirty", "--always")
	out, err := cmd.Output()
	if err != nil {
		die("git describe returned error: %v\n", err)
	}

	version := strings.TrimSpace(string(out))
	verbosePrintf("git version is %s\n", version)
	return version
}

func main() {
	for _, arg := range os.Args[1:] {
		switch arg {
		case "-v", "--verbose":
			verbose = true
		case "-k", "--keep-gopath":
			keepGopath = true
		case "-h":
			showUsage(os.Stdout)
		default:
			fmt.Fprintf(os.Stderr, "Error: unknown option %q\n\n", arg)
			showUsage(os.Stderr)
			os.Exit(1)
		}
	}

	root, err := os.Getwd()
	if err != nil {
		die("Getwd(): %v\n", err)
	}

	gopath, err := ioutil.TempDir("", "restic-build-")
	if err != nil {
		die("TempDir(): %v\n", err)
	}

	verbosePrintf("create GOPATH at %v\n", gopath)
	if err = updateGopath(gopath, root, "github.com/restic/restic"); err != nil {
		die("copying files from %v to %v failed: %v\n", root, gopath, err)
	}

	vendor := filepath.Join(root, "Godeps", "_workspace", "src")
	if err = updateGopath(gopath, vendor, ""); err != nil {
		die("copying files from %v to %v failed: %v\n", root, gopath, err)
	}

	version := getVersion()
	compileTime := time.Now().Format(timeFormat)
	args := []string{
		"-tags", "release",
		"-ldflags", fmt.Sprintf(`-s -X main.version %q -X main.compiledAt %q`, version, compileTime),
		"-o", "restic", "github.com/restic/restic/cmd/restic",
	}
	err = build(gopath, args...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n", err)
	}

	if !keepGopath {
		verbosePrintf("remove %v\n", gopath)
		if err = os.RemoveAll(gopath); err != nil {
			die("remove GOPATH at %s failed: %v\n", err)
		}
	} else {
		fmt.Printf("leaving temporary GOPATH at %v\n", gopath)
	}
}
