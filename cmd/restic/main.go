package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"runtime"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/jessevdk/go-flags"
	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
)

var version = "compiled manually"

var opts struct {
	Repo string `short:"r" long:"repo"    description:"Repository directory to backup to/restore from"`
}

var parser = flags.NewParser(&opts, flags.Default)

func errx(code int, format string, data ...interface{}) {
	if len(format) > 0 && format[len(format)-1] != '\n' {
		format += "\n"
	}
	fmt.Fprintf(os.Stderr, format, data...)
	os.Exit(code)
}

func readPassword(env string, prompt string) string {

	if env != "" {
		p := os.Getenv(env)

		if p != "" {
			return p
		}
	}

	fmt.Fprint(os.Stderr, prompt)
	pw, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		errx(2, "unable to read password: %v", err)
	}
	fmt.Fprintln(os.Stderr)

	return string(pw)
}

type CmdInit struct{}

func (cmd CmdInit) Execute(args []string) error {
	if opts.Repo == "" {
		return errors.New("Please specify repository location (-r)")
	}

	pw := readPassword("RESTIC_PASSWORD", "enter password for new backend: ")
	pw2 := readPassword("RESTIC_PASSWORD", "enter password again: ")

	if pw != pw2 {
		errx(1, "passwords do not match")
	}

	be, err := create(opts.Repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating backend at %s failed: %v\n", opts.Repo, err)
		os.Exit(1)
	}

	s := restic.NewServer(be)

	_, err = restic.CreateKey(s, pw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating key in backend at %s failed: %v\n", opts.Repo, err)
		os.Exit(1)
	}

	fmt.Printf("created restic backend at %s\n", opts.Repo)

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
		return backend.OpenLocal(url.Path)
	}

	args := []string{url.Host}
	if url.User != nil && url.User.Username() != "" {
		args = append(args, "-l")
		args = append(args, url.User.Username())
	}
	args = append(args, "-s")
	args = append(args, "sftp")
	return backend.OpenSFTP(url.Path[1:], "ssh", args...)
}

// Create the backend specified by URI.
func create(u string) (backend.Backend, error) {
	url, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	if url.Scheme == "" {
		return backend.CreateLocal(url.Path)
	}

	args := []string{url.Host}
	if url.User != nil && url.User.Username() != "" {
		args = append(args, "-l")
		args = append(args, url.User.Username())
	}
	args = append(args, "-s")
	args = append(args, "sftp")
	return backend.CreateSFTP(url.Path[1:], "ssh", args...)
}

func OpenRepo() (restic.Server, error) {
	if opts.Repo == "" {
		return restic.Server{}, errors.New("Please specify repository location (-r)")
	}

	be, err := open(opts.Repo)
	if err != nil {
		return restic.Server{}, err
	}

	s := restic.NewServer(be)

	err = s.SearchKey(readPassword("RESTIC_PASSWORD", "Enter Password for Repository: "))
	if err != nil {
		return restic.Server{}, fmt.Errorf("unable to open repo: %v", err)
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
	opts.Repo = os.Getenv("RESTIC_REPOSITORY")

	debug.Log("restic", "main %#v", os.Args)

	_, err := parser.Parse()
	if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
		os.Exit(0)
	}

	if err != nil {
		os.Exit(1)
	}

	// restic.PoolAlloc()
}
