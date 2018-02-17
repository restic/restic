// +build ignore

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/pflag"
)

var opts = struct {
	Version string

	IgnoreBranchName           bool
	IgnoreUncommittedChanges   bool
	IgnoreChangelogVersion     bool
	IgnoreChangelogReleaseDate bool
	IgnoreChangelogCurrent     bool
	IgnoreDockerBuildGoVersion bool

	tarFilename string
	buildDir    string
}{}

var versionRegex = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func init() {
	pflag.BoolVar(&opts.IgnoreBranchName, "ignore-branch-name", false, "allow releasing from other branches as 'master'")
	pflag.BoolVar(&opts.IgnoreUncommittedChanges, "ignore-uncommitted-changes", false, "allow uncommitted changes")
	pflag.BoolVar(&opts.IgnoreChangelogVersion, "ignore-changelog-version", false, "ignore missing entry in CHANGELOG.md")
	pflag.BoolVar(&opts.IgnoreChangelogReleaseDate, "ignore-changelog-release-date", false, "ignore missing subdir with date in changelog/")
	pflag.BoolVar(&opts.IgnoreChangelogCurrent, "ignore-changelog-current", false, "ignore check if CHANGELOG.md is up to date")
	pflag.BoolVar(&opts.IgnoreDockerBuildGoVersion, "ignore-docker-build-go-version", false, "ignore check if docker builder go version is up to date")
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
	err := os.Mkdir(dir, 0755)
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

func uncommittedChanges(dirs ...string) string {
	args := []string{"status", "--porcelain", "--untracked-files=no"}
	if len(dirs) > 0 {
		args = append(args, dirs...)
	}

	changes, err := exec.Command("git", args...).Output()
	if err != nil {
		die("unable to run command: %v", err)
	}

	return string(changes)
}

func preCheckBranchMaster() {
	if opts.IgnoreBranchName {
		return
	}

	branch, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		die("error running 'git': %v", err)
	}

	if strings.TrimSpace(string(branch)) != "master" {
		die("wrong branch: %s", branch)
	}
}

func preCheckUncommittedChanges() {
	if opts.IgnoreUncommittedChanges {
		return
	}

	changes := uncommittedChanges()
	if len(changes) > 0 {
		die("uncommitted changes found:\n%s\n", changes)
	}
}

func preCheckVersionExists() {
	buf, err := exec.Command("git", "tag", "-l").Output()
	if err != nil {
		die("error running 'git tag -l': %v", err)
	}

	sc := bufio.NewScanner(bytes.NewReader(buf))
	for sc.Scan() {
		if sc.Err() != nil {
			die("error scanning version tags: %v", sc.Err())
		}

		if strings.TrimSpace(sc.Text()) == "v"+opts.Version {
			die("tag v%v already exists", opts.Version)
		}
	}
}

func preCheckChangelogCurrent() {
	if opts.IgnoreChangelogCurrent {
		return
	}

	// regenerate changelog
	run("calens", "--output", "CHANGELOG.md")

	// check for uncommitted changes in changelog
	if len(uncommittedChanges("CHANGELOG.md")) > 0 {
		msg("committing file CHANGELOG.md")
		run("git", "commit", "-m", fmt.Sprintf("Generate CHANGELOG.md for %v", opts.Version), "CHANGELOG.md")
	}
}

func preCheckChangelogRelease() {
	if opts.IgnoreChangelogReleaseDate {
		return
	}

	d, err := os.Open("changelog")
	if err != nil {
		die("error opening dir: %v", err)
	}

	names, err := d.Readdirnames(-1)
	if err != nil {
		_ = d.Close()
		die("error listing dir: %v", err)
	}

	err = d.Close()
	if err != nil {
		die("error closing dir: %v", err)
	}

	for _, name := range names {
		if strings.HasPrefix(name, opts.Version+"_") {
			return
		}
	}

	die("unable to find subdir with date for version %v in changelog", opts.Version)
}

