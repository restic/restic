// Description
//
// This program aims to make building Go programs for end users easier by just
// calling it with `go run`, without having to setup a GOPATH.
//
// For Go < 1.11, it'll create a new GOPATH in a temporary directory, then run
// `go build` on the package configured as Main in the Config struct.
//
// For Go >= 1.11 if the file go.mod is present, it'll use Go modules and not
// setup a GOPATH. It builds the package configured as Main in the Config
// struct with `go build -mod=vendor` to use the vendored dependencies.
// The variable GOPROXY is set to `off` so that no network calls are made. All
// files are copied to a temporary directory before `go build` is called within
// that directory.

// BSD 2-Clause License
//
// Copyright (c) 2016-2018, Alexander Neumann <alexander@bumpern.de>
// All rights reserved.
//
// This file has been copied from the repository at:
// https://github.com/fd0/build-go
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// * Redistributions of source code must retain the above copyright notice, this
//   list of conditions and the following disclaimer.
//
// * Redistributions in binary form must reproduce the above copyright notice,
//   this list of conditions and the following disclaimer in the documentation
//   and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
// FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
// DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
// SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
// CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
// OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

// +build ignore_build_go

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// config contains the configuration for the program to build.
var config = Config{
	Name:             "restic",                                // name of the program executable and directory
	Namespace:        "github.com/restic/restic",              // subdir of GOPATH, e.g. "github.com/foo/bar"
	Main:             "./cmd/restic",                          // package name for the main package
	DefaultBuildTags: []string{"selfupdate"},                  // specify build tags which are always used
	Tests:            []string{"./..."},                       // tests to run
	MinVersion:       GoVersion{Major: 1, Minor: 9, Patch: 0}, // minimum Go version supported
}

// Config configures the build.
type Config struct {
	Name             string
	Namespace        string
	Main             string
	DefaultBuildTags []string
	Tests            []string
	MinVersion       GoVersion
}

var (
	verbose    bool
	keepGopath bool
	runTests   bool
	enableCGO  bool
	enablePIE  bool
	goVersion  = ParseGoVersion(runtime.Version())
)

// copy all Go files in src to dst, creating directories on the fly, so calling
//
//   copy("/tmp/gopath/src/github.com/restic/restic", "/home/u/restic")
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
func copy(dst, src string) error {
	verbosePrintf("copy contents of %v to %v\n", src, dst)
	return filepath.Walk(src, func(name string, fi os.FileInfo, err error) error {
		if name == src {
			return err
		}

		if name == ".git" {
			return filepath.SkipDir
		}

		if err != nil {
			return err
		}

		if fi.IsDir() {
			return nil
		}

		intermediatePath, err := filepath.Rel(src, name)
		if err != nil {
			return err
		}

		fileSrc := filepath.Join(src, intermediatePath)
		fileDst := filepath.Join(dst, intermediatePath)

		return copyFile(fileDst, fileSrc)
	})
}

func directoryExists(dirname string) bool {
	stat, err := os.Stat(dirname)
	if err != nil && os.IsNotExist(err) {
		return false
	}

	return stat.IsDir()
}

func fileExists(filename string) bool {
	stat, err := os.Stat(filename)
	if err != nil && os.IsNotExist(err) {
		return false
	}

	return stat.Mode().IsRegular()
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
		_ = fsrc.Close()
		return err
	}

	_, err = io.Copy(fdst, fsrc)
	if err != nil {
		_ = fsrc.Close()
		_ = fdst.Close()
		return err
	}

	err = fdst.Close()
	if err != nil {
		_ = fsrc.Close()
		return err
	}

	err = fsrc.Close()
	if err != nil {
		return err
	}

	err = os.Chmod(dst, fi.Mode())
	if err != nil {
		return err
	}

	return os.Chtimes(dst, fi.ModTime(), fi.ModTime())
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
	fmt.Fprintf(output, "  -k     --keep-tempdir  do not remove the temporary directory after build\n")
	fmt.Fprintf(output, "  -T     --test          run tests\n")
	fmt.Fprintf(output, "  -o     --output        set output file name\n")
	fmt.Fprintf(output, "         --enable-cgo    use CGO to link against libc\n")
	fmt.Fprintf(output, "         --enable-pie    use PIE buildmode\n")
	fmt.Fprintf(output, "         --goos value    set GOOS for cross-compilation\n")
	fmt.Fprintf(output, "         --goarch value  set GOARCH for cross-compilation\n")
	fmt.Fprintf(output, "         --goarm value   set GOARM for cross-compilation\n")
	fmt.Fprintf(output, "         --tempdir dir   use a specific directory for compilation\n")
}

