package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"runtime"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/jessevdk/go-flags"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/backend/local"
	"github.com/restic/restic/backend/sftp"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/repository"
)

var version = "compiled manually"

var mainOpts struct {
	Repo     string `short:"r" long:"repo"                      description:"Repository directory to backup to/restore from"`
	CacheDir string `          long:"cache-dir"                 description:"Directory to use as a local cache"`
	Quiet    bool   `short:"q" long:"quiet"     default:"false" description:"Do not output comprehensive progress report"`

	password string
}

var parser = flags.NewParser(&mainOpts, flags.Default)

func errx(code int, format string, data ...interface{}) {
	if len(format) > 0 && format[len(format)-1] != '\n' {
		format += "\n"
	}
	fmt.Fprintf(os.Stderr, format, data...)
	os.Exit(code)
}

func readPassword(prompt string) string {
	fmt.Fprint(os.Stderr, prompt)
	pw, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		errx(2, "unable to read password: %v", err)
	}
	fmt.Fprintln(os.Stderr)

	return string(pw)
}

func disableProgress() bool {
	if mainOpts.Quiet {
		return true
	}

	if !terminal.IsTerminal(int(os.Stdout.Fd())) {
		return true
	}

	return false
}

func silenceRequested() bool {
	if mainOpts.Quiet {
		return true
	}

	return false
}

func verbosePrintf(format string, args ...interface{}) {
	if silenceRequested() {
		return
	}

	fmt.Printf(format, args...)
}

type CmdInit struct{}

func (cmd CmdInit) Execute(args []string) error {
	if mainOpts.Repo == "" {
		return errors.New("Please specify repository location (-r)")
	}

	if mainOpts.password == "" {
		pw := readPassword("enter password for new backend: ")
		pw2 := readPassword("enter password again: ")

		if pw != pw2 {
			errx(1, "passwords do not match")
		}

		mainOpts.password = pw
	}

	be, err := create(mainOpts.Repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating backend at %s failed: %v\n", mainOpts.Repo, err)
		os.Exit(1)
	}

	s := repository.New(be)
	err = s.Init(mainOpts.password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating key in backend at %s failed: %v\n", mainOpts.Repo, err)
		os.Exit(1)
	}

	verbosePrintf("created restic backend %v at %s\n", s.Config.ID[:10], mainOpts.Repo)
	verbosePrintf("\n")
	verbosePrintf("Please note that knowledge of your password is required to access\n")
	verbosePrintf("the repository. Losing your password means that your data is\n")
	verbosePrintf("irrecoverably lost.\n")

	return nil
}

// Open the backend specified by URI.
// Valid formats are:
// * /foo/bar -> local repository at /foo/bar
// * sftp://user@host/foo/bar -> remote sftp repository on host for user at path foo/bar
// * sftp://host//tmp/backup -> remote sftp repository on host at path /tmp/backup
func open(u string) (backend.Backend, error) {
	url, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	if url.Scheme == "" {
		return local.Open(url.Path)
	}

	args := []string{url.Host}
	if url.User != nil && url.User.Username() != "" {
		args = append(args, "-l")
		args = append(args, url.User.Username())
	}
	args = append(args, "-s")
	args = append(args, "sftp")
	return sftp.Open(url.Path[1:], "ssh", args...)
}

// Create the backend specified by URI.
func create(u string) (backend.Backend, error) {
	url, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	if url.Scheme == "" {
		return local.Create(url.Path)
	}

	args := []string{url.Host}
	if url.User != nil && url.User.Username() != "" {
		args = append(args, "-l")
		args = append(args, url.User.Username())
	}
	args = append(args, "-s")
	args = append(args, "sftp")
	return sftp.Create(url.Path[1:], "ssh", args...)
}

func OpenRepo() (*repository.Repository, error) {
	if mainOpts.Repo == "" {
		return nil, errors.New("Please specify repository location (-r)")
	}

	be, err := open(mainOpts.Repo)
	if err != nil {
		return nil, err
	}

	s := repository.New(be)

	if mainOpts.password == "" {
		mainOpts.password = readPassword("enter password for repository: ")
	}

	err = s.SearchKey(mainOpts.password)
	if err != nil {
		return nil, fmt.Errorf("unable to open repo: %v", err)
	}

	return s, nil
}

func init() {
	// set GOMAXPROCS to number of CPUs
	runtime.GOMAXPROCS(runtime.NumCPU())

	_, err := parser.AddCommand("init",
		"create repository",
		"The init command creates a new repository",
		&CmdInit{})
	if err != nil {
		panic(err)
	}
}

func main() {
	// defer profile.Start(profile.MemProfileRate(100000), profile.ProfilePath(".")).Stop()
	// defer profile.Start(profile.CPUProfile, profile.ProfilePath(".")).Stop()
	mainOpts.Repo = os.Getenv("RESTIC_REPOSITORY")
	mainOpts.password = os.Getenv("RESTIC_PASSWORD")

	debug.Log("restic", "main %#v", os.Args)

	_, err := parser.Parse()
	if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
		os.Exit(0)
	}

	if err != nil {
		os.Exit(1)
	}
}
