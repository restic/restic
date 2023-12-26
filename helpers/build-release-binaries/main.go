package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"
)

var opts = struct {
	Verbose        bool
	SourceDir      string
	OutputDir      string
	Tags           string
	PlatformSubset string
	Platform       string
	SkipCompress   bool
	Version        string
}{}

func init() {
	pflag.BoolVarP(&opts.Verbose, "verbose", "v", false, "be verbose")
	pflag.StringVarP(&opts.SourceDir, "source", "s", "/restic", "path to the source code `directory`")
	pflag.StringVarP(&opts.OutputDir, "output", "o", "/output", "path to the output `directory`")
	pflag.StringVar(&opts.Tags, "tags", "", "additional build `tags`")
	pflag.StringVar(&opts.PlatformSubset, "platform-subset", "", "specify `n/t` to only build this subset")
	pflag.StringVarP(&opts.Platform, "platform", "p", "", "specify `os/arch` to only build this specific platform")
	pflag.BoolVar(&opts.SkipCompress, "skip-compress", false, "skip binary compression step")
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

func rm(file string) {
	err := os.Remove(file)

	if os.IsNotExist(err) {
		err = nil
	}

	if err != nil {
		die("error removing %v: %v", file, err)
	}
}

func mkdir(dir string) {
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		die("mkdir %v: %v", dir, err)
	}
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

	tags := "selfupdate"
	if opts.Tags != "" {
		tags += "," + opts.Tags
	}

	c := exec.Command("go", "build",
		"-o", outputFile,
		"-ldflags", "-s -w",
		"-tags", tags,
		"./cmd/restic",
	)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = sourceDir
	c.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS="+goos,
		"GOARCH="+goarch,
	)
	if goarch == "arm" {
		// the raspberry pi 1 only supports the ARMv6 instruction set
		c.Env = append(c.Env, "GOARM=6")
	}
	verbose("run %v %v in %v", "go", c.Args, c.Dir)

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
	default:
		outputFile = filename + ".bz2"
		c = exec.Command("bzip2", filename)
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
	if !opts.SkipCompress {
		filename = compress(goos, outputDir, filename)
	}
	return filename
}

func buildTargets(sourceDir, outputDir string, targets map[string][]string) {
	start := time.Now()
	// the go compiler is already parallelized, thus reduce the concurrency a bit
	workers := runtime.GOMAXPROCS(0) / 4
	if workers < 1 {
		workers = 1
	}
	msg("building with %d workers", workers)

	type Job struct{ GOOS, GOARCH string }

	var wg errgroup.Group
	ch := make(chan Job)

	for i := 0; i < workers; i++ {
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
	"aix":     {"ppc64"},
	"darwin":  {"amd64", "arm64"},
	"freebsd": {"386", "amd64", "arm"},
	"linux":   {"386", "amd64", "arm", "arm64", "ppc64le", "mips", "mipsle", "mips64", "mips64le", "riscv64", "s390x"},
	"netbsd":  {"386", "amd64"},
	"openbsd": {"386", "amd64"},
	"windows": {"386", "amd64"},
	"solaris": {"amd64"},
}

func downloadModules(sourceDir string) {
	c := exec.Command("go", "mod", "download")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = sourceDir

	err := c.Run()
	if err != nil {
		die("error downloading modules: %v", err)
	}
}

func selectSubset(subset string, target map[string][]string) (map[string][]string, error) {
	t, n, _ := strings.Cut(subset, "/")
	part, err := strconv.ParseInt(t, 10, 8)
	if err != nil {
		return nil, fmt.Errorf("failed to parse platform subset %q", subset)
	}
	total, err := strconv.ParseInt(n, 10, 8)
	if err != nil {
		return nil, fmt.Errorf("failed to parse platform subset %q", subset)
	}
	if total < 0 || part < 0 {
		return nil, errors.New("platform subset out of range")
	}
	if part >= total {
		return nil, errors.New("t must be in 0 <= t < n")
	}

	// flatten platform list
	platforms := []string{}
	for os, archs := range target {
		for _, arch := range archs {
			platforms = append(platforms, os+"/"+arch)
		}
	}
	sort.Strings(platforms)

	// select subset
	lower := len(platforms) * int(part) / int(total)
	upper := len(platforms) * int(part+1) / int(total)
	platforms = platforms[lower:upper]

	return buildPlatformList(platforms), nil
}

func buildPlatformList(platforms []string) map[string][]string {
	fmt.Printf("Building for %v\n", platforms)

	targets := make(map[string][]string)
	for _, platform := range platforms {
		os, arch, _ := strings.Cut(platform, "/")
		targets[os] = append(targets[os], arch)
	}
	return targets
}

func main() {
	if len(pflag.Args()) != 0 {
		die("USAGE: build-release-binaries [OPTIONS]")
	}

	targets := defaultBuildTargets
	if opts.PlatformSubset != "" {
		var err error
		targets, err = selectSubset(opts.PlatformSubset, targets)
		if err != nil {
			die("%s", err)
		}
	} else if opts.Platform != "" {
		targets = buildPlatformList([]string{opts.Platform})
	}

	sourceDir := abs(opts.SourceDir)
	outputDir := abs(opts.OutputDir)
	mkdir(outputDir)

	downloadModules(sourceDir)
	buildTargets(sourceDir, outputDir, targets)
}
