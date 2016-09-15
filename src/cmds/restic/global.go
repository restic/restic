package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"restic"
	"runtime"
	"strings"
	"syscall"

	"restic/backend/local"
	"restic/backend/rest"
	"restic/backend/s3"
	"restic/backend/sftp"
	"restic/cache"
	"restic/debug"
	"restic/location"
	"restic/repository"

	"restic/errors"

	"github.com/jessevdk/go-flags"
	"golang.org/x/crypto/ssh/terminal"
)

var version = "compiled manually"
var compiledAt = "unknown time"

// GlobalOptions holds all those options that can be set for every command.
type GlobalOptions struct {
	Repo         string   `short:"r" long:"repo"                      description:"Repository directory to backup to/restore from"`
	PasswordFile string   `short:"p" long:"password-file"             description:"Read the repository password from a file"`
	CacheDir     string   `          long:"cache-dir"                 description:"Directory to use as a local cache"`
	Quiet        bool     `short:"q" long:"quiet"     default:"false" description:"Do not output comprehensive progress report"`
	NoLock       bool     `          long:"no-lock"   default:"false" description:"Do not lock the repo, this allows some operations on read-only repos."`
	Options      []string `short:"o" long:"option"                    description:"Specify options in the form 'foo.key=value'"`

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

func stdinIsTerminal() bool {
	return terminal.IsTerminal(int(os.Stdin.Fd()))
}

func stdoutIsTerminal() bool {
	return terminal.IsTerminal(int(os.Stdout.Fd()))
}

// restoreTerminal installs a cleanup handler that restores the previous
// terminal state on exit.
func restoreTerminal() {
	if !stdoutIsTerminal() {
		return
	}

	fd := int(os.Stdout.Fd())
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

// ClearLine creates a platform dependent string to clear the current
// line, so it can be overwritten. ANSI sequences are not supported on
// current windows cmd shell.
func ClearLine() string {
	if runtime.GOOS == "windows" {
		w, _, err := terminal.GetSize(int(os.Stdout.Fd()))
		if err == nil {
			return strings.Repeat(" ", w-1) + "\r"
		}
		return ""
	}
	return "\x1b[2K"
}

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
// the quiet flag is not set.
func (o GlobalOptions) ShowProgress() bool {
	if o.Quiet {
		return false
	}

	return true
}

// PrintProgress wraps fmt.Printf to handle the difference in writing progress
// information to terminals and non-terminal stdout
func PrintProgress(format string, args ...interface{}) {
	var (
		message         string
		carriageControl string
	)
	message = fmt.Sprintf(format, args...)

	if !(strings.HasSuffix(message, "\r") || strings.HasSuffix(message, "\n")) {
		if stdoutIsTerminal() {
			carriageControl = "\r"
		} else {
			carriageControl = "\n"
		}
		message = fmt.Sprintf("%s%s", message, carriageControl)
	}

	if stdoutIsTerminal() {
		message = fmt.Sprintf("%s%s", ClearLine(), message)
	}

	fmt.Print(message)
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

	if err != nil && errors.Cause(err) != io.ErrUnexpectedEOF {
		return "", errors.Wrap(err, "ReadFull")
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
		return "", errors.Wrap(err, "ReadPassword")
	}

	password = string(buf)
	return password, nil
}

// ReadPassword reads the password from a password file, the environment
// variable RESTIC_PASSWORD or prompts the user.
func (o GlobalOptions) ReadPassword(prompt string) (string, error) {
	if o.PasswordFile != "" {
		s, err := ioutil.ReadFile(o.PasswordFile)
		return strings.TrimSpace(string(s)), errors.Wrap(err, "Readfile")
	}

	if pwd := os.Getenv("RESTIC_PASSWORD"); pwd != "" {
		return pwd, nil
	}

	var (
		password string
		err      error
	)

	if stdinIsTerminal() {
		password, err = readPasswordTerminal(os.Stdin, os.Stderr, prompt)
	} else {
		password, err = readPassword(os.Stdin)
	}

	if err != nil {
		return "", errors.Wrap(err, "unable to read password")
	}

	if len(password) == 0 {
		return "", errors.Fatal("an empty password is not a password")
	}

	return password, nil
}

// ReadPasswordTwice calls ReadPassword two times and returns an error when the
// passwords don't match.
func (o GlobalOptions) ReadPasswordTwice(prompt1, prompt2 string) (string, error) {
	pw1, err := o.ReadPassword(prompt1)
	if err != nil {
		return "", err
	}
	pw2, err := o.ReadPassword(prompt2)
	if err != nil {
		return "", err
	}

	if pw1 != pw2 {
		return "", errors.Fatal("passwords do not match")
	}

	return pw1, nil
}

const maxKeys = 20

// OpenRepository reads the password and opens the repository.
func (o GlobalOptions) OpenRepository() (*repository.Repository, error) {
	if o.Repo == "" {
		return nil, errors.Fatal("Please specify repository location (-r)")
	}

	be, err := open(o.Repo)
	if err != nil {
		return nil, err
	}

	s := repository.New(be)

	if o.password == "" {
		o.password, err = o.ReadPassword("enter password for repository: ")
		if err != nil {
			return nil, err
		}
	}

	err = s.SearchKey(o.password, maxKeys)
	if err != nil {
		return nil, errors.Fatalf("unable to open repo: %v", err)
	}

	cache, err := cache.New(o.CacheDir, s.Config().ID)
	if err != nil {
		return nil, err
	}

	s.UseCache(cache)

	return s, nil
}

// Open the backend specified by a location config.
func open(s string) (restic.Backend, error) {
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
	case "rest":
		return rest.Open(loc.Config.(rest.Config))
	}

	debug.Log("open", "invalid repository location: %v", s)
	return nil, errors.Fatalf("invalid scheme %q", loc.Scheme)
}

// Create the backend specified by URI.
func create(s string) (restic.Backend, error) {
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
	case "rest":
		return rest.Open(loc.Config.(rest.Config))
	}

	debug.Log("open", "invalid repository scheme: %v", s)
	return nil, errors.Fatalf("invalid scheme %q", loc.Scheme)
}
