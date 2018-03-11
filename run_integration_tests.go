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
	"strings"
)

// ForbiddenImports are the packages from the stdlib that should not be used in
// our code.
var ForbiddenImports = map[string]bool{
	"errors": true,
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
		"golang.org/x/tools/cmd/cover",
		"github.com/pierrre/gotestcover",
		"github.com/NebulousLabs/glyphcheck",
		"github.com/golang/dep/cmd/dep",
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
				"linux/arm", "freebsd/arm",
			}
		} else {
			env.goxOSArch = []string{runtime.GOOS + "/" + runtime.GOARCH}
		}

		msg("gox: OS/ARCH %v\n", env.goxOSArch)
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
	// do not run fuse tests on darwin
	if runtime.GOOS == "darwin" {
		msg("skip fuse integration tests on %v\n", runtime.GOOS)
		_ = os.Setenv("RESTIC_TEST_FUSE", "0")
	}

	env.env["GOPATH"] = os.Getenv("GOPATH")
	if env.gcsCredentialsFile != "" {
		env.env["GOOGLE_APPLICATION_CREDENTIALS"] = env.gcsCredentialsFile
	}

	// ensure that the following tests cannot be silently skipped on Travis
	ensureTests := []string{
		"restic/backend/rest.TestBackendREST",
		"restic/backend/sftp.TestBackendSFTP",
		"restic/backend/s3.TestBackendMinio",
	}

	// if the test s3 repository is available, make sure that the test is not skipped
	if os.Getenv("RESTIC_TEST_S3_REPOSITORY") != "" {
		ensureTests = append(ensureTests, "restic/backend/s3.TestBackendS3")
	} else {
		msg("S3 repository not available\n")
	}

	// if the test swift service is available, make sure that the test is not skipped
	if os.Getenv("RESTIC_TEST_SWIFT") != "" {
		ensureTests = append(ensureTests, "restic/backend/swift.TestBackendSwift")
	} else {
		msg("Swift service not available\n")
	}

	// if the test b2 repository is available, make sure that the test is not skipped
	if os.Getenv("RESTIC_TEST_B2_REPOSITORY") != "" {
		ensureTests = append(ensureTests, "restic/backend/b2.TestBackendB2")
	} else {
		msg("B2 repository not available\n")
	}

	// if the test gs repository is available, make sure that the test is not skipped
	if os.Getenv("RESTIC_TEST_GS_REPOSITORY") != "" {
		ensureTests = append(ensureTests, "restic/backend/gs.TestBackendGS")
	} else {
		msg("GS repository not available\n")
	}

	env.env["RESTIC_TEST_DISALLOW_SKIP"] = strings.Join(ensureTests, ",")

	if *runCrossCompile {
		// compile for all target architectures with tags
		for _, tags := range []string{"release", "debug"} {
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

	// run the build script
	if err := run("go", "run", "build.go"); err != nil {
		return err
	}

	// run the tests and gather coverage information
	err := runWithEnv(env.env, "gotestcover", "-coverprofile", "all.cov", "github.com/restic/restic/cmd/...", "github.com/restic/restic/internal/...")
	if err != nil {
		return err
	}

	if err = runGofmt(); err != nil {
		return err
	}

	if err = runDep(); err != nil {
		return err
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

func runDep() error {
	cmd := exec.Command("dep", "ensure", "-no-vendor", "-dry-run")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error running dep: %v\nThis probably means that Gopkg.lock is not up to date, run 'dep ensure' and commit all changes", err)
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