func preCheckChangelogVersion() {
	if opts.IgnoreChangelogVersion {
		return
	}

	f, err := os.Open("CHANGELOG.md")
	if err != nil {
		die("unable to open CHANGELOG.md: %v", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if sc.Err() != nil {
			die("error scanning: %v", sc.Err())
		}

		if strings.Contains(strings.TrimSpace(sc.Text()), fmt.Sprintf("Changelog for restic %v", opts.Version)) {
			return
		}
	}

	die("CHANGELOG.md does not contain version %v", opts.Version)
}

func preCheckDockerBuilderGoVersion() {
	if opts.IgnoreDockerBuildGoVersion {
		return
	}

	buf, err := exec.Command("go", "version").Output()
	if err != nil {
		die("unable to check local Go version: %v", err)
	}
	localVersion := strings.TrimSpace(string(buf))

	run("docker", "pull", "restic/builder")
	buf, err = exec.Command("docker", "run", "--rm", "restic/builder", "go", "version").Output()
	if err != nil {
		die("unable to check Go version in docker image: %v", err)
	}
	containerVersion := strings.TrimSpace(string(buf))

	if localVersion != containerVersion {
		die("version in docker container restic/builder is different:\n  local:     %v\n  container: %v\n",
			localVersion, containerVersion)
	}
}

func generateFiles() {
	msg("generate files")
	run("go", "run", "build.go", "-o", "restic-generate.temp")

	mandir := filepath.Join("doc", "man")
	rmdir(mandir)
	mkdir(mandir)
	run("./restic-generate.temp", "generate",
		"--man", "doc/man",
		"--zsh-completion", "doc/zsh-completion.zsh",
		"--bash-completion", "doc/bash-completion.sh")
	rm("restic-generate.temp")

	run("git", "add", "doc")
	changes := uncommittedChanges("doc")
	if len(changes) > 0 {
		msg("committing manpages and auto-completion")
		run("git", "commit", "-m", "Update manpages and auto-completion", "doc")
	}
}

func updateVersion() {
	err := ioutil.WriteFile("VERSION", []byte(opts.Version+"\n"), 0644)
	if err != nil {
		die("unable to write version to file: %v", err)
	}

	if len(uncommittedChanges("VERSION")) > 0 {
		msg("committing file VERSION")
		run("git", "commit", "-m", fmt.Sprintf("Add VERSION for %v", opts.Version), "VERSION")
	}
}

func addTag() {
	tagname := "v" + opts.Version
	msg("add tag %v", tagname)
	run("git", "tag", "-a", "-s", "-m", tagname, tagname)
}

func exportTar() {
	cmd := fmt.Sprintf("git archive --format=tar --prefix=restic-%s/ v%s | gzip -n > %s",
		opts.Version, opts.Version, opts.tarFilename)
	run("sh", "-c", cmd)
	msg("build restic-%s.tar.gz", opts.Version)
}

func runBuild() {
	msg("building binaries...")
	run("docker", "run", "--rm", "--volume", getwd()+":/home/build", "restic/builder", "build.sh", opts.tarFilename)
}

func findBuildDir() string {
	nameRegex := regexp.MustCompile(`restic-` + opts.Version + `-\d{8}-\d{6}`)

	f, err := os.Open(".")
	if err != nil {
		die("Open(.): %v", err)
	}

	entries, err := f.Readdirnames(-1)
	if err != nil {
		die("Readdirnames(): %v", err)
	}

	err = f.Close()
	if err != nil {
		die("Close(): %v", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[j] < entries[i]
	})

	for _, entry := range entries {
		if nameRegex.MatchString(entry) {
			msg("found restic build dir: %v", entry)
			return entry
		}
	}

	die("restic build dir not found")
	return ""
}

func signFiles() {
	run("gpg", "--armor", "--detach-sign", filepath.Join(opts.buildDir, "SHA256SUMS"))
	run("gpg", "--armor", "--detach-sign", filepath.Join(opts.buildDir, opts.tarFilename))
}

func updateDocker() {
	cmd := fmt.Sprintf("bzcat %s/restic_%s_linux_amd64.bz2 > restic", opts.buildDir, opts.Version)
	run("sh", "-c", cmd)
	run("chmod", "+x", "restic")
	run("docker", "build", "--rm", "--tag", "restic/restic:latest", "-f", "docker/Dockerfile", ".")
	run("docker", "tag", "restic/restic:latest", "restic/restic:"+opts.Version)
}

func main() {
	if len(pflag.Args()) == 0 {
		die("USAGE: release-version [OPTIONS] VERSION")
	}

	opts.Version = pflag.Args()[0]
	if !versionRegex.MatchString(opts.Version) {
		die("invalid new version")
	}

	opts.tarFilename = fmt.Sprintf("restic-%s.tar.gz", opts.Version)

	preCheckBranchMaster()
	preCheckUncommittedChanges()
	preCheckVersionExists()
	preCheckDockerBuilderGoVersion()
	preCheckChangelogRelease()
	preCheckChangelogCurrent()
	preCheckChangelogVersion()

	generateFiles()
	updateVersion()
	addTag()

	exportTar()
	runBuild()
	opts.buildDir = findBuildDir()
	signFiles()

	updateDocker()

	msg("done, build dir is %v", opts.buildDir)

	msg("now run:\n\ngit push --tags origin master\ndocker push restic/restic\n")
}
