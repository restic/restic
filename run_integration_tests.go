// +build ignore

package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// ForbiddenImports are the packages from the stdlib that should not be used in
// our code.
var ForbiddenImports = map[string]bool{
	"errors": true,
}

// Use a specific version of gofmt (the latest stable, usually) to guarantee
// deterministic formatting. This is used with the GoVersion.AtLeast()
// function (so that we don't forget to update it).
var GofmtVersion = ParseGoVersion("go1.11")

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

// CloudBackends contains a map of backend tests for cloud services to one
// of the essential environment variables which must be present in order to
// test it.
var CloudBackends = map[string]string{
	"restic/backend/s3.TestBackendS3":       "RESTIC_TEST_S3_REPOSITORY",
	"restic/backend/swift.TestBackendSwift": "RESTIC_TEST_SWIFT",
	"restic/backend/b2.TestBackendB2":       "RESTIC_TEST_B2_REPOSITORY",
	"restic/backend/gs.TestBackendGS":       "RESTIC_TEST_GS_REPOSITORY",
	"restic/backend/azure.TestBackendAzure": "RESTIC_TEST_AZURE_REPOSITORY",
}

var runCrossCompile = flag.Bool("cross-compile", true, "run cross compilation tests")

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
	goxOSArch          []string
	env                map[string]string
	gcsCredentialsFile string
}

func (env *TravisEnvironment) getMinio() error {
	tempfile, err := os.Create(filepath.Join(os.Getenv("GOPATH"), "bin", "minio"))
	if err != nil {
		return fmt.Errorf("create tempfile for minio download failed: %v", err)
	}

	url := fmt.Sprintf("https://dl.minio.io/server/minio/release/%s-%s/minio",
		runtime.GOOS, runtime.GOARCH)
	msg("downloading %v\n", url)
	res, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("error downloading minio server: %v", err)
	}

	_, err = io.Copy(tempfile, res.Body)
	if err != nil {
		return fmt.Errorf("error saving minio server to file: %v", err)
	}

	err = res.Body.Close()
	if err != nil {
		return fmt.Errorf("error closing HTTP download: %v", err)
	}

	err = tempfile.Close()
	if err != nil {
		msg("closing tempfile failed: %v\n", err)
		return fmt.Errorf("error closing minio server file: %v", err)
	}

	err = os.Chmod(tempfile.Name(), 0755)
	if err != nil {
		return fmt.Errorf("chmod(minio-server) failed: %v", err)
	}

	msg("downloaded minio server to %v\n", tempfile.Name())
	return nil
}

// Prepare installs dependencies and starts services in order to run the tests.
func (env *TravisEnvironment) Prepare() error {
	env.env = make(map[string]string)

	msg("preparing environment for Travis CI\n")

	pkgs := []string{
		"github.com/NebulousLabs/glyphcheck",
		"github.com/restic/rest-server/cmd/rest-server",
		"github.com/restic/calens",
		"github.com/ncw/rclone",
	}

	for _, pkg := range pkgs {
		err := run("go", "get", pkg)
		if err != nil {
			return err
		}
	}

	if err := env.getMinio(); err != nil {
		return err
	}

	if *runCrossCompile {
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
				"openbsd/386", "openbsd/amd64",
				"netbsd/386", "netbsd/amd64",
				"linux/arm", "freebsd/arm",
			}

			if os.Getenv("RESTIC_BUILD_SOLARIS") == "0" {
				msg("Skipping Solaris build\n")
			} else {
				env.goxOSArch = append(env.goxOSArch, "solaris/amd64")
			}
		} else {
			env.goxOSArch = []string{runtime.GOOS + "/" + runtime.GOARCH}
		}

		msg("gox: OS/ARCH %v\n", env.goxOSArch)
	}

	// do not run cloud tests on darwin
	if os.Getenv("RESTIC_TEST_CLOUD_BACKENDS") == "0" {
		msg("skipping cloud backend tests\n")

		for _, name := range CloudBackends {
			err := os.Unsetenv(name)
			if err != nil {
				msg("    error unsetting %v: %v\n", name, err)
			}
		}
	}

	// extract credentials file for GCS tests
	if b64data := os.Getenv("RESTIC_TEST_GS_APPLICATION_CREDENTIALS_B64"); b64data != "" {
		buf, err := base64.StdEncoding.DecodeString(b64data)
		if err != nil {
			return err
		}

		f, err := ioutil.TempFile("", "gcs-credentials-")
		if err != nil {
			return err
		}

		msg("saving GCS credentials to %v\n", f.Name())

		_, err = f.Write(buf)
		if err != nil {
			f.Close()
			return err
		}

		env.gcsCredentialsFile = f.Name()

		if err = f.Close(); err != nil {
			return err
		}
	}

	return nil
}

