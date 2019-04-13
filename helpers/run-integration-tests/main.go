package main

import (
	"archive/zip"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/pflag"
)

// MessagePrefix is printed before all messages
const MessagePrefix = "[integration tests] "

func die(f string, args ...interface{}) {
	if !strings.HasSuffix(f, "\n") {
		f += "\n"
	}
	f = MessagePrefix + f
	fmt.Fprintf(os.Stderr, f, args...)
	os.Exit(1)
}

func msg(f string, args ...interface{}) {
	if !strings.HasSuffix(f, "\n") {
		f += "\n"
	}
	f = MessagePrefix + f
	fmt.Printf(f, args...)
}

func warn(f string, args ...interface{}) {
	if !strings.HasSuffix(f, "\n") {
		f += "\n"
	}
	f = MessagePrefix + f
	fmt.Fprintf(os.Stderr, f, args...)
}

func verbose(f string, args ...interface{}) {
	if !opts.Verbose {
		return
	}
	if !strings.HasSuffix(f, "\n") {
		f += "\n"
	}
	f = MessagePrefix + f
	fmt.Printf(f, args...)
}

func rm(file string) {
	err := os.Remove(file)

	if os.IsNotExist(err) {
		err = nil
	}

	if err != nil {
		die("error removing %v: %v", file, err)
	}
}

func rmdir(dir string) {
	err := os.RemoveAll(dir)
	if err != nil {
		die("unable to remove %v: %v", dir, err)
	}
}

func mkdirall(dir string) {
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		die("mkdirall(%v) returned error: %v", dir, err)
	}
}

func mktemp(prefix string) *os.File {
	tempfile, err := ioutil.TempFile("", prefix)
	if err != nil {
		die("unable to create tempfile: %v", err)
	}
	return tempfile
}

func mktempdir(prefix string) string {
	tempdir, err := ioutil.TempDir("", prefix)
	if err != nil {
		die("unable to create tempdir: %v", err)
	}
	return tempdir
}

