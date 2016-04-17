// +build ignore

package main

import (
	"bytes"
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

var runCrossCompile = flag.Bool("cross-compile", true, "run cross compilation tests")
var minioServer = flag.String("minio", "", "path to the minio server binary")

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

type CIEnvironment interface {
	Prepare()
	RunTests()
	Teardown()
}

type TravisEnvironment struct {
	goxArch []string
	goxOS   []string
	minio   string

	minioSrv     *Background
	minioTempdir string

	env map[string]string
}

func (env *TravisEnvironment) getMinio() {
	if *minioServer != "" {
		msg("using minio server at %q\n", *minioServer)
		env.minio = *minioServer
		return
	}

	tempfile, err := ioutil.TempFile("", "minio-server-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create tempfile failed: %v\n", err)
		os.Exit(10)
	}

	url := fmt.Sprintf("https://dl.minio.io/server/minio/release/%s-%s/minio",
		runtime.GOOS, runtime.GOARCH)
	msg("downloading %v\n", url)
	res, err := http.Get(url)
	if err != nil {
		msg("downloading minio failed: %v\n", err)
		return
	}

	_, err = io.Copy(tempfile, res.Body)
	if err != nil {
		msg("downloading minio failed: %v\n", err)
		return
	}

	err = res.Body.Close()
	if err != nil {
		msg("saving minio failed: %v\n", err)
		return
	}

	err = tempfile.Close()
	if err != nil {
		msg("closing tempfile failed: %v\n", err)
		return
	}

	err = os.Chmod(tempfile.Name(), 0755)
	if err != nil {
		msg("making minio server executable failed: %v\n", err)
		return
	}

	msg("downloaded minio server to %v\n", tempfile.Name())
	env.minio = tempfile.Name()
}

func (env *TravisEnvironment) runMinio() {
	// start minio server
	if env.minio != "" {
		msg("starting minio server at %s", env.minio)

		dir, err := ioutil.TempDir("", "minio-root")
		if err != nil {
			os.Exit(80)
		}

		env.minioSrv, err = StartBackgroundCommand(minioServerEnv, env.minio,
			"server",
			"--address", "127.0.0.1:9000",
			dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error running minio server: %v", err)
			os.Exit(80)
		}

		for k, v := range minioEnv {
			env.env[k] = v
		}

		env.minioTempdir = dir
	}
}

func (env *TravisEnvironment) Prepare() {
	env.env = make(map[string]string)

	msg("preparing environment for Travis CI\n")

	run("go", "get", "golang.org/x/tools/cmd/cover")
	run("go", "get", "github.com/mattn/goveralls")
	run("go", "get", "github.com/pierrre/gotestcover")
	env.getMinio()
	env.runMinio()

	if runtime.GOOS == "darwin" {
		// install the libraries necessary for fuse
		run("brew", "update")
		run("brew", "cask", "install", "osxfuse")
	}

	if *runCrossCompile {
		// only test cross compilation on linux with Travis
		run("go", "get", "github.com/mitchellh/gox")
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

		v := runtime.Version()
		if !strings.HasPrefix(v, "go1.5") && !strings.HasPrefix(v, "go1.6") {
			run("gox", "-build-toolchain",
				"-os", strings.Join(env.goxOS, " "),
				"-arch", strings.Join(env.goxArch, " "))
		}
	}
}

func (env *TravisEnvironment) Teardown() {
	msg("run travis teardown\n")
	if env.minioSrv != nil {
		msg("stopping minio server\n")
		result := <-env.minioSrv.Result
		if result.Error != nil {
			msg("minio server returned error: %v\n", result.Error)
			msg("stdout: %s\n", result.Stdout)
			msg("stderr: %s\n", result.Stderr)
		}

		err := os.RemoveAll(env.minioTempdir)
		if err != nil {
			msg("error removing minio tempdir %v: %v\n", env.minioTempdir, err)
		}
	}
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

type Background struct {
	Cmd    *exec.Cmd
	Done   chan struct{}
	Result chan Result
}

type Result struct {
	Stdout, Stderr string
	Error          error
}

func StartBackgroundCommand(env map[string]string, cmd string, args ...string) (*Background, error) {
	msg("running background command %v %v\n", cmd, args)
	b := Background{
		Done:   make(chan struct{}),
		Result: make(chan Result, 1),
	}

	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)

	c := exec.Command(cmd, args...)
	c.Stdout = stdout
	c.Stderr = stderr
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
		b.Result <- Result{
			Stdout: string(stdout.Bytes()),
			Stderr: string(stderr.Bytes()),
			Error:  err,
		}
	}()

	return &b, nil
}

func (env *TravisEnvironment) RunTests() {
	// run fuse tests on darwin
	if runtime.GOOS != "darwin" {
		msg("skip fuse integration tests on %v\n", runtime.GOOS)
		os.Setenv("RESTIC_TEST_FUSE", "0")
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Getwd() returned error: %v", err)
		os.Exit(9)
	}

	env.env["GOPATH"] = cwd + ":" + filepath.Join(cwd, "vendor")

	if *runCrossCompile {
		// compile for all target architectures with tags
		for _, tags := range []string{"release", "debug"} {
			runWithEnv(env.env, "gox", "-verbose",
				"-os", strings.Join(env.goxOS, " "),
				"-arch", strings.Join(env.goxArch, " "),
				"-tags", tags,
				"-output", "/tmp/{{.Dir}}_{{.OS}}_{{.Arch}}",
				"cmds/restic")
		}
	}

	// run the build script
	run("go", "run", "build.go")

	// run the tests and gather coverage information
	runWithEnv(env.env, "gotestcover", "-coverprofile", "all.cov", "cmds/...", "restic/...")

	runGofmt()
}

type AppveyorEnvironment struct{}

func (env *AppveyorEnvironment) Prepare() {
	msg("preparing environment for Appveyor CI\n")
}

func (env *AppveyorEnvironment) RunTests() {
	run("go", "run", "build.go", "-v", "-T")
}

func (env *AppveyorEnvironment) Teardown() {}

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
	runWithEnv(nil, command, args...)
}

// runWithEnv calls a command with the current environment, except the entries
// of the env map are set additionally.
func runWithEnv(env map[string]string, command string, args ...string) {
	msg("runWithEnv %v %v\n", command, strings.Join(args, " "))
	cmd := exec.Command(command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if env != nil {
		cmd.Env = updateEnv(os.Environ(), env)
	}
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

	for _, f := range []func(){env.Prepare, env.RunTests, env.Teardown} {
		f()
	}
}
