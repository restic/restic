package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/azure"
	"github.com/restic/restic/internal/backend/b2"
	"github.com/restic/restic/internal/backend/cache"
	"github.com/restic/restic/internal/backend/gs"
	"github.com/restic/restic/internal/backend/limiter"
	"github.com/restic/restic/internal/backend/local"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/backend/logger"
	"github.com/restic/restic/internal/backend/rclone"
	"github.com/restic/restic/internal/backend/rest"
	"github.com/restic/restic/internal/backend/retry"
	"github.com/restic/restic/internal/backend/s3"
	"github.com/restic/restic/internal/backend/sema"
	"github.com/restic/restic/internal/backend/sftp"
	"github.com/restic/restic/internal/backend/swift"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/options"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/textfile"
	"github.com/restic/restic/internal/ui/termstatus"

	"github.com/restic/restic/internal/errors"

	"os/exec"

	"golang.org/x/term"
)

// ErrNoRepository is used to report if opening a repsitory failed due
// to a missing backend storage location or config file
var ErrNoRepository = errors.New("repository does not exist")

var version = "0.17.3-dev (compiled manually)"

// TimeFormat is the format used for all timestamps printed by restic.
const TimeFormat = "2006-01-02 15:04:05"

type backendWrapper func(r backend.Backend) (backend.Backend, error)

// GlobalOptions hold all global options for restic.
type GlobalOptions struct {
	Repo               string
	RepositoryFile     string
	PasswordFile       string
	PasswordCommand    string
	KeyHint            string
	Quiet              bool
	Verbose            int
	NoLock             bool
	RetryLock          time.Duration
	JSON               bool
	CacheDir           string
	NoCache            bool
	CleanupCache       bool
	Compression        repository.CompressionMode
	PackSize           uint
	NoExtraVerify      bool
	InsecureNoPassword bool

	backend.TransportOptions
	limiter.Limits

	password string
	stdout   io.Writer
	stderr   io.Writer

	backends                              *location.Registry
	backendTestHook, backendInnerTestHook backendWrapper

	// verbosity is set as follows:
	//  0 means: don't print any messages except errors, this is used when --quiet is specified
	//  1 is the default: print essential messages
	//  2 means: print more messages, report minor things, this is used when --verbose is specified
	//  3 means: print very detailed debug messages, this is used when --verbose=2 is specified
	verbosity uint

	Options []string

	extended options.Options
}

var globalOptions = GlobalOptions{
	stdout: os.Stdout,
	stderr: os.Stderr,
}

