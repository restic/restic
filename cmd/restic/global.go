package main

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/jessevdk/go-flags"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/backend/local"
	"github.com/restic/restic/backend/rest"
	"github.com/restic/restic/backend/s3"
	"github.com/restic/restic/backend/sftp"
	"github.com/restic/restic/repository"
	"golang.org/x/crypto/ssh/terminal"
)

var version = "compiled manually"
var compiledAt = "unknown time"

type GlobalOptions struct {
	Repo     string `short:"r" long:"repo"                      description:"Repository directory to backup to/restore from"`
	CacheDir string `          long:"cache-dir"                 description:"Directory to use as a local cache"`
	Quiet    bool   `short:"q" long:"quiet"     default:"false" description:"Do not output comprehensive progress report"`

	password string
	stdout   io.Writer
	stderr   io.Writer
}

var globalOpts = GlobalOptions{stdout: os.Stdout, stderr: os.Stderr}
var parser = flags.NewParser(&globalOpts, flags.Default)

func (o GlobalOptions) Printf(format string, args ...interface{}) {
	_, err := fmt.Fprintf(o.stdout, format, args...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to write to stdout: %v\n", err)
		os.Exit(100)
	}
}

func (o GlobalOptions) Verbosef(format string, args ...interface{}) {
	if o.Quiet {
		return
	}

	o.Printf(format, args...)
}

func (o GlobalOptions) ShowProgress() bool {
	if o.Quiet {
		return false
	}

	if !terminal.IsTerminal(int(os.Stdout.Fd())) {
		return false
	}

	return true
}

func (o GlobalOptions) Warnf(format string, args ...interface{}) {
	_, err := fmt.Fprintf(o.stderr, format, args...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to write to stderr: %v\n", err)
		os.Exit(100)
	}
}

func (o GlobalOptions) Exitf(exitcode int, format string, args ...interface{}) {
	if format[len(format)-1] != '\n' {
		format += "\n"
	}

	o.Warnf(format, args...)
	os.Exit(exitcode)
}

func (o GlobalOptions) ReadPassword(prompt string) string {
	fmt.Fprint(os.Stderr, prompt)
	pw, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		o.Exitf(2, "unable to read password: %v", err)
	}
	fmt.Fprintln(os.Stderr)

	if len(pw) == 0 {
		o.Exitf(1, "an empty password is not a password")
	}

	return string(pw)
}

func (o GlobalOptions) ReadPasswordTwice(prompt1, prompt2 string) string {
	pw1 := o.ReadPassword(prompt1)
	pw2 := o.ReadPassword(prompt2)
	if pw1 != pw2 {
		o.Exitf(1, "passwords do not match")
	}

	return pw1
}

func (o GlobalOptions) OpenRepository() (*repository.Repository, error) {
	if o.Repo == "" {
		return nil, errors.New("Please specify repository location (-r)")
	}

	be, err := open(o.Repo)
	if err != nil {
		return nil, err
	}

	s := repository.New(be)

	if o.password == "" {
		o.password = o.ReadPassword("enter password for repository: ")
	}

	err = s.SearchKey(o.password)
	if err != nil {
		return nil, fmt.Errorf("unable to open repo: %v", err)
	}

	return s, nil
}

// Open the backend specified by URI.
// Valid formats are:
// * /foo/bar -> local repository at /foo/bar
// * s3://region/bucket -> amazon s3 bucket
// * sftp://user@host/foo/bar -> remote sftp repository on host for user at path foo/bar
// * sftp://host//tmp/backup -> remote sftp repository on host at path /tmp/backup
// * http://host -> remote and public http repository
func open(u string) (backend.Backend, error) {
	fmt.Println(u)

	url, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	if url.Scheme == "" {
		return local.Open(url.Path)
	} else if url.Scheme == "s3" {
		return s3.Open(url.Host, url.Path[1:])
	} else if url.Scheme == "http" {
		return rest.Open(url)
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
	} else if url.Scheme == "s3" {
		return s3.Open(url.Host, url.Path[1:])
	} else if url.Scheme == "http" {
		return rest.Open(url)
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
