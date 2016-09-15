// +build ignore

package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// ForbiddenImports are the packages from the stdlib that should not be used in
// our code.
var ForbiddenImports = map[string]bool{
	"errors": true,
}

var runCrossCompile = flag.Bool("cross-compile", true, "run cross compilation tests")
var minioServer = flag.String("minio", "", "path to the minio server binary")
var debug = flag.Bool("debug", false, "output debug messages")

var minioServerEnv = map[string]string{
	"MINIO_ACCESS_KEY": "KEBIYDZ87HCIH5D17YCN",
	"MINIO_SECRET_KEY": "bVX1KhipSBPopEfmhc7rGz8ooxx27xdJ7Gkh1mVe",
}

var minioEnv = map[string]string{
	"RESTIC_TEST_S3_SERVER": "http://127.0.0.1:9000",
	"AWS_ACCESS_KEY_ID":     "KEBIYDZ87HCIH5D17YCN",
	"AWS_SECRET_ACCESS_KEY": "bVX1KhipSBPopEfmhc7rGz8ooxx27xdJ7Gkh1mVe",
}

func init() {
	flag.Parse()
}

// CIEnvironment is implemented by environments where tests can be run.
type CIEnvironment interface {
	Prepare() error
	RunTests() error
	Teardown() error
}

// TravisEnvironment is the environment in which Travis tests run.
type TravisEnvironment struct {
	goxOSArch []string
	minio     string

	minioSrv     *Background
	minioTempdir string

	env map[string]string
}

func (env *TravisEnvironment) getMinio() error {
	if *minioServer != "" {
		msg("using minio server at %q\n", *minioServer)
		env.minio = *minioServer
		return nil
	}

	tempfile, err := ioutil.TempFile("", "minio-server-")
	if err != nil {
		return fmt.Errorf("create tempfile for minio download failed: %v\n", err)
	}

	url := fmt.Sprintf("https://dl.minio.io/server/minio/release/%s-%s/minio",
		runtime.GOOS, runtime.GOARCH)
	msg("downloading %v\n", url)
	res, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("error downloading minio server: %v\n", err)
	}

	_, err = io.Copy(tempfile, res.Body)
	if err != nil {
		return fmt.Errorf("error saving minio server to file: %v\n", err)
	}

	err = res.Body.Close()
	if err != nil {
		return fmt.Errorf("error closing HTTP download: %v\n", err)
	}

	err = tempfile.Close()
	if err != nil {
		msg("closing tempfile failed: %v\n", err)
		return fmt.Errorf("error closing minio server file: %v\n", err)
	}

	err = os.Chmod(tempfile.Name(), 0755)
	if err != nil {
		return fmt.Errorf("chmod(minio-server) failed: %v", err)
	}

	msg("downloaded minio server to %v\n", tempfile.Name())
	env.minio = tempfile.Name()
	return nil
}

func (env *TravisEnvironment) runMinio() error {
	if env.minio == "" {
		return nil
	}

	// start minio server
	msg("starting minio server at %s", env.minio)

	dir, err := ioutil.TempDir("", "minio-root")
	if err != nil {
		return fmt.Errorf("running minio server failed: %v", err)
	}

	env.minioSrv, err = StartBackgroundCommand(minioServerEnv, env.minio,
		"server",
		"--address", "127.0.0.1:9000",
		dir)
	if err != nil {
		return fmt.Errorf("error running minio server: %v", err)
	}

	// go func() {
	// 	time.Sleep(300 * time.Millisecond)
	// 	env.minioSrv.Cmd.Process.Kill()
	// }()

	for k, v := range minioEnv {
		env.env[k] = v
	}

	env.minioTempdir = dir
	return nil
}