func init() {
	backends := location.NewRegistry()
	backends.Register(azure.NewFactory())
	backends.Register(b2.NewFactory())
	backends.Register(gs.NewFactory())
	backends.Register(local.NewFactory())
	backends.Register(rclone.NewFactory())
	backends.Register(rest.NewFactory())
	backends.Register(s3.NewFactory())
	backends.Register(sftp.NewFactory())
	backends.Register(swift.NewFactory())
	globalOptions.backends = backends

	f := cmdRoot.PersistentFlags()
	f.StringVarP(&globalOptions.Repo, "repo", "r", "", "`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)")
	f.StringVarP(&globalOptions.RepositoryFile, "repository-file", "", "", "`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)")
	f.StringVarP(&globalOptions.PasswordFile, "password-file", "p", "", "`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)")
	f.StringVarP(&globalOptions.KeyHint, "key-hint", "", "", "`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)")
	f.StringVarP(&globalOptions.PasswordCommand, "password-command", "", "", "shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)")
	f.BoolVarP(&globalOptions.Quiet, "quiet", "q", false, "do not output comprehensive progress report")
	// use empty parameter name as `-v, --verbose n` instead of the correct `--verbose=n` is confusing
	f.CountVarP(&globalOptions.Verbose, "verbose", "v", "be verbose (specify multiple times or a level using --verbose=n``, max level/times is 2)")
	f.BoolVar(&globalOptions.NoLock, "no-lock", false, "do not lock the repository, this allows some operations on read-only repositories")
	f.DurationVar(&globalOptions.RetryLock, "retry-lock", 0, "retry to lock the repository if it is already locked, takes a value like 5m or 2h (default: no retries)")
	f.BoolVarP(&globalOptions.JSON, "json", "", false, "set output mode to JSON for commands that support it")
	f.StringVar(&globalOptions.CacheDir, "cache-dir", "", "set the cache `directory`. (default: use system default cache directory)")
	f.BoolVar(&globalOptions.NoCache, "no-cache", false, "do not use a local cache")
	f.StringSliceVar(&globalOptions.RootCertFilenames, "cacert", nil, "`file` to load root certificates from (default: use system certificates or $RESTIC_CACERT)")
	f.StringVar(&globalOptions.TLSClientCertKeyFilename, "tls-client-cert", "", "path to a `file` containing PEM encoded TLS client certificate and private key (default: $RESTIC_TLS_CLIENT_CERT)")
	f.BoolVar(&globalOptions.InsecureNoPassword, "insecure-no-password", false, "use an empty password for the repository, must be passed to every restic command (insecure)")
	f.BoolVar(&globalOptions.InsecureTLS, "insecure-tls", false, "skip TLS certificate verification when connecting to the repository (insecure)")
	f.BoolVar(&globalOptions.CleanupCache, "cleanup-cache", false, "auto remove old cache directories")
	f.Var(&globalOptions.Compression, "compression", "compression mode (only available for repository format version 2), one of (auto|off|max) (default: $RESTIC_COMPRESSION)")
	f.BoolVar(&globalOptions.NoExtraVerify, "no-extra-verify", false, "skip additional verification of data before upload (see documentation)")
	f.IntVar(&globalOptions.Limits.UploadKb, "limit-upload", 0, "limits uploads to a maximum `rate` in KiB/s. (default: unlimited)")
	f.IntVar(&globalOptions.Limits.DownloadKb, "limit-download", 0, "limits downloads to a maximum `rate` in KiB/s. (default: unlimited)")
	f.UintVar(&globalOptions.PackSize, "pack-size", 0, "set target pack `size` in MiB, created pack files may be larger (default: $RESTIC_PACK_SIZE)")
	f.StringSliceVarP(&globalOptions.Options, "option", "o", []string{}, "set extended option (`key=value`, can be specified multiple times)")
	f.StringVar(&globalOptions.HTTPUserAgent, "http-user-agent", "", "set a http user agent for outgoing http requests")
	f.DurationVar(&globalOptions.StuckRequestTimeout, "stuck-request-timeout", 5*time.Minute, "`duration` after which to retry stuck requests")
	// Use our "generate" command instead of the cobra provided "completion" command
	cmdRoot.CompletionOptions.DisableDefaultCmd = true

	globalOptions.Repo = os.Getenv("RESTIC_REPOSITORY")
	globalOptions.RepositoryFile = os.Getenv("RESTIC_REPOSITORY_FILE")
	globalOptions.PasswordFile = os.Getenv("RESTIC_PASSWORD_FILE")
	globalOptions.KeyHint = os.Getenv("RESTIC_KEY_HINT")
	globalOptions.PasswordCommand = os.Getenv("RESTIC_PASSWORD_COMMAND")
	if os.Getenv("RESTIC_CACERT") != "" {
		globalOptions.RootCertFilenames = strings.Split(os.Getenv("RESTIC_CACERT"), ",")
	}
	globalOptions.TLSClientCertKeyFilename = os.Getenv("RESTIC_TLS_CLIENT_CERT")
	comp := os.Getenv("RESTIC_COMPRESSION")
	if comp != "" {
		// ignore error as there's no good way to handle it
		_ = globalOptions.Compression.Set(comp)
	}
	// parse target pack size from env, on error the default value will be used
	targetPackSize, _ := strconv.ParseUint(os.Getenv("RESTIC_PACK_SIZE"), 10, 32)
	globalOptions.PackSize = uint(targetPackSize)

	if os.Getenv("RESTIC_HTTP_USER_AGENT") != "" {
		globalOptions.HTTPUserAgent = os.Getenv("RESTIC_HTTP_USER_AGENT")
	}
}

func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func stdoutIsTerminal() bool {
	// mintty on windows can use pipes which behave like a posix terminal,
	// but which are not a terminal handle
	return term.IsTerminal(int(os.Stdout.Fd())) || stdoutCanUpdateStatus()
}

func stdoutCanUpdateStatus() bool {
	return termstatus.CanUpdateStatus(os.Stdout.Fd())
}

func stdoutTerminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0
	}
	return w
}

// ClearLine creates a platform dependent string to clear the current
// line, so it can be overwritten.
//
// w should be the terminal width, or 0 to let clearLine figure it out.
func clearLine(w int) string {
	if runtime.GOOS != "windows" {
		return "\x1b[2K"
	}

	// ANSI sequences are not supported on Windows cmd shell.
	if w <= 0 {
		if w = stdoutTerminalWidth(); w <= 0 {
			return ""
		}
	}
	return strings.Repeat(" ", w-1) + "\r"
}

