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
	"runtime"
	"strings"
	"time"
)

var (
	verbose    bool
	keepGopath bool
	runTests   bool
)

const timeFormat = "2006-01-02 15:04:05"

// specialDir returns true if the file begins with a special character ('.' or '_').
func specialDir(name string) bool {
	if name == "." {
		return false
	}

	base := filepath.Base(name)
	if base == "vendor" || base[0] == '_' || base[0] == '.' {
		return true
	}

	return false
}

// excludePath returns true if the file should not be copied to the new GOPATH.
func excludePath(name string) bool {
	ext := path.Ext(name)
	if ext == ".go" || ext == ".s" {
		return false
	}

	parentDir := filepath.Base(filepath.Dir(name))
	if parentDir == "testdata" {
		return false
	}

	return true
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

		if excludePath(name) {
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
	fmt.Fprintf(output, "  -v     --verbose       output more messages\n")
	fmt.Fprintf(output, "  -t     --tags          specify additional build tags\n")
	fmt.Fprintf(output, "  -k     --keep-gopath   do not remove the GOPATH after build\n")
	fmt.Fprintf(output, "  -T     --test          run tests\n")
	fmt.Fprintf(output, "         --goos value    set GOOS for cross-compilation\n")
	fmt.Fprintf(output, "         --goarch value  set GOARCH for cross-compilation\n")
}

func verbosePrintf(message string, args ...interface{}) {
	if !verbose {
		return
	}

	fmt.Printf("build: "+message, args...)
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
func build(cwd, goos, goarch, gopath string, args ...string) error {
	args = append([]string{"build"}, args...)
	cmd := exec.Command("go", args...)
	cmd.Env = append(cleanEnv(), "GOPATH="+gopath, "GOARCH="+goarch, "GOOS="+goos)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	verbosePrintf("go %s\n", args)

	return cmd.Run()
}

// test runs "go test args..." with GOPATH set to gopath.
func test(cwd, gopath string, args ...string) error {
	args = append([]string{"test"}, args...)
	cmd := exec.Command("go", args...)
	cmd.Env = append(cleanEnv(), "GOPATH="+gopath)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	verbosePrintf("go %s\n", args)

	return cmd.Run()
}

// getVersion returns the version string from the file VERSION in the current
// directory.
func getVersionFromFile() string {
	buf, err := ioutil.ReadFile("VERSION")
	if err != nil {
		verbosePrintf("error reading file VERSION: %v\n", err)
		return ""
	}

	return strings.TrimSpace(string(buf))
}

// getVersion returns a version string which is a combination of the contents
// of the file VERSION in the current directory and the version from git (if
// available).
func getVersion() string {
	versionFile := getVersionFromFile()
	versionGit := getVersionFromGit()

	verbosePrintf("version from file 'VERSION' is %q, version from git %q\n",
		versionFile, versionGit)

	switch {
	case versionFile == "":
		return versionGit
	case versionGit == "":
		return versionFile
	}

	return fmt.Sprintf("%s (%s)", versionFile, versionGit)
}

// getVersionFromGit returns a version string that identifies the currently
// checked out git commit.
func getVersionFromGit() string {
	cmd := exec.Command("git", "describe",
		"--long", "--tags", "--dirty", "--always")
	out, err := cmd.Output()
	if err != nil {
		verbosePrintf("git describe returned error: %v\n", err)
		return ""
	}

	version := strings.TrimSpace(string(out))
	verbosePrintf("git version is %s\n", version)
	return version
}

// Constants represents a set of constants that are set in the final binary to
// the given value via compiler flags.
type Constants map[string]string

// LDFlags returns the string that can be passed to go build's `-ldflags`.
func (cs Constants) LDFlags() string {
	l := make([]string, 0, len(cs))

	if runtime.Version() < "go1.5" {
		for k, v := range cs {
			l = append(l, fmt.Sprintf(`-X %q %q`, k, v))
		}
	} else {
		for k, v := range cs {
			l = append(l, fmt.Sprintf(`-X "%s=%s"`, k, v))
		}
	}

	return strings.Join(l, " ")
}

func main() {
	if runtime.Version() < "go1.6" {
		fmt.Fprintf(os.Stderr, "old version of Go detected (%v), I'll try but no guarantees\n", runtime.Version())
	}

	buildTags := []string{}

	skipNext := false
	params := os.Args[1:]

	targetGOOS := runtime.GOOS
	targetGOARCH := runtime.GOARCH

	for i, arg := range params {
		if skipNext {
			skipNext = false
			continue
		}

		switch arg {
		case "-v", "--verbose":
			verbose = true
		case "-k", "--keep-gopath":
			keepGopath = true
		case "-t", "-tags", "--tags":
			skipNext = true
			buildTags = strings.Split(params[i+1], " ")
		case "-T", "--test":
			runTests = true
		case "--goos":
			skipNext = true
			targetGOOS = params[i+1]
		case "--goarch":
			skipNext = true
			targetGOARCH = params[i+1]
		case "-h":
			showUsage(os.Stdout)
			return
		default:
			fmt.Fprintf(os.Stderr, "Error: unknown option %q\n\n", arg)
			showUsage(os.Stderr)
			os.Exit(1)
		}
	}

	if len(buildTags) == 0 {
		verbosePrintf("adding build-tag release\n")
		buildTags = []string{"release"}
	}

	for i := range buildTags {
		buildTags[i] = strings.TrimSpace(buildTags[i])
	}

	verbosePrintf("build tags: %s\n", buildTags)

	root, err := os.Getwd()
	if err != nil {
		die("Getwd(): %v\n", err)
	}

	gopath, err := ioutil.TempDir("", "restic-build-")
	if err != nil {
		die("TempDir(): %v\n", err)
	}

	verbosePrintf("create GOPATH at %v\n", gopath)
	if err = updateGopath(gopath, filepath.Join(root, "src", "restic"), "restic"); err != nil {
		die("copying files from %v/src/restic to %v/src/restic failed: %v\n", root, gopath, err)
	}

	if err = updateGopath(gopath, filepath.Join(root, "src", "cmds"), "cmds"); err != nil {
		die("copying files from %v/src/cmds to %v/src/restic/cmds failed: %v\n", root, gopath, err)
	}

	vendor := filepath.Join(root, "vendor", "src")
	if err = updateGopath(gopath, vendor, ""); err != nil {
		die("copying files from %v to %v/src failed: %v\n", vendor, gopath, err)
	}

	defer func() {
		if !keepGopath {
			verbosePrintf("remove %v\n", gopath)
			if err = os.RemoveAll(gopath); err != nil {
				die("remove GOPATH at %s failed: %v\n", err)
			}
		} else {
			verbosePrintf("leaving temporary GOPATH at %v\n", gopath)
		}
	}()

	outputFilename := "restic"
	if targetGOOS == "windows" {
		outputFilename = "restic.exe"
	}

	cwd, err := os.Getwd()
	if err != nil {
		die("Getwd() returned %v\n", err)
	}
	output := filepath.Join(cwd, outputFilename)

	version := getVersion()
	compileTime := time.Now().Format(timeFormat)
	constants := Constants{`main.compiledAt`: compileTime}
	if version != "" {
		constants["main.version"] = version
	}
	ldflags := "-s -w " + constants.LDFlags()
	verbosePrintf("ldflags: %s\n", ldflags)

	args := []string{
		"-tags", strings.Join(buildTags, " "),
		"-ldflags", ldflags,
		"-o", output, "cmds/restic",
	}

	err = build(filepath.Join(gopath, "src"), targetGOOS, targetGOARCH, gopath, args...)
	if err != nil {
		die("build failed: %v\n", err)
	}

	if runTests {
		verbosePrintf("running tests\n")

		err = test(filepath.Join(gopath, "src"), gopath, "restic/...")
		if err != nil {
			die("running tests failed: %v\n", err)
		}
	}
}