func verbosePrintf(message string, args ...interface{}) {
	if !verbose {
		return
	}

	fmt.Printf("build: "+message, args...)
}

// cleanEnv returns a clean environment with GOPATH, GOBIN and GO111MODULE
// removed (if present).
func cleanEnv() (env []string) {
	removeKeys := map[string]struct{}{
		"GOPATH":      struct{}{},
		"GOBIN":       struct{}{},
		"GO111MODULE": struct{}{},
	}

	for _, v := range os.Environ() {
		data := strings.SplitN(v, "=", 2)
		name := data[0]

		if _, ok := removeKeys[name]; ok {
			continue
		}

		env = append(env, v)
	}

	return env
}

// build runs "go build args..." with GOPATH set to gopath.
func build(cwd string, env map[string]string, args ...string) error {
	a := []string{"build"}

	if goVersion.AtLeast(GoVersion{1, 10, 0}) {
		verbosePrintf("Go version is at least 1.10, using new syntax for -gcflags\n")
		// use new prefix
		a = append(a, "-asmflags", fmt.Sprintf("all=-trimpath=%s", cwd))
		a = append(a, "-gcflags", fmt.Sprintf("all=-trimpath=%s", cwd))
	} else {
		a = append(a, "-asmflags", fmt.Sprintf("-trimpath=%s", cwd))
		a = append(a, "-gcflags", fmt.Sprintf("-trimpath=%s", cwd))
	}
	if enablePIE {
		a = append(a, "-buildmode=pie")
	}

	a = append(a, args...)
	cmd := exec.Command("go", a...)
	cmd.Env = append(cleanEnv(), "GOPROXY=off")
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if !enableCGO {
		cmd.Env = append(cmd.Env, "CGO_ENABLED=0")
	}

	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	verbosePrintf("chdir %q\n", cwd)
	verbosePrintf("go %q\n", a)

	return cmd.Run()
}

// test runs "go test args..." with GOPATH set to gopath.
func test(cwd string, env map[string]string, args ...string) error {
	args = append([]string{"test", "-count", "1"}, args...)
	cmd := exec.Command("go", args...)
	cmd.Env = append(cleanEnv(), "GOPROXY=off")
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if !enableCGO {
		cmd.Env = append(cmd.Env, "CGO_ENABLED=0")
	}
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	verbosePrintf("chdir %q\n", cwd)
	verbosePrintf("go %q\n", args)

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

	for k, v := range cs {
		l = append(l, fmt.Sprintf(`-X "%s=%s"`, k, v))
	}

	return strings.Join(l, " ")
}

// GoVersion is the version of Go used to compile the project.
type GoVersion struct {
	Major int
	Minor int
	Patch int
}

// ParseGoVersion parses the Go version s. If s cannot be parsed, the returned GoVersion is null.
func ParseGoVersion(s string) (v GoVersion) {
	if !strings.HasPrefix(s, "go") {
		return
	}

	s = s[2:]
	data := strings.Split(s, ".")
	if len(data) < 2 || len(data) > 3 {
		// invalid version
		return GoVersion{}
	}

	var err error

	v.Major, err = strconv.Atoi(data[0])
	if err != nil {
		return GoVersion{}
	}

	// try to parse the minor version while removing an eventual suffix (like
	// "rc2" or so)
	for s := data[1]; s != ""; s = s[:len(s)-1] {
		v.Minor, err = strconv.Atoi(s)
		if err == nil {
			break
		}
	}

	if v.Minor == 0 {
		// no minor version found
		return GoVersion{}
	}

	if len(data) >= 3 {
		v.Patch, err = strconv.Atoi(data[2])
		if err != nil {
			return GoVersion{}
		}
	}

	return
}

// AtLeast returns true if v is at least as new as other. If v is empty, true is returned.
func (v GoVersion) AtLeast(other GoVersion) bool {
	var empty GoVersion

	// the empty version satisfies all versions
	if v == empty {
		return true
	}

	if v.Major < other.Major {
		return false
	}

	if v.Minor < other.Minor {
		return false
	}

	if v.Patch < other.Patch {
		return false
	}

	return true
}

func (v GoVersion) String() string {
	return fmt.Sprintf("Go %d.%d.%d", v.Major, v.Minor, v.Patch)
}