// Printf writes the message to the configured stdout stream.
func Printf(format string, args ...interface{}) {
	_, err := fmt.Fprintf(globalOptions.stdout, format, args...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to write to stdout: %v\n", err)
	}
}

// Print writes the message to the configured stdout stream.
func Print(args ...interface{}) {
	_, err := fmt.Fprint(globalOptions.stdout, args...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to write to stdout: %v\n", err)
	}
}

// Println writes the message to the configured stdout stream.
func Println(args ...interface{}) {
	_, err := fmt.Fprintln(globalOptions.stdout, args...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to write to stdout: %v\n", err)
	}
}

// Verbosef calls Printf to write the message when the verbose flag is set.
func Verbosef(format string, args ...interface{}) {
	if globalOptions.verbosity >= 1 {
		Printf(format, args...)
	}
}

// Verboseff calls Printf to write the message when the verbosity is >= 2
func Verboseff(format string, args ...interface{}) {
	if globalOptions.verbosity >= 2 {
		Printf(format, args...)
	}
}

// Warnf writes the message to the configured stderr stream.
func Warnf(format string, args ...interface{}) {
	_, err := fmt.Fprintf(globalOptions.stderr, format, args...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to write to stderr: %v\n", err)
	}
	debug.Log(format, args...)
}

// resolvePassword determines the password to be used for opening the repository.
func resolvePassword(opts GlobalOptions, envStr string) (string, error) {
	if opts.PasswordFile != "" && opts.PasswordCommand != "" {
		return "", errors.Fatalf("Password file and command are mutually exclusive options")
	}
	if opts.PasswordCommand != "" {
		args, err := backend.SplitShellStrings(opts.PasswordCommand)
		if err != nil {
			return "", err
		}
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stderr = os.Stderr
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		return (strings.TrimSpace(string(output))), nil
	}
	if opts.PasswordFile != "" {
		return loadPasswordFromFile(opts.PasswordFile)
	}

	if pwd := os.Getenv(envStr); pwd != "" {
		return pwd, nil
	}

	return "", nil
}

// loadPasswordFromFile loads a password from a file while stripping a BOM and
// converting the password to UTF-8.
func loadPasswordFromFile(pwdFile string) (string, error) {
	s, err := textfile.Read(pwdFile)
	if errors.Is(err, os.ErrNotExist) {
		return "", errors.Fatalf("%s does not exist", pwdFile)
	}
	return strings.TrimSpace(string(s)), errors.Wrap(err, "Readfile")
}

// readPassword reads the password from the given reader directly.
func readPassword(in io.Reader) (password string, err error) {
	sc := bufio.NewScanner(in)
	sc.Scan()

	return sc.Text(), errors.WithStack(sc.Err())
}

// readPasswordTerminal reads the password from the given reader which must be a
// tty. Prompt is printed on the writer out before attempting to read the
// password. If the context is canceled, the function leaks the password reading
// goroutine.
func readPasswordTerminal(ctx context.Context, in *os.File, out *os.File, prompt string) (password string, err error) {
	fd := int(out.Fd())
	state, err := term.GetState(fd)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "unable to get terminal state: %v\n", err)
		return "", err
	}

	done := make(chan struct{})
	var buf []byte

	go func() {
		defer close(done)
		_, err = fmt.Fprint(out, prompt)
		if err != nil {
			return
		}
		buf, err = term.ReadPassword(int(in.Fd()))
		if err != nil {
			return
		}
		_, err = fmt.Fprintln(out)
	}()

	select {
	case <-ctx.Done():
		err := term.Restore(fd, state)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "unable to restore terminal state: %v\n", err)
		}
		return "", ctx.Err()
	case <-done:
		// clean shutdown, nothing to do
	}

	if err != nil {
		return "", errors.Wrap(err, "ReadPassword")
	}

	return string(buf), nil
}

// ReadPassword reads the password from a password file, the environment
// variable RESTIC_PASSWORD or prompts the user. If the context is canceled,
// the function leaks the password reading goroutine.
func ReadPassword(ctx context.Context, opts GlobalOptions, prompt string) (string, error) {
	if opts.InsecureNoPassword {
		if opts.password != "" {
			return "", errors.Fatal("--insecure-no-password must not be specified together with providing a password via a cli option or environment variable")
		}
		return "", nil
	}

	if opts.password != "" {
		return opts.password, nil
	}

	var (
		password string
		err      error
	)

	if stdinIsTerminal() {
		password, err = readPasswordTerminal(ctx, os.Stdin, os.Stderr, prompt)
	} else {
		password, err = readPassword(os.Stdin)
		Verbosef("reading repository password from stdin\n")
	}

	if err != nil {
		return "", errors.Wrap(err, "unable to read password")
	}

	if len(password) == 0 {
		return "", errors.Fatal("an empty password is not allowed by default. Pass the flag `--insecure-no-password` to restic to disable this check")
	}

	return password, nil
}

