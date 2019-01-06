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

	OutputDir string
}{}

var versionRegex = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func init() {
	pflag.BoolVar(&opts.IgnoreBranchName, "ignore-branch-name", false, "allow releasing from other branches as 'master'")
	pflag.BoolVar(&opts.IgnoreUncommittedChanges, "ignore-uncommitted-changes", false, "allow uncommitted changes")
	pflag.BoolVar(&opts.IgnoreChangelogVersion, "ignore-changelog-version", false, "ignore missing entry in CHANGELOG.md")
	pflag.BoolVar(&opts.IgnoreChangelogReleaseDate, "ignore-changelog-release-date", false, "ignore missing subdir with date in changelog/")
	pflag.BoolVar(&opts.IgnoreChangelogCurrent, "ignore-changelog-current", false, "ignore check if CHANGELOG.md is up to date")
	pflag.BoolVar(&opts.IgnoreDockerBuildGoVersion, "ignore-docker-build-go-version", false, "ignore check if docker builder go version is up to date")

	pflag.StringVar(&opts.OutputDir, "output-dir", "", "use `dir` as output directory")

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

func replace(filename, from, to string) {
	reg := regexp.MustCompile(from)

	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		die("error reading file %v: %v", filename, err)
	}

	buf = reg.ReplaceAll(buf, []byte(to))
	err = ioutil.WriteFile(filename, buf, 0644)
	if err != nil {
		die("error writing file %v: %v", filename, err)
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

	msg("update docker container restic/builder")
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

var versionPattern = `var version = ".*"`

const versionCodeFile = "cmd/restic/global.go"

func updateVersion() {
	err := ioutil.WriteFile("VERSION", []byte(opts.Version+"\n"), 0644)
	if err != nil {
		die("unable to write version to file: %v", err)
	}

	newVersion := fmt.Sprintf("var version = %q", opts.Version)
	replace(versionCodeFile, versionPattern, newVersion)

	if len(uncommittedChanges("VERSION")) > 0 || len(uncommittedChanges(versionCodeFile)) > 0 {
		msg("committing version files")
		run("git", "commit", "-m", fmt.Sprintf("Add version for %v", opts.Version), "VERSION", versionCodeFile)
	}
}

func updateVersionDev() {
	newVersion := fmt.Sprintf(`var version = "%s-dev (compiled manually)"`, opts.Version)
	replace(versionCodeFile, versionPattern, newVersion)

	msg("committing cmd/restic/global.go with dev version")
	run("git", "commit", "-m", fmt.Sprintf("Set development version for %v", opts.Version), "VERSION", versionCodeFile)
}

func addTag() {
	tagname := "v" + opts.Version
	msg("add tag %v", tagname)
	run("git", "tag", "-a", "-s", "-m", tagname, tagname)
}

func exportTar(version, tarFilename string) {
	cmd := fmt.Sprintf("git archive --format=tar --prefix=restic-%s/ v%s | gzip -n > %s",
		version, version, tarFilename)
	run("sh", "-c", cmd)
	msg("build restic-%s.tar.gz", version)
}

func extractTar(filename, outputDir string) {
	msg("extract tar into %v", outputDir)
	c := exec.Command("tar", "xz", "--strip-components=1", "-f", filename)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = outputDir
	err := c.Run()
	if err != nil {
		die("error extracting tar: %v", err)
	}
}

func runBuild(sourceDir, outputDir, version string) {
	msg("building binaries...")
	run("docker", "run", "--rm",
		"--volume", sourceDir+":/restic",
		"--volume", outputDir+":/output",
		"restic/builder",
		"go", "run", "-mod=vendor", "helpers/build-release-binaries/main.go",
		"--version", version)
}

func readdir(dir string) []string {
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		die("readdir %v failed: %v", dir, err)
	}

	filenames := make([]string, 0, len(fis))
	for _, fi := range fis {
		filenames = append(filenames, fi.Name())
	}
	return filenames
}

func sha256sums(inputDir, outputFile string) {
	msg("runnnig sha256sum in %v", inputDir)

	filenames := readdir(inputDir)

	f, err := os.Create(outputFile)
	if err != nil {
		die("unable to create %v: %v", outputFile, err)
	}

	c := exec.Command("sha256sum", filenames...)
	c.Stdout = f
	c.Stderr = os.Stderr
	c.Dir = inputDir

	err = c.Run()
	if err != nil {
		die("error running sha256sums: %v", err)
	}

	err = f.Close()
	if err != nil {
		die("close %v: %v", outputFile, err)
	}
}

func signFiles(filenames ...string) {
	for _, filename := range filenames {
		run("gpg", "--armor", "--detach-sign", filename)
	}
}

func updateDocker(outputDir, version string) {
	cmd := fmt.Sprintf("bzcat %s/restic_%s_linux_amd64.bz2 > restic", outputDir, version)
	run("sh", "-c", cmd)
	run("chmod", "+x", "restic")
	run("docker", "pull", "alpine:latest")
	run("docker", "build", "--rm", "--tag", "restic/restic:latest", "-f", "docker/Dockerfile", ".")
	run("docker", "tag", "restic/restic:latest", "restic/restic:"+version)
}

func tempdir(prefix string) string {
	dir, err := ioutil.TempDir(getwd(), prefix)
	if err != nil {
		die("unable to create temp dir %q: %v", prefix, err)
	}
	return dir
}

func main() {
	if len(pflag.Args()) == 0 {
		die("USAGE: release-version [OPTIONS] VERSION")
	}

	opts.Version = pflag.Args()[0]
	if !versionRegex.MatchString(opts.Version) {
		die("invalid new version")
	}

	preCheckBranchMaster()
	preCheckUncommittedChanges()
	preCheckVersionExists()
	preCheckDockerBuilderGoVersion()
	preCheckChangelogRelease()
	preCheckChangelogCurrent()
	preCheckChangelogVersion()

	if opts.OutputDir == "" {
		opts.OutputDir = tempdir("build-output-")
	}
	sourceDir := tempdir("source-")

	msg("using output dir %v", opts.OutputDir)
	msg("using source dir %v", sourceDir)

	generateFiles()
	updateVersion()
	addTag()
	updateVersionDev()

	tarFilename := filepath.Join(opts.OutputDir, fmt.Sprintf("restic-%s.tar.gz", opts.Version))
	exportTar(opts.Version, tarFilename)

	extractTar(tarFilename, sourceDir)
	runBuild(sourceDir, opts.OutputDir, opts.Version)
	rmdir(sourceDir)

	sha256sums(opts.OutputDir, filepath.Join(opts.OutputDir, "SHA256SUMS"))

	signFiles(filepath.Join(opts.OutputDir, "SHA256SUMS"), tarFilename)

	updateDocker(opts.OutputDir, opts.Version)

	msg("done, output dir is %v", opts.OutputDir)

	msg("now run:\n\ngit push --tags origin master\ndocker push restic/restic\n")
}