func main() {
	if !goVersion.AtLeast(config.MinVersion) {
		fmt.Fprintf(os.Stderr, "%s detected, this program requires at least %s\n", goVersion, config.MinVersion)
		os.Exit(1)
	}

	buildTags := config.DefaultBuildTags

	skipNext := false
	params := os.Args[1:]

	goEnv := map[string]string{}
	buildEnv := map[string]string{
		"GOOS":   runtime.GOOS,
		"GOARCH": runtime.GOARCH,
		"GOARM":  "",
	}

	tempdir := ""

	var outputFilename string

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
			if i+1 >= len(params) {
				die("-t given but no tag specified")
			}
			skipNext = true
			buildTags = append(buildTags, strings.Split(params[i+1], " ")...)
		case "-o", "--output":
			skipNext = true
			outputFilename = params[i+1]
		case "--tempdir":
			skipNext = true
			tempdir = params[i+1]
		case "-T", "--test":
			runTests = true
		case "--enable-cgo":
			enableCGO = true
		case "--enable-pie":
			enablePIE = true
		case "--goos":
			skipNext = true
			buildEnv["GOOS"] = params[i+1]
		case "--goarch":
			skipNext = true
			buildEnv["GOARCH"] = params[i+1]
		case "--goarm":
			skipNext = true
			buildEnv["GOARM"] = params[i+1]
		case "-h":
			showUsage(os.Stdout)
			return
		default:
			fmt.Fprintf(os.Stderr, "Error: unknown option %q\n\n", arg)
			showUsage(os.Stderr)
			os.Exit(1)
		}
	}

	verbosePrintf("detected Go version %v\n", goVersion)

	for i := range buildTags {
		buildTags[i] = strings.TrimSpace(buildTags[i])
	}

	verbosePrintf("build tags: %s\n", buildTags)

	root, err := os.Getwd()
	if err != nil {
		die("Getwd(): %v\n", err)
	}

	if outputFilename == "" {
		outputFilename = config.Name
		if buildEnv["GOOS"] == "windows" {
			outputFilename += ".exe"
		}
	}

	output := outputFilename
	if !filepath.IsAbs(output) {
		output = filepath.Join(root, output)
	}

	version := getVersion()
	constants := Constants{}
	if version != "" {
		constants["main.version"] = version
	}
	ldflags := "-s -w " + constants.LDFlags()
	verbosePrintf("ldflags: %s\n", ldflags)

	var (
		buildArgs []string
		testArgs  []string
	)

	mainPackage := config.Main
	if strings.HasPrefix(mainPackage, config.Namespace) {
		mainPackage = strings.Replace(mainPackage, config.Namespace, "./", 1)
	}

	buildTarget := filepath.FromSlash(mainPackage)
	buildCWD := ""

	if goVersion.AtLeast(GoVersion{1, 11, 0}) && fileExists("go.mod") {
		verbosePrintf("Go >= 1.11 and 'go.mod' found, building with modules\n")
		buildCWD = root

		buildArgs = append(buildArgs, "-mod=vendor")
		testArgs = append(testArgs, "-mod=vendor")
	} else {
		if tempdir == "" {
			tempdir, err = ioutil.TempDir("", fmt.Sprintf("%v-build-", config.Name))
			if err != nil {
				die("TempDir(): %v\n", err)
			}
		}

		verbosePrintf("Go < 1.11 or 'go.mod' not found, create GOPATH at %v\n", tempdir)
		targetdir := filepath.Join(tempdir, "src", filepath.FromSlash(config.Namespace))
		if err = copy(targetdir, root); err != nil {
			die("copying files from %v to %v/src failed: %v\n", root, tempdir, err)
		}

		defer func() {
			if !keepGopath {
				verbosePrintf("remove %v\n", tempdir)
				if err = os.RemoveAll(tempdir); err != nil {
					die("remove GOPATH at %s failed: %v\n", tempdir, err)
				}
			} else {
				verbosePrintf("leaving temporary GOPATH at %v\n", tempdir)
			}
		}()

		buildCWD = targetdir

		goEnv["GOPATH"] = tempdir
		buildEnv["GOPATH"] = tempdir
	}

	verbosePrintf("environment:\n  go: %v\n  build: %v\n", goEnv, buildEnv)

	buildArgs = append(buildArgs,
		"-tags", strings.Join(buildTags, " "),
		"-ldflags", ldflags,
		"-o", output, buildTarget,
	)

	err = build(buildCWD, buildEnv, buildArgs...)
	if err != nil {
		die("build failed: %v\n", err)
	}

	if runTests {
		verbosePrintf("running tests\n")

		testArgs = append(testArgs, config.Tests...)

		err = test(buildCWD, goEnv, testArgs...)
		if err != nil {
			die("running tests failed: %v\n", err)
		}
	}
}