func run(cmd string, args ...string) {
	msg("run %s %s", cmd, strings.Join(args, " "))
	c := exec.Command(cmd, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	err := c.Run()
	if err != nil {
		die("error running %s %s: %v", cmd, strings.Join(args, " "), err)
	}
}

func runWithEnv(env []string, cmd string, args ...string) {
	msg("run %s %s, env %v", cmd, strings.Join(args, " "), env)

	cur := os.Environ()
	for _, v := range env {
		cur = append(cur, v)
	}

	c := exec.Command(cmd, args...)
	c.Env = cur
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	err := c.Run()
	if err != nil {
		die("error running %s %s: %v", cmd, strings.Join(args, " "), err)
	}
}

func runSilent(cmd string, args ...string) {
	msg("run %s %s", cmd, strings.Join(args, " "))
	start := time.Now()
	c := exec.Command(cmd, args...)
	buf, err := c.CombinedOutput()
	if err != nil {
		die("error running %s %s: %v, output:\n%s\n", cmd, strings.Join(args, " "), err, buf)
	}
	verbose("    this took %.3fs", time.Since(start).Seconds())
}

func runDiff(files ...string) {
	args := []string{"diff", "--exit-code"}
	args = append(args, files...)
	c := exec.Command("git", args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	err := c.Run()
	if err != nil {
		die("git diff returned an error, there were unexpected changes: %v", err)
	}
}

func download(url, targetfile string) {
	f, err := os.Create(targetfile)
	if err != nil {
		die("unable to open file %v: %v", targetfile, err)
	}

	res, err := http.Get(url)
	if err != nil {
		die("unable to request %v: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		die("unexpected status code for %v: %v (%v)", url, res.StatusCode, res.Status)
	}

	_, err = io.Copy(f, res.Body)
	if err != nil {
		die("error reading %v: %v", url, err)
	}

	err = res.Body.Close()
	if err != nil {
		die("error closing %v: %v", url, err)
	}

	err = f.Close()
	if err != nil {
		die("error closing %v: %v", targetfile, err)
	}
}

var externalPrograms = []string{
	"github.com/restic/rest-server/cmd/rest-server@master", // use the master branch for now
	"github.com/restic/calens",
}

func installPrograms(list []string) {
	// create a tempdir and run the program there so "go get" does not modify our go.mod/go.sum files
	tempdir := mktempdir("go-get-")

	for _, item := range list {
		verbose("install %v", item)

		start := time.Now()
		cmd := []string{"go", "get", item}
		msg("run %s", strings.Join(cmd, " "))
		c := exec.Command(cmd[0], cmd[1:]...)
		c.Dir = tempdir
		buf, err := c.CombinedOutput()
		if err != nil {
			die("error running %s: %v\n, output: %s\n", strings.Join(cmd, " "), err, buf)
		}
		msg("    installed %v in %.3fs", item, time.Since(start).Seconds())
	}
}

func installMinio(gobin string) {
	target := filepath.Join(gobin, "minio")
	var ext string
	if runtime.GOOS == "windows" {
		ext = ".exe"
		target += ".exe"
	}
	url := fmt.Sprintf("https://dl.minio.io/server/minio/release/%s-%s/minio%s", runtime.GOOS, runtime.GOARCH, ext)
	verbose("download minio from %v", url)
	start := time.Now()
	download(url, target)
	verbose("    downloaded minio in %.3fs", time.Since(start).Seconds())

	err := os.Chmod(target, 0755)
	if err != nil {
		die("chmod %v failed: %v", target, err)
	}
}

func createFileAt(filename string, mode os.FileMode, rd io.Reader) {
	mkdirall(filepath.Dir(filename))

	f, err := os.Create(filename)
	if err != nil {
		die("unable to create %v: %v", filename, err)
	}

	_, err = io.Copy(f, rd)
	if err != nil {
		die("write %v failed: %v", filename, err)
	}

	err = f.Close()
	if err != nil {
		die("closing %v returned error: %v", filename, err)
	}

	// make sure the file mode is not too wide
	mode &= 0755
	err = os.Chmod(filename, mode)
	if err != nil {
		die("chmod %v returned error: %v", filename, err)
	}
}

func installRclone(gobin string) {
	tempfile := mktemp("rcolne-download")
	err := tempfile.Close()
	if err != nil {
		die("error closing tempfile: %v", err)
	}
	defer func() {
		rm(tempfile.Name())
	}()

	os := runtime.GOOS
	if os == "darwin" {
		os = "osx"
	}

	url := fmt.Sprintf("https://downloads.rclone.org/rclone-current-%s-%s.zip", os, runtime.GOARCH)
	verbose("download rclone from %v", url)
	start := time.Now()
	download(url, tempfile.Name())

	target := filepath.Join(gobin, "rclone")
	if runtime.GOOS == "windows" {
		target += ".exe"
	}

	rd, err := zip.OpenReader(tempfile.Name())
	if err != nil {
		die("error opening zip %v: %v", tempfile.Name(), err)
	}

	found := false
	for _, item := range rd.File {
		if filepath.Base(item.Name) != filepath.Base(target) {
			continue
		}

		found = true

		f, err := item.Open()
		if err != nil {
			die("unable to open %v: %v", item.Name, err)
		}

		if !item.Mode().IsRegular() {
			die("invalid mode for %v in %v: got %v", item.Name, tempfile.Name())
		}

		createFileAt(target, item.Mode(), f)

		err = f.Close()
		if err != nil {
			die("error closing %v: %v", item.Name, err)
		}

		break
	}

	if !found {
		die("unable to find file %v in downloaded ZIP for rclone", filepath.Base(target))
	}

	err = rd.Close()
	if err != nil {
		die("error close %v: %v", tempfile.Name(), err)
	}

	verbose("extracted %v from %v in %.3fs", target, tempfile.Name(), time.Since(start).Seconds())
}

var opts struct {
	SkipCrossCompile    bool
	SkipCloudBackends   bool
	SkipProgramBackends bool
	SkipInstall         bool
	SkipTidy            bool
	SkipVendor          bool
	SkipChangelog       bool
	SkipFuse            bool
	SkipFormat          bool
	Verbose             bool
	binDir              string
}

func init() {
	pflag.BoolVar(&opts.SkipCrossCompile, "skip-cross-compile", false, "do not run cross-compilation tests")
	pflag.BoolVar(&opts.SkipCloudBackends, "skip-cloud-backends", false, "do not run tests for cloud backends")
	pflag.BoolVar(&opts.SkipProgramBackends, "skip-program-backends", false, "do not run tests for backends which require a program")
	pflag.BoolVar(&opts.SkipInstall, "skip-install", false, "do not install programs needed for tests")
	pflag.BoolVar(&opts.SkipTidy, "skip-tidy", false, "skip running 'go mod tidy'")
	pflag.BoolVar(&opts.SkipVendor, "skip-vendor", false, "skip running 'go mod vendor'")
	pflag.BoolVar(&opts.SkipChangelog, "skip-changelog", false, "skip checking the changelog files")
	pflag.BoolVar(&opts.SkipFuse, "skip-fuse", false, "skip testing the FUSE support")
	pflag.BoolVar(&opts.SkipFormat, "skip-format", false, "skip running `gofmt`")
	pflag.StringVar(&opts.binDir, "bin-dir", "", "install programs to `dir` (default: create tempdir)")
	pflag.BoolVar(&opts.Verbose, "verbose", false, "be verbose")
	pflag.Parse()
}

// CloudBackends contains a map of backend tests for cloud services to one
// of the essential environment variables which must be present in order to
// test it. We can then test if the environment variable is present and bail
// out early if it is not.
var CloudBackends = map[string]string{
	"restic/backend/s3.TestBackendS3":       "RESTIC_TEST_S3_REPOSITORY",
	"restic/backend/swift.TestBackendSwift": "RESTIC_TEST_SWIFT",
	"restic/backend/b2.TestBackendB2":       "RESTIC_TEST_B2_REPOSITORY",
	"restic/backend/gs.TestBackendGS":       "RESTIC_TEST_GS_REPOSITORY",
	"restic/backend/azure.TestBackendAzure": "RESTIC_TEST_AZURE_REPOSITORY",
}

// ProgramBackends contains a list of tests for backends which require running
// another program locally.
var ProgramBackends = []string{
	"restic/backend/rest.TestBackendREST",
	"restic/backend/sftp.TestBackendSFTP",
	"restic/backend/s3.TestBackendMinio",
	"restic/backend/rclone.TestBackendRclone",
}

func saveGCSCredentials(b64data string) (filename string) {
	if b64data == "" {
		return ""
	}

	buf, err := base64.StdEncoding.DecodeString(b64data)
	if err != nil {
		die("base64 decode failed: %v", err)
	}

	f, err := ioutil.TempFile("", "gcs-credentials-")
	if err != nil {
		die("tempfile: %v", err)
	}

	verbose("saving GCS credentials to %v\n", f.Name())

	_, err = f.Write(buf)
	if err != nil {
		f.Close()
		die("save GCS credentials to %v failed: %v", f.Name(), err)
	}

	err = f.Close()
	if err != nil {
		die("close %v failed: %v", err)
	}

	return f.Name()
}

func prepareEnvironment() {
	// make sure GCS credentials are available
	b64creds := os.Getenv("RESTIC_TEST_GS_APPLICATION_CREDENTIALS_B64")
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" && b64creds != "" {
		// extract the credentials for GCS into a temporary file
		gcsCredentialsFile := saveGCSCredentials(b64creds)
		os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", gcsCredentialsFile)
	}

	var ensure []string

	if !opts.SkipProgramBackends {
		// run local backends which require installing a program
		for _, backend := range ProgramBackends {
			ensure = append(ensure, backend)
		}
	}

	// make sure that cloud backends for which we have credentials are not
	// silently skipped.
	for pkg, env := range CloudBackends {
		if _, ok := os.LookupEnv(env); ok {
			verbose("env var %v is present, so ensure that %v is not skipped", env, pkg)
			ensure = append(ensure, pkg)
		} else {
			msg("credentials for %v are not available (environment variable %v unset), skipping\n", pkg, env)
		}
	}
	verbose("disallow skip: %v", ensure)
	os.Setenv("RESTIC_TEST_DISALLOW_SKIP", strings.Join(ensure, ","))

	if opts.SkipFuse {
		verbose("setting RESTIC_TEST_FUSE=0 to skip fuse tests")
		os.Setenv("RESTIC_TEST_FUSE", "0")
	}
}

// Target captures one OS and architecture we build for.
type Target struct {
	OS, Arch string
}

// BuildTargets contains all OS/arch combinations we cross-compile for during testing.
var BuildTargets = []Target{
	{"linux", "386"}, {"linux", "amd64"}, {"linux", "arm"}, {"linux", "arm64"},
	{"darwin", "386"}, {"darwin", "amd64"},
	{"windows", "386"}, {"windows", "amd64"},
	{"freebsd", "386"}, {"freebsd", "amd64"},
	{"openbsd", "386"}, {"openbsd", "amd64"},
	{"netbsd", "386"}, {"netbsd", "amd64"},
	{"solaris", "amd64"},
}

func buildAllTargets(env []string) {
	for _, target := range BuildTargets {
		msg("building %v/%v", target.OS, target.Arch)
		buildenv := env
		buildenv = append(buildenv, "GOOS="+target.OS, "GOARCH="+target.Arch)
		runWithEnv(buildenv, "go", "build", "-mod=vendor", "./cmd/restic")
	}
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

func runGofmt() {
	dir, err := os.Getwd()
	if err != nil {
		die("getwd returned error: %", err)
	}

	files, err := findGoFiles(dir)
	if err != nil {
		die("unable to find .go files: %v", err)
	}

	msg("run gofmt with %d files\n", len(files))
	args := append([]string{"-l"}, files...)
	cmd := exec.Command("gofmt", args...)
	cmd.Stderr = os.Stderr

	buf, err := cmd.Output()
	if err != nil {
		die("error running gofmt: %v, output:\n%s", err, buf)
	}

	if len(buf) > 0 {
		die("not formatted with `gofmt`:\n%s", buf)
	}
}

func main() {
	if opts.binDir == "" {
		opts.binDir = mktempdir("bin-dir-")
		defer func() {
			rmdir(opts.binDir)
		}()
	}

	// make sure Go does not download anything while building or running the tests
	testEnv := []string{
		"GOPROXY=off",
		"CGO_ENABLED=0",
	}

	// build for all architectures, catches compilation errors
	if !opts.SkipCrossCompile {
		buildAllTargets(testEnv)
	}

	// ensure that build.go works
	runWithEnv(testEnv, "go", "run", "-mod=vendor", "build.go")

	verbose("run integration tests on %v/%v", runtime.GOOS, runtime.GOARCH)

	// make sure "go install" uses the correct target directory
	gobin, err := filepath.Abs(opts.binDir)
	if err != nil {
		die("unable to find absolute path to %v: %v", opts.binDir, err)
	}
	verbose("install programs into %v", gobin)
	os.Setenv("GOBIN", gobin)

	// insert GOBIN into PATH
	os.Setenv("PATH", gobin+":"+os.Getenv("PATH"))

	if !opts.SkipInstall {
		installPrograms(externalPrograms)
		installMinio(opts.binDir)
		installRclone(opts.binDir)
	}

	// remove environment variables if we're not going to test cloud backends
	if opts.SkipCloudBackends {
		for _, name := range CloudBackends {
			_ = os.Unsetenv(name)
		}
	}

	prepareEnvironment()

	// run the tests
	msg("run tests")
	runWithEnv(testEnv, "go", "test", "-count", "1", "-mod=vendor", "-coverprofile", "all.cov", "./...")

	msg("run checks")
	if !opts.SkipTidy {
		// make sure the vendored dependencies and the go.mod/go.sum are up to date
		runSilent("go", "mod", "tidy")
		runDiff("go.sum", "go.mod")
	}
	if !opts.SkipVendor {
		run("go", "mod", "vendor")
		runDiff("vendor")
	}
	if !opts.SkipFormat {
		runGofmt()
		runDiff()
	}

	if !opts.SkipChangelog {
		// generate the changelog and check the files in changelog/
		msg("run calens")
		run("calens", "--output", "generated-changelog.md")
	}

	// TODO check forbidden imports
}