// Teardown stops backend services and cleans the environment again.
func (env *TravisEnvironment) Teardown() error {
	msg("run travis teardown\n")

	if env.gcsCredentialsFile != "" {
		msg("remove gcs credentials file %v\n", env.gcsCredentialsFile)
		return os.Remove(env.gcsCredentialsFile)
	}

	return nil
}

// RunTests starts the tests for Travis.
func (env *TravisEnvironment) RunTests() error {
	env.env["GOPATH"] = os.Getenv("GOPATH")
	if env.gcsCredentialsFile != "" {
		env.env["GOOGLE_APPLICATION_CREDENTIALS"] = env.gcsCredentialsFile
	}

	// ensure that the following tests cannot be silently skipped on Travis
	ensureTests := []string{
		"restic/backend/rest.TestBackendREST",
		"restic/backend/sftp.TestBackendSFTP",
		"restic/backend/s3.TestBackendMinio",
		"restic/backend/rclone.TestBackendRclone",
	}

	// make sure that cloud backends for which we have credentials are not
	// silently skipped.
	for pkg, env := range CloudBackends {
		if _, ok := os.LookupEnv(env); ok {
			ensureTests = append(ensureTests, pkg)
		} else {
			msg("credentials for %v are not available, skipping\n", pkg)
		}
	}

	env.env["RESTIC_TEST_DISALLOW_SKIP"] = strings.Join(ensureTests, ",")

	if *runCrossCompile {
		// compile for all target architectures with tags
		for _, tags := range []string{"", "debug"} {
			err := runWithEnv(env.env, "gox", "-verbose",
				"-osarch", strings.Join(env.goxOSArch, " "),
				"-tags", tags,
				"-output", "/tmp/{{.Dir}}_{{.OS}}_{{.Arch}}",
				"./cmd/restic")
			if err != nil {
				return err
			}
		}
	}

	args := []string{"go", "run", "build.go"}
	v := ParseGoVersion(runtime.Version())
	msg("Detected Go version %v\n", v)
	if v.AtLeast(GoVersion{1, 11, 0}) {
		args = []string{"go", "run", "-mod=vendor", "build.go"}
		env.env["GOPROXY"] = "off"
		delete(env.env, "GOPATH")
		os.Unsetenv("GOPATH")
	}

	// run the build script
	err := run(args[0], args[1:]...)
	if err != nil {
		return err
	}

	// run the tests and gather coverage information (for Go >= 1.10)
	switch {
	case v.AtLeast(GoVersion{1, 11, 0}):
		err = runWithEnv(env.env, "go", "test", "-count", "1", "-mod=vendor", "-coverprofile", "all.cov", "./...")
	case v.AtLeast(GoVersion{1, 10, 0}):
		err = runWithEnv(env.env, "go", "test", "-count", "1", "-coverprofile", "all.cov", "./...")
	default:
		err = runWithEnv(env.env, "go", "test", "-count", "1", "./...")
	}
	if err != nil {
		return err
	}

	// only run gofmt on a specific version of Go.
	if v.AtLeast(GofmtVersion) {
		if err = runGofmt(); err != nil {
			return err
		}

		msg("run go mod vendor\n")
		if err := runGoModVendor(); err != nil {
			return err
		}

		msg("run go mod tidy\n")
		if err := runGoModTidy(); err != nil {
			return err
		}
	} else {
		msg("Skipping gofmt and module vendor check for %v\n", v)
	}

	if err = runGlyphcheck(); err != nil {
		return err
	}

	// check for forbidden imports
	deps, err := env.findImports()
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

	// check that the entries in changelog/ are valid
	if err := run("calens"); err != nil {
		return errors.New("calens failed, files in changelog/ are not valid")
	}

	return nil
}

