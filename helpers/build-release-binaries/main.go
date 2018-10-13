package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"
)

var opts = struct {
	Verbose   bool
	SourceDir string
	OutputDir string
	Version   string
}{}

func init() {
	pflag.BoolVarP(&opts.Verbose, "verbose", "v", false, "be verbose")
	pflag.StringVarP(&opts.SourceDir, "source", "s", "/restic", "path to the source code `directory`")
	pflag.StringVarP(&opts.OutputDir, "output", "o", "/output", "path to the output `directory`")
	pflag.StringVar(&opts.Version, "version", "", "use `x.y.z` as the version for output files")
	pflag.Parse()
}

func die(f string, args ...interface{}) {
	if !strings.HasSuffix(f, "\n") {
		f += "\n"
	}
	f = "\x1b[31m" + f + "\x1b[0m"
	fmt.Fprintf(os.Stderr, f, args...)
	os.Exit(1)
}

func msg(f string, args ...interface{}) {
	if !strings.HasSuffix(f, "\n") {
		f += "\n"
	}
	f = "\x1b[32m" + f + "\x1b[0m"
	fmt.Printf(f, args...)
}

func verbose(f string, args ...interface{}) {
	if !opts.Verbose {
		return
	}
	if !strings.HasSuffix(f, "\n") {
		f += "\n"
	}
	f = "\x1b[32m" + f + "\x1b[0m"
	fmt.Printf(f, args...)
}

func run(cmd string, args ...string) {
	c := exec.Command(cmd, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	err := c.Run()
	if err != nil {
		die("error running %s %s: %v", cmd, args, err)
	}
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
		die("error removing %v: %v", dir, err)
	}
}

func mkdir(dir string) {
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		die("mkdir %v: %v", dir, err)
	}
}

func getwd() string {
	pwd, err := os.Getwd()
	if err != nil {
		die("Getwd(): %v", err)
	}
	return pwd
}

func abs(dir string) string {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		die("unable to find absolute path for %v: %v", dir, err)
	}
	return absDir
}

func build(sourceDir, outputDir, goos, goarch string) (filename string) {
	filename = fmt.Sprintf("%v_%v_%v", "restic", goos, goarch)

	if opts.Version != "" {
		filename = fmt.Sprintf("%v_%v_%v_%v", "restic", opts.Version, goos, goarch)
	}

	if goos == "windows" {
		filename += ".exe"
	}
	outputFile := filepath.Join(outputDir, filename)

	c := exec.Command("go", "build",
		"-mod=vendor",
		"-o", outputFile,
		"-ldflags", "-s -w",
		"-tags", "selfupdate",
		"./cmd/restic",
	)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = sourceDir

	verbose("run %v %v in %v", "go", c.Args, c.Dir)

	c.Dir = sourceDir
	c.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOPROXY=off",
		"GOOS="+goos,
		"GOARCH="+goarch,
	)

	err := c.Run()
	if err != nil {
		die("error building %v/%v: %v", goos, goarch, err)
	}

	return filename
}

func modTime(file string) time.Time {
	fi, err := os.Lstat(file)
	if err != nil {
		die("unable to get modtime of %v: %v", file, err)
	}

	return fi.ModTime()
}

func touch(file string, t time.Time) {
	err := os.Chtimes(file, t, t)
	if err != nil {
		die("unable to update timestamps for %v: %v", file, err)
	}
}

func chmod(file string, mode os.FileMode) {
	err := os.Chmod(file, mode)
	if err != nil {
		die("unable to chmod %v to %s: %v", file, mode, err)
	}
}

func compress(goos, inputDir, filename string) (outputFile string) {
	var c *exec.Cmd
	switch goos {
	case "windows":
		outputFile = strings.TrimSuffix(filename, ".exe") + ".zip"
		c = exec.Command("zip", "-q", "-X", outputFile, filename)
		c.Dir = inputDir
	default:
		outputFile = filename + ".bz2"
		c = exec.Command("bzip2", filename)
		c.Dir = inputDir
	}

	rm(filepath.Join(inputDir, outputFile))

	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = inputDir

	verbose("run %v %v in %v", "go", c.Args, c.Dir)

	err := c.Run()
	if err != nil {
		die("error compressing: %v", err)
	}

	rm(filepath.Join(inputDir, filename))

	return outputFile
}

func buildForTarget(sourceDir, outputDir, goos, goarch string) (filename string) {
	mtime := modTime(filepath.Join(sourceDir, "VERSION"))

	filename = build(sourceDir, outputDir, goos, goarch)
	touch(filepath.Join(outputDir, filename), mtime)
	chmod(filepath.Join(outputDir, filename), 0755)
	filename = compress(goos, outputDir, filename)
	return filename
}

func buildTargets(sourceDir, outputDir string, targets map[string][]string) {
	start := time.Now()
	msg("building with %d workers", runtime.NumCPU())

	type Job struct{ GOOS, GOARCH string }

	var wg errgroup.Group
	ch := make(chan Job)

	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Go(func() error {
			for job := range ch {
				start := time.Now()
				verbose("build %v/%v", job.GOOS, job.GOARCH)
				buildForTarget(sourceDir, outputDir, job.GOOS, job.GOARCH)
				msg("built %v/%v in %.3fs", job.GOOS, job.GOARCH, time.Since(start).Seconds())
			}
			return nil
		})
	}

	wg.Go(func() error {
		for goos, archs := range targets {
			for _, goarch := range archs {
				ch <- Job{goos, goarch}
			}
		}
		close(ch)
		return nil
	})

	_ = wg.Wait()
	msg("build finished in %.3fs", time.Since(start).Seconds())
}

var defaultBuildTargets = map[string][]string{
	"darwin":  []string{"386", "amd64"},
	"freebsd": []string{"386", "amd64", "arm"},
	"linux":   []string{"386", "amd64", "arm", "arm64"},
	"openbsd": []string{"386", "amd64"},
	"windows": []string{"386", "amd64"},
}

func main() {
	if len(pflag.Args()) != 0 {
		die("USAGE: build-release-binaries [OPTIONS]")
	}

	sourceDir := abs(opts.SourceDir)
	outputDir := abs(opts.OutputDir)
	mkdir(outputDir)

	buildTargets(sourceDir, outputDir, defaultBuildTargets)
}