// ReadPasswordTwice calls ReadPassword two times and returns an error when the
// passwords don't match. If the context is canceled, the function leaks the
// password reading goroutine.
func ReadPasswordTwice(ctx context.Context, gopts GlobalOptions, prompt1, prompt2 string) (string, error) {
	pw1, err := ReadPassword(ctx, gopts, prompt1)
	if err != nil {
		return "", err
	}
	if stdinIsTerminal() {
		pw2, err := ReadPassword(ctx, gopts, prompt2)
		if err != nil {
			return "", err
		}

		if pw1 != pw2 {
			return "", errors.Fatal("passwords do not match")
		}
	}

	return pw1, nil
}

func ReadRepo(opts GlobalOptions) (string, error) {
	if opts.Repo == "" && opts.RepositoryFile == "" {
		return "", errors.Fatal("Please specify repository location (-r or --repository-file)")
	}

	repo := opts.Repo
	if opts.RepositoryFile != "" {
		if repo != "" {
			return "", errors.Fatal("Options -r and --repository-file are mutually exclusive, please specify only one")
		}

		s, err := textfile.Read(opts.RepositoryFile)
		if errors.Is(err, os.ErrNotExist) {
			return "", errors.Fatalf("%s does not exist", opts.RepositoryFile)
		}
		if err != nil {
			return "", err
		}

		repo = strings.TrimSpace(string(s))
	}

	return repo, nil
}

const maxKeys = 20

// OpenRepository reads the password and opens the repository.
func OpenRepository(ctx context.Context, opts GlobalOptions) (*repository.Repository, error) {
	repo, err := ReadRepo(opts)
	if err != nil {
		return nil, err
	}

	be, err := open(ctx, repo, opts, opts.extended)
	if err != nil {
		return nil, err
	}

	s, err := repository.New(be, repository.Options{
		Compression:   opts.Compression,
		PackSize:      opts.PackSize * 1024 * 1024,
		NoExtraVerify: opts.NoExtraVerify,
	})
	if err != nil {
		return nil, errors.Fatal(err.Error())
	}

	passwordTriesLeft := 1
	if stdinIsTerminal() && opts.password == "" && !opts.InsecureNoPassword {
		passwordTriesLeft = 3
	}

	for ; passwordTriesLeft > 0; passwordTriesLeft-- {
		opts.password, err = ReadPassword(ctx, opts, "enter password for repository: ")
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil && passwordTriesLeft > 1 {
			opts.password = ""
			fmt.Printf("%s. Try again\n", err)
		}
		if err != nil {
			continue
		}

		err = s.SearchKey(ctx, opts.password, maxKeys, opts.KeyHint)
		if err != nil && passwordTriesLeft > 1 {
			opts.password = ""
			fmt.Fprintf(os.Stderr, "%s. Try again\n", err)
		}
	}
	if err != nil {
		if errors.IsFatal(err) || errors.Is(err, repository.ErrNoKeyFound) {
			return nil, err
		}
		return nil, errors.Fatalf("%s", err)
	}

	if stdoutIsTerminal() && !opts.JSON {
		id := s.Config().ID
		if len(id) > 8 {
			id = id[:8]
		}
		if !opts.JSON {
			extra := ""
			if s.Config().Version >= 2 {
				extra = ", compression level " + opts.Compression.String()
			}
			Verbosef("repository %v opened (version %v%s)\n", id, s.Config().Version, extra)
		}
	}

	if opts.NoCache {
		return s, nil
	}

	c, err := cache.New(s.Config().ID, opts.CacheDir)
	if err != nil {
		Warnf("unable to open cache: %v\n", err)
		return s, nil
	}

	if c.Created && !opts.JSON && stdoutIsTerminal() {
		Verbosef("created new cache in %v\n", c.Base)
	}

	// start using the cache
	s.UseCache(c)

	oldCacheDirs, err := cache.Old(c.Base)
	if err != nil {
		Warnf("unable to find old cache directories: %v", err)
	}

	// nothing more to do if no old cache dirs could be found
	if len(oldCacheDirs) == 0 {
		return s, nil
	}

	// cleanup old cache dirs if instructed to do so
	if opts.CleanupCache {
		if stdoutIsTerminal() && !opts.JSON {
			Verbosef("removing %d old cache dirs from %v\n", len(oldCacheDirs), c.Base)
		}
		for _, item := range oldCacheDirs {
			dir := filepath.Join(c.Base, item.Name())
			err = os.RemoveAll(dir)
			if err != nil {
				Warnf("unable to remove %v: %v\n", dir, err)
			}
		}
	} else {
		if stdoutIsTerminal() {
			Verbosef("found %d old cache directories in %v, run `restic cache --cleanup` to remove them\n",
				len(oldCacheDirs), c.Base)
		}
	}

	return s, nil
}