// AppveyorEnvironment is the environment on Windows.
type AppveyorEnvironment struct{}

// Prepare installs dependencies and starts services in order to run the tests.
func (env *AppveyorEnvironment) Prepare() error {
	return nil
}

// RunTests start the tests.
func (env *AppveyorEnvironment) RunTests() error {
	e := map[string]string{
		"GOPROXY": "off",
	}
	return runWithEnv(e, "go", "run", "-mod=vendor", "build.go", "-v", "-T")
}

// Teardown is a noop.
func (env *AppveyorEnvironment) Teardown() error {
	return nil
}

// findGoFiles returns a list of go source code file names below dir.
func findGoFiles(dir string) (list []string, err error) {
	err = filepath.Walk(dir, func(name string, fi os.FileInfo, err error) error {
		relpath, err := filepath.Rel(dir, name)
		if err != nil {
			return err
		}

		if relpath == "vendor" || relpath == "pkg" {
			return filepath.SkipDir
		}

		if filepath.Ext(relpath) == ".go" {
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

func (env *TravisEnvironment) findImports() (map[string][]string, error) {
	res := make(map[string][]string)

	cmd := exec.Command("go", "list", "-f", `{{.ImportPath}} {{join .Imports " "}}`, "./internal/...", "./cmd/...")
	cmd.Env = updateEnv(os.Environ(), env.env)
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
		return fmt.Errorf("Getwd(): %v", err)
	}

	files, err := findGoFiles(dir)
	if err != nil {
		return fmt.Errorf("error finding Go files: %v", err)
	}

	msg("runGofmt() with %d files\n", len(files))
	args := append([]string{"-l"}, files...)
	cmd := exec.Command("gofmt", args...)
	cmd.Stderr = os.Stderr

	buf, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("error running gofmt: %v\noutput: %s", err, buf)
	}

	if len(buf) > 0 {
		return fmt.Errorf("not formatted with `gofmt`:\n%s", buf)
	}

	return nil
}

func runGoModVendor() error {
	cmd := exec.Command("go", "mod", "vendor")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Env = updateEnv(os.Environ(), map[string]string{
		"GO111MODULE": "on",
	})

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error running 'go mod vendor': %v", err)
	}

	// check that "git diff" does not return any output
	cmd = exec.Command("git", "diff", "vendor")
	cmd.Stderr = os.Stderr

	buf, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("error running 'git diff vendor': %v\noutput: %s", err, buf)
	}

	if len(buf) > 0 {
		return fmt.Errorf("vendor/ directory was modified:\n%s", buf)
	}

	return nil
}

// run "go mod tidy" so that go.sum and go.mod are updated to reflect all
// dependencies for all OS/Arch combinations, see
// https://github.com/golang/go/wiki/Modules#why-does-go-mod-tidy-put-so-many-indirect-dependencies-in-my-gomod
func runGoModTidy() error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Env = updateEnv(os.Environ(), map[string]string{
		"GO111MODULE": "on",
	})

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error running 'go mod vendor': %v", err)
	}

	// check that "git diff" does not return any output
	cmd = exec.Command("git", "diff", "go.sum", "go.mod")
	cmd.Stderr = os.Stderr

	buf, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("error running 'git diff vendor': %v\noutput: %s", err, buf)
	}

	if len(buf) > 0 {
		return fmt.Errorf("vendor/ directory was modified:\n%s", buf)
	}

	return nil
}

func runGlyphcheck() error {
	cmd := exec.Command("glyphcheck", "./cmd/...", "./internal/...")
	cmd.Stderr = os.Stderr

	buf, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("error running glyphcheck: %v\noutput: %s", err, buf)
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

	err := env.Prepare()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error preparing: %v\n", err)
		os.Exit(1)
	}

	err = env.RunTests()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error running tests: %v\n", err)
		os.Exit(2)
	}

	err = env.Teardown()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error during teardown: %v\n", err)
		os.Exit(3)
	}
}
