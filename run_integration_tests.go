// +build ignore

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type CIEnvironment interface {
	Prepare()
	RunTests()
}

type TravisEnvironment struct {
	goxArch []string
	goxOS   []string
}

func (env *TravisEnvironment) Prepare() {
	msg("preparing environment for Travis CI\n")

	run("go", "get", "golang.org/x/tools/cmd/cover")
	run("go", "get", "github.com/mattn/goveralls")
	run("go", "get", "github.com/mitchellh/gox")

	if runtime.GOOS == "darwin" {
		// install the libraries necessary for fuse
		run("brew", "update")
		run("brew", "install", "caskroom/cask/brew-cask")
		run("brew", "cask", "install", "osxfuse")
	}

	// only test cross compilation on linux with Travis
	if runtime.GOOS == "linux" {
		env.goxArch = []string{"386", "amd64"}
		if !strings.HasPrefix(runtime.Version(), "go1.3") {
			env.goxArch = append(env.goxArch, "arm")
		}

		env.goxOS = []string{"linux", "darwin", "freebsd", "openbsd", "windows"}
	} else {
		env.goxArch = []string{runtime.GOARCH}
		env.goxOS = []string{runtime.GOOS}
	}

	msg("gox: OS %v, ARCH %v\n", env.goxOS, env.goxArch)

	if !strings.HasPrefix(runtime.Version(), "go1.5") {
		run("gox", "-build-toolchain",
			"-os", strings.Join(env.goxOS, " "),
			"-arch", strings.Join(env.goxArch, " "))
	}
}

func (env *TravisEnvironment) RunTests() {
	// run fuse tests on darwin
	if runtime.GOOS != "darwin" {
		msg("skip fuse integration tests on %v\n", runtime.GOOS)
		os.Setenv("RESTIC_TEST_FUSE", "0")
	}

	// compile for all target architectures with tags
	for _, tags := range []string{"release", "debug"} {
		run("gox", "-verbose",
			"-os", strings.Join(env.goxOS, " "),
			"-arch", strings.Join(env.goxArch, " "),
			"-tags", tags,
			"./cmd/restic")
	}

	// run the build script
	run("go", "run", "build.go")

	// gather coverage information
	run("go", "run", "run_tests.go", "all.cov")

	runGofmt()
}

type AppveyorEnvironment struct{}

func (env *AppveyorEnvironment) Prepare() {
	msg("preparing environment for Appveyor CI\n")
}

func (env *AppveyorEnvironment) RunTests() {
	run("go", "run", "build.go", "-v", "-T")
}

// findGoFiles returns a list of go source code file names below dir.
func findGoFiles(dir string) (list []string, err error) {
	err = filepath.Walk(dir, func(name string, fi os.FileInfo, err error) error {
		if filepath.Base(name) == "Godeps" {
			return filepath.SkipDir
		}

		if filepath.Ext(name) == ".go" {
			relpath, err := filepath.Rel(dir, name)
			if err != nil {
				return err
			}

			list = append(list, relpath)
		}

		return err
	})

	return list, err
}

func msg(format string, args ...interface{}) {
	fmt.Printf("CI: "+format, args...)
}

func runGofmt() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Getwd(): %v\n", err)
		os.Exit(5)
	}

	files, err := findGoFiles(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error finding Go files: %v\n", err)
		os.Exit(4)
	}

	msg("runGofmt() with %d files\n", len(files))
	args := append([]string{"-l"}, files...)
	cmd := exec.Command("gofmt", args...)
	cmd.Stderr = os.Stderr

	buf, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error running gofmt: %v", err)
		fmt.Fprintf(os.Stderr, "output:\n%s\n", buf)
		os.Exit(3)
	}

	if len(buf) > 0 {
		fmt.Fprintf(os.Stderr, "not formatted with `gofmt`:\n")
		fmt.Fprintln(os.Stderr, string(buf))
		os.Exit(6)
	}
}

func run(command string, args ...string) {
	msg("run %v %v\n", command, strings.Join(args, " "))
	cmd := exec.Command(command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()

	if err != nil {
		fmt.Fprintf(os.Stderr, "error running %v %v: %v",
			command, strings.Join(args, " "), err)
		os.Exit(3)
	}
}

func isTravis() bool {
	return os.Getenv("TRAVIS_BUILD_DIR") != ""
}

func isAppveyor() bool {
	return runtime.GOOS == "windows"
}

func main() {
	var env CIEnvironment

	switch {
	case isTravis():
		env = &TravisEnvironment{}
	case isAppveyor():
		env = &AppveyorEnvironment{}
	default:
		fmt.Fprintln(os.Stderr, "unknown CI environment")
		os.Exit(1)
	}

	for _, f := range []func(){env.Prepare, env.RunTests} {
		f()
	}
}
