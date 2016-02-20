package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/jessevdk/go-flags"
	"golang.org/x/crypto/ssh/terminal"
	"restic/backend"
	"restic/backend/local"
	"restic/backend/s3"
	"restic/backend/sftp"
	"restic/debug"
	"restic/location"
	"restic/repository"
)

var version = "compiled manually"
var compiledAt = "unknown time"

// GlobalOptions holds all those options that can be set for every command.
type GlobalOptions struct {
	Repo     string   `short:"r" long:"repo"                      description:"Repository directory to backup to/restore from"`
	CacheDir string   `          long:"cache-dir"                 description:"Directory to use as a local cache"`
	Quiet    bool     `short:"q" long:"quiet"     default:"false" description:"Do not output comprehensive progress report"`
	NoLock   bool     `          long:"no-lock"   default:"false" description:"Do not lock the repo, this allows some operations on read-only repos."`
	Options  []string `short:"o" long:"option"                    description:"Specify options in the form 'foo.key=value'"`

	password string
	stdout   io.Writer
	stderr   io.Writer
}

func init() {
	restoreTerminal()
}

// checkErrno returns nil when err is set to syscall.Errno(0), since this is no
// error condition.
func checkErrno(err error) error {
	e, ok := err.(syscall.Errno)
	if !ok {
		return err
	}

	if e == 0 {
		return nil
	}

	return err
}

// restoreTerminal installs a cleanup handler that restores the previous
// terminal state on exit.
func restoreTerminal() {
	fd := int(os.Stdout.Fd())
	if !terminal.IsTerminal(fd) {
		return
	}

	state, err := terminal.GetState(fd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to get terminal state: %v\n", err)
		return
	}

	AddCleanupHandler(func() error {
		err := checkErrno(terminal.Restore(fd, state))
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to get restore terminal state: %#+v\n", err)
		}
		return err
	})
}

var globalOpts = GlobalOptions{stdout: os.Stdout, stderr: os.Stderr}
var parser = flags.NewParser(&globalOpts, flags.HelpFlag|flags.PassDoubleDash)

// Printf writes the message to the configured stdout stream.
func (o GlobalOptions) Printf(format string, args ...interface{}) {
	_, err := fmt.Fprintf(o.stdout, format, args...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to write to stdout: %v\n", err)
		os.Exit(100)
	}
}

// Verbosef calls Printf to write the message when the verbose flag is set.
func (o GlobalOptions) Verbosef(format string, args ...interface{}) {
	if o.Quiet {
		return
	}

	o.Printf(format, args...)
}

// ShowProgress returns true iff the progress status should be written, i.e.
// the quiet flag is not set and the output is a terminal.
func (o GlobalOptions) ShowProgress() bool {
	if o.Quiet {
		return false
	}

	if !terminal.IsTerminal(int(os.Stdout.Fd())) {
		return false
	}

	return true
}

// Warnf writes the message to the configured stderr stream.
func (o GlobalOptions) Warnf(format string, args ...interface{}) {
	_, err := fmt.Fprintf(o.stderr, format, args...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to write to stderr: %v\n", err)
		os.Exit(100)
	}
}

// Exitf uses Warnf to write the message and then calls os.Exit(exitcode).
func (o GlobalOptions) Exitf(exitcode int, format string, args ...interface{}) {
	if format[len(format)-1] != '\n' {
		format += "\n"
	}

	o.Warnf(format, args...)
	os.Exit(exitcode)
}

// readPassword reads the password from the given reader directly.
func readPassword(in io.Reader) (password string, err error) {
	buf := make([]byte, 1000)
	n, err := io.ReadFull(in, buf)
	buf = buf[:n]

	if err != nil && err != io.ErrUnexpectedEOF {
		return "", err
	}

	return strings.TrimRight(string(buf), "\r\n"), nil
}

// readPasswordTerminal reads the password from the given reader which must be a
// tty. Prompt is printed on the writer out before attempting to read the
// password.
func readPasswordTerminal(in *os.File, out io.Writer, prompt string) (password string, err error) {
	fmt.Fprint(out, prompt)
	buf, err := terminal.ReadPassword(int(in.Fd()))
	fmt.Fprintln(out)
	if err != nil {
		return "", err
	}

	password = string(buf)
	return password, nil
}

// ReadPassword reads the password from stdin.
func (o GlobalOptions) ReadPassword(prompt string) string {
	var (
		password string
		err      error
	)

	if terminal.IsTerminal(int(os.Stdin.Fd())) {
		password, err = readPasswordTerminal(os.Stdin, os.Stderr, prompt)
	} else {
		password, err = readPassword(os.Stdin)
	}

	if err != nil {
		o.Exitf(2, "unable to read password: %v", err)
	}

	if len(password) == 0 {
		o.Exitf(1, "an empty password is not a password")
	}

	return password
}

// ReadPasswordTwice calls ReadPassword two times and returns an error when the
// passwords don't match.
func (o GlobalOptions) ReadPasswordTwice(prompt1, prompt2 string) string {
	pw1 := o.ReadPassword(prompt1)
	pw2 := o.ReadPassword(prompt2)
	if pw1 != pw2 {
		o.Exitf(1, "passwords do not match")
	}

	return pw1
}

// OpenRepository reads the password and opens the repository.
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

// Open the backend specified by a location config.
func open(s string) (backend.Backend, error) {
	debug.Log("open", "parsing location %v", s)
	loc, err := location.Parse(s)
	if err != nil {
		return nil, err
	}

	switch loc.Scheme {
	case "local":
		debug.Log("open", "opening local repository at %#v", loc.Config)
		return local.Open(loc.Config.(string))
	case "sftp":
		debug.Log("open", "opening sftp repository at %#v", loc.Config)
		return sftp.OpenWithConfig(loc.Config.(sftp.Config))
	case "s3":
		cfg := loc.Config.(s3.Config)
		if cfg.KeyID == "" {
			cfg.KeyID = os.Getenv("AWS_ACCESS_KEY_ID")

		}
		if cfg.Secret == "" {
			cfg.Secret = os.Getenv("AWS_SECRET_ACCESS_KEY")
		}

		debug.Log("open", "opening s3 repository at %#v", cfg)
		return s3.Open(cfg)
	}

	debug.Log("open", "invalid repository location: %v", s)
	return nil, fmt.Errorf("invalid scheme %q", loc.Scheme)
}

// Create the backend specified by URI.
func create(s string) (backend.Backend, error) {
	debug.Log("open", "parsing location %v", s)
	loc, err := location.Parse(s)
	if err != nil {
		return nil, err
	}

	switch loc.Scheme {
	case "local":
		debug.Log("open", "create local repository at %#v", loc.Config)
		return local.Create(loc.Config.(string))
	case "sftp":
		debug.Log("open", "create sftp repository at %#v", loc.Config)
		return sftp.CreateWithConfig(loc.Config.(sftp.Config))
	case "s3":
		cfg := loc.Config.(s3.Config)
		if cfg.KeyID == "" {
			cfg.KeyID = os.Getenv("AWS_ACCESS_KEY_ID")

		}
		if cfg.Secret == "" {
			cfg.Secret = os.Getenv("AWS_SECRET_ACCESS_KEY")
		}

		debug.Log("open", "create s3 repository at %#v", loc.Config)
		return s3.Open(cfg)
	}

	debug.Log("open", "invalid repository scheme: %v", s)
	return nil, fmt.Errorf("invalid scheme %q", loc.Scheme)
}