func parseConfig(loc location.Location, opts options.Options) (interface{}, error) {
	cfg := loc.Config
	if cfg, ok := cfg.(backend.ApplyEnvironmenter); ok {
		cfg.ApplyEnvironment("")
	}

	// only apply options for a particular backend here
	opts = opts.Extract(loc.Scheme)
	if err := opts.Apply(loc.Scheme, cfg); err != nil {
		return nil, err
	}

	debug.Log("opening %v repository at %#v", loc.Scheme, cfg)
	return cfg, nil
}

func innerOpen(ctx context.Context, s string, gopts GlobalOptions, opts options.Options, create bool) (backend.Backend, error) {
	debug.Log("parsing location %v", location.StripPassword(gopts.backends, s))
	loc, err := location.Parse(gopts.backends, s)
	if err != nil {
		return nil, errors.Fatalf("parsing repository location failed: %v", err)
	}

	cfg, err := parseConfig(loc, opts)
	if err != nil {
		return nil, err
	}

	rt, err := backend.Transport(globalOptions.TransportOptions)
	if err != nil {
		return nil, errors.Fatal(err.Error())
	}

	// wrap the transport so that the throughput via HTTP is limited
	lim := limiter.NewStaticLimiter(gopts.Limits)
	rt = lim.Transport(rt)

	factory := gopts.backends.Lookup(loc.Scheme)
	if factory == nil {
		return nil, errors.Fatalf("invalid backend: %q", loc.Scheme)
	}

	var be backend.Backend
	if create {
		be, err = factory.Create(ctx, cfg, rt, lim)
	} else {
		be, err = factory.Open(ctx, cfg, rt, lim)
	}

	if errors.Is(err, backend.ErrNoRepository) {
		return nil, fmt.Errorf("Fatal: %w at %v: %v", ErrNoRepository, location.StripPassword(gopts.backends, s), err)
	}
	if err != nil {
		return nil, errors.Fatalf("unable to open repository at %v: %v", location.StripPassword(gopts.backends, s), err)
	}

	// wrap with debug logging and connection limiting
	be = logger.New(sema.NewBackend(be))

	// wrap backend if a test specified an inner hook
	if gopts.backendInnerTestHook != nil {
		be, err = gopts.backendInnerTestHook(be)
		if err != nil {
			return nil, err
		}
	}

	report := func(msg string, err error, d time.Duration) {
		if d >= 0 {
			Warnf("%v returned error, retrying after %v: %v\n", msg, d, err)
		} else {
			Warnf("%v failed: %v\n", msg, err)
		}
	}
	success := func(msg string, retries int) {
		Warnf("%v operation successful after %d retries\n", msg, retries)
	}
	be = retry.New(be, 15*time.Minute, report, success)

	// wrap backend if a test specified a hook
	if gopts.backendTestHook != nil {
		be, err = gopts.backendTestHook(be)
		if err != nil {
			return nil, err
		}
	}

	return be, nil
}

// Open the backend specified by a location config.
func open(ctx context.Context, s string, gopts GlobalOptions, opts options.Options) (backend.Backend, error) {
	be, err := innerOpen(ctx, s, gopts, opts, false)
	if err != nil {
		return nil, err
	}

	// check if config is there
	fi, err := be.Stat(ctx, backend.Handle{Type: restic.ConfigFile})
	if be.IsNotExist(err) {
		return nil, fmt.Errorf("Fatal: %w: unable to open config file: %v\nIs there a repository at the following location?\n%v", ErrNoRepository, err, location.StripPassword(gopts.backends, s))
	}
	if err != nil {
		return nil, errors.Fatalf("unable to open config file: %v\nIs there a repository at the following location?\n%v", err, location.StripPassword(gopts.backends, s))
	}

	if fi.Size == 0 {
		return nil, errors.New("config file has zero size, invalid repository?")
	}

	return be, nil
}

// Create the backend specified by URI.
func create(ctx context.Context, s string, gopts GlobalOptions, opts options.Options) (backend.Backend, error) {
	return innerOpen(ctx, s, gopts, opts, true)
}