// Prepare installs dependencies and starts services in order to run the tests.
func (env *TravisEnvironment) Prepare() error {
	env.env = make(map[string]string)

	msg("preparing environment for Travis CI\n")

	for _, pkg := range []string{
		"golang.org/x/tools/cmd/cover",
		"github.com/pierrre/gotestcover",
	} {
		err := run("go", "get", pkg)
		if err != nil {
			return err
		}
	}

	if err := env.getMinio(); err != nil {
		return err
	}
	if err := env.runMinio(); err != nil {
		return err
	}

	if *runCrossCompile && !(runtime.Version() < "go1.7") {
		// only test cross compilation on linux with Travis
		if err := run("go", "get", "github.com/mitchellh/gox"); err != nil {
			return err
		}
		if runtime.GOOS == "linux" {
			env.goxOSArch = []string{
				"linux/386", "linux/amd64",
				"windows/386", "windows/amd64",
				"darwin/386", "darwin/amd64",
				"freebsd/386", "freebsd/amd64",
				"opendbsd/386", "opendbsd/amd64",
			}
			if !strings.HasPrefix(runtime.Version(), "go1.3") {
				env.goxOSArch = append(env.goxOSArch,
					"linux/arm", "freebsd/arm")
			}
		} else {
			env.goxOSArch = []string{runtime.GOOS + "/" + runtime.GOARCH}
		}

		msg("gox: OS/ARCH %v\n", env.goxOSArch)

		if runtime.Version() < "go1.5" {
			err := run("gox", "-build-toolchain",
				"-osarch", strings.Join(env.goxOSArch, " "))

			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Teardown stops backend services and cleans the environment again.
func (env *TravisEnvironment) Teardown() error {
	msg("run travis teardown\n")
	if env.minioSrv != nil {
		msg("stopping minio server\n")

		if env.minioSrv.Cmd.ProcessState == nil {
			err := env.minioSrv.Cmd.Process.Kill()
			if err != nil {
				fmt.Fprintf(os.Stderr, "error killing minio server process: %v", err)
			}
		} else {
			result := <-env.minioSrv.Result
			if result.Error != nil {
				msg("minio server returned error: %v\n", result.Error)
				msg("stdout: %s\n", result.Stdout)
				msg("stderr: %s\n", result.Stderr)
			}
		}

		err := os.RemoveAll(env.minioTempdir)
		if err != nil {
			msg("error removing minio tempdir %v: %v\n", env.minioTempdir, err)
		}
	}

	return nil
}

func goVersionAtLeast151() bool {
	v := runtime.Version()

	if match, _ := regexp.MatchString(`^go1\.[0-4]`, v); match {
		return false
	}

	if v == "go1.5" {
		return false
	}

	return true
}

// Background is a program running in the background.
type Background struct {
	Cmd    *exec.Cmd
	Result chan Result
}

// Result is the result of a program that ran in the background.
type Result struct {
	Stdout, Stderr string
	Error          error
}

// StartBackgroundCommand runs a program in the background.
func StartBackgroundCommand(env map[string]string, cmd string, args ...string) (*Background, error) {
	msg("running background command %v %v\n", cmd, args)
	b := Background{
		Result: make(chan Result, 1),
	}

	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)

	c := exec.Command(cmd, args...)
	c.Stdout = stdout
	c.Stderr = stderr

	if *debug {
		c.Stdout = io.MultiWriter(c.Stdout, os.Stdout)
		c.Stderr = io.MultiWriter(c.Stderr, os.Stderr)
	}
	c.Env = updateEnv(os.Environ(), env)

	b.Cmd = c

	err := c.Start()
	if err != nil {
		msg("error starting background job %v: %v\n", cmd, err)
		return nil, err
	}

	go func() {
		err := b.Cmd.Wait()
		msg("background job %v returned: %v\n", cmd, err)
		msg("stdout: %s\n", stdout.Bytes())
		msg("stderr: %s\n", stderr.Bytes())
		b.Result <- Result{
			Stdout: string(stdout.Bytes()),
			Stderr: string(stderr.Bytes()),
			Error:  err,
		}
	}()

	return &b, nil
}

// RunTests starts the tests for Travis.
func (env *TravisEnvironment) RunTests() error {
	// do not run fuse tests on darwin
	if runtime.GOOS == "darwin" {
		msg("skip fuse integration tests on %v\n", runtime.GOOS)
		os.Setenv("RESTIC_TEST_FUSE", "0")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() returned error: %v", err)
	}

	env.env["GOPATH"] = cwd + ":" + filepath.Join(cwd, "vendor")

	if *runCrossCompile && !(runtime.Version() < "go1.7") {
		// compile for all target architectures with tags
		for _, tags := range []string{"release", "debug"} {
			err := runWithEnv(env.env, "gox", "-verbose",
				"-osarch", strings.Join(env.goxOSArch, " "),
				"-tags", tags,
				"-output", "/tmp/{{.Dir}}_{{.OS}}_{{.Arch}}",
				"cmds/restic")
			if err != nil {
				return err
			}
		}
	}

	// run the build script
	if err := run("go", "run", "build.go"); err != nil {
		return err
	}

	// run the tests and gather coverage information
	err = runWithEnv(env.env, "gotestcover", "-coverprofile", "all.cov", "cmds/...", "restic/...")
	if err != nil {
		return err
	}

	if err = runGofmt(); err != nil {
		return err
	}

	deps, err := findImports()
	if err != nil {
		return err
	}

	foundForbiddenImports := false
	for name, imports := range deps {
		for _, pkg := range imports {
			if _, ok := ForbiddenImports[pkg]; ok {
				fmt.Fprintf(os.Stderr, "========== package %v imports forbidden package %v\n", name, pkg)
				foundForbiddenImports = true
			}
		}
	}

	if foundForbiddenImports {
		return errors.New("CI: forbidden imports found")
	}

	return nil
}

// AppveyorEnvironment is the environment on Windows.
type AppveyorEnvironment struct{}

// Prepare installs dependencies and starts services in order to run the tests.
func (env *AppveyorEnvironment) Prepare() error {
	msg("preparing environment for Appveyor CI\n")
	return nil
}

// RunTests start the tests.
func (env *AppveyorEnvironment) RunTests() error {
	return run("go", "run", "build.go", "-v", "-T")
}

// Teardown is a noop.
func (env *AppveyorEnvironment) Teardown() error {
	return nil
}

// findGoFiles returns a list of go source code file names below dir.
func findGoFiles(dir string) (list []string, err error) {
	err = filepath.Walk(dir, func(name string, fi os.FileInfo, err error) error {
		if filepath.Base(name) == "vendor" {
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

func updateEnv(env []string, override map[string]string) []string {
	var newEnv []string
	for _, s := range env {
		d := strings.SplitN(s, "=", 2)
		key := d[0]

		if _, ok := override[key]; ok {
			continue
		}

		newEnv = append(newEnv, s)
	}

	for k, v := range override {
		newEnv = append(newEnv, k+"="+v)
	}

	return newEnv
}

func findImports() (map[string][]string, error) {
	res := make(map[string][]string)

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("Getwd() returned error: %v", err)
	}

	gopath := cwd + ":" + filepath.Join(cwd, "vendor")

	cmd := exec.Command("go", "list", "-f", `{{.ImportPath}} {{join .Imports " "}}`, "./src/...")
	cmd.Env = updateEnv(os.Environ(), map[string]string{"GOPATH": gopath})
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	sc := bufio.NewScanner(bytes.NewReader(output))
	for sc.Scan() {
		wordScanner := bufio.NewScanner(strings.NewReader(sc.Text()))
		wordScanner.Split(bufio.ScanWords)

		if !wordScanner.Scan() {
			return nil, fmt.Errorf("package name not found in line: %s", output)
		}
		name := wordScanner.Text()
		var deps []string

		for wordScanner.Scan() {
			deps = append(deps, wordScanner.Text())
		}

		res[name] = deps
	}

	return res, nil
}

func runGofmt() error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd(): %v\n", err)
	}

	files, err := findGoFiles(dir)
	if err != nil {
		return fmt.Errorf("error finding Go files: %v\n", err)
	}

	msg("runGofmt() with %d files\n", len(files))
	args := append([]string{"-l"}, files...)
	cmd := exec.Command("gofmt", args...)
	cmd.Stderr = os.Stderr

	buf, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("error running gofmt: %v\noutput: %s\n", err, buf)
	}

	if len(buf) > 0 {
		return fmt.Errorf("not formatted with `gofmt`:\n%s\n", buf)
	}

	return nil
}

func run(command string, args ...string) error {
	msg("run %v %v\n", command, strings.Join(args, " "))
	return runWithEnv(nil, command, args...)
}

// runWithEnv calls a command with the current environment, except the entries
// of the env map are set additionally.
func runWithEnv(env map[string]string, command string, args ...string) error {
	msg("runWithEnv %v %v\n", command, strings.Join(args, " "))
	cmd := exec.Command(command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if env != nil {
		cmd.Env = updateEnv(os.Environ(), env)
	}
	err := cmd.Run()

	if err != nil {
		return fmt.Errorf("error running %v %v: %v",
			command, strings.Join(args, " "), err)
	}
	return nil
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

	foundError := false
	for _, f := range []func() error{env.Prepare, env.RunTests, env.Teardown} {
		err := f()
		if err != nil {
			foundError = true
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
	}

	if foundError {
		os.Exit(1)
	}
}
