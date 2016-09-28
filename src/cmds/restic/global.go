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

	"github.com/spf13/cobra"

	"restic/backend/local"
	"restic/backend/rest"
	"restic/backend/s3"
	"restic/backend/sftp"
	"restic/debug"
	"restic/location"
	"restic/repository"

	"restic/errors"

	"golang.org/x/crypto/ssh/terminal"
)

var version = "compiled manually"
var compiledAt = "unknown time"

func parseEnvironment(cmd *cobra.Command, args []string) {
	repo := os.Getenv("RESTIC_REPOSITORY")
	if repo != "" {
		globalOptions.Repo = repo
	}

	pw := os.Getenv("RESTIC_PASSWORD")
	if pw != "" {
		globalOptions.password = pw
	}
}

// GlobalOptions hold all global options for restic.
type GlobalOptions struct {
	Repo         string
	PasswordFile string
	Quiet        bool
	NoLock       bool

	password string
	stdout   io.Writer
	stderr   io.Writer
}

var globalOptions = GlobalOptions{
	stdout: os.Stdout,
	stderr: os.Stderr,
}

func init() {
	f := cmdRoot.PersistentFlags()
	f.StringVarP(&globalOptions.Repo, "repo", "r", "", "repository to backup to or restore from (default: $RESTIC_REPOSITORY)")
	f.StringVarP(&globalOptions.PasswordFile, "password-file", "p", "", "read the repository password from a file")
	f.BoolVarP(&globalOptions.Quiet, "quiet", "q", false, "do not outputcomprehensive progress report")
	f.BoolVar(&globalOptions.NoLock, "no-lock", false, "do not lock the repo, this allows some operations on read-only repos")

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
func Printf(format string, args ...interface{}) {
	_, err := fmt.Fprintf(globalOptions.stdout, format, args...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to write to stdout: %v\n", err)
		os.Exit(100)
	}
}

// Verbosef calls Printf to write the message when the verbose flag is set.
func Verbosef(format string, args ...interface{}) {
	if globalOptions.Quiet {
		return
	}

	Printf(format, args...)
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
func Warnf(format string, args ...interface{}) {
	_, err := fmt.Fprintf(globalOptions.stderr, format, args...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to write to stderr: %v\n", err)
		os.Exit(100)
	}
}

// Exitf uses Warnf to write the message and then calls os.Exit(exitcode).
func Exitf(exitcode int, format string, args ...interface{}) {
	if format[len(format)-1] != '\n' {
		format += "\n"
	}

	Warnf(format, args...)
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
func ReadPassword(opts GlobalOptions, prompt string) (string, error) {
	if opts.PasswordFile != "" {
		s, err := ioutil.ReadFile(opts.PasswordFile)
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
func ReadPasswordTwice(gopts GlobalOptions, prompt1, prompt2 string) (string, error) {
	pw1, err := ReadPassword(gopts, prompt1)
	if err != nil {
		return "", err
	}
	pw2, err := ReadPassword(gopts, prompt2)
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
func OpenRepository(opts GlobalOptions) (*repository.Repository, error) {
	if opts.Repo == "" {
		return nil, errors.Fatal("Please specify repository location (-r)")
	}

	be, err := open(opts.Repo)
	if err != nil {
		return nil, err
	}

	s := repository.New(be)

	if opts.password == "" {
		opts.password, err = ReadPassword(opts, "enter password for repository: ")
		if err != nil {
			return nil, err
		}
	}

	err = s.SearchKey(opts.password, maxKeys)
	if err != nil {
		return nil, errors.Fatalf("unable to open repo: %v", err)
	}

	return s, nil
}

// Open the backend specified by a location config.
func open(s string) (restic.Backend, error) {
	debug.Log("parsing location %v", s)
	loc, err := location.Parse(s)
	if err != nil {
		return nil, errors.Fatalf("parsing repository location failed: %v", err)
	}

	var be restic.Backend

	switch loc.Scheme {
	case "local":
		debug.Log("opening local repository at %#v", loc.Config)
		be, err = local.Open(loc.Config.(string))
	case "sftp":
		debug.Log("opening sftp repository at %#v", loc.Config)
		be, err = sftp.OpenWithConfig(loc.Config.(sftp.Config))
	case "s3":
		cfg := loc.Config.(s3.Config)
		if cfg.KeyID == "" {
			cfg.KeyID = os.Getenv("AWS_ACCESS_KEY_ID")

		}
		if cfg.Secret == "" {
			cfg.Secret = os.Getenv("AWS_SECRET_ACCESS_KEY")
		}

		debug.Log("opening s3 repository at %#v", cfg)
		be, err = s3.Open(cfg)
	case "rest":
		be, err = rest.Open(loc.Config.(rest.Config))
	default:
		return nil, errors.Fatalf("invalid backend: %q", loc.Scheme)
	}

	if err != nil {
		return nil, errors.Fatalf("unable to open repo at %v: %v", s, err)
	}

	return be, nil
}

// Create the backend specified by URI.
func create(s string) (restic.Backend, error) {
	debug.Log("parsing location %v", s)
	loc, err := location.Parse(s)
	if err != nil {
		return nil, err
	}

	switch loc.Scheme {
	case "local":
		debug.Log("create local repository at %#v", loc.Config)
		return local.Create(loc.Config.(string))
	case "sftp":
		debug.Log("create sftp repository at %#v", loc.Config)
		return sftp.CreateWithConfig(loc.Config.(sftp.Config))
	case "s3":
		cfg := loc.Config.(s3.Config)
		if cfg.KeyID == "" {
			cfg.KeyID = os.Getenv("AWS_ACCESS_KEY_ID")

		}
		if cfg.Secret == "" {
			cfg.Secret = os.Getenv("AWS_SECRET_ACCESS_KEY")
		}

		debug.Log("create s3 repository at %#v", loc.Config)
		return s3.Open(cfg)
	case "rest":
		return rest.Open(loc.Config.(rest.Config))
	}

	debug.Log("invalid repository scheme: %v", s)
	return nil, errors.Fatalf("invalid scheme %q", loc.Scheme)
}
