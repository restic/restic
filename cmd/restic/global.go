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
	"syscall"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/azure"
	"github.com/restic/restic/internal/backend/b2"
	"github.com/restic/restic/internal/backend/gs"
	"github.com/restic/restic/internal/backend/limiter"
	"github.com/restic/restic/internal/backend/local"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/backend/rclone"
	"github.com/restic/restic/internal/backend/rest"
	"github.com/restic/restic/internal/backend/retry"
	"github.com/restic/restic/internal/backend/s3"
	"github.com/restic/restic/internal/backend/sftp"
	"github.com/restic/restic/internal/backend/swift"
	"github.com/restic/restic/internal/cache"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/options"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/textfile"
	"github.com/restic/restic/internal/ui/termstatus"

	"github.com/restic/restic/internal/errors"

	"os/exec"

	"golang.org/x/term"
)

var version = "0.14.0-dev (compiled manually)"

// TimeFormat is the format used for all timestamps printed by restic.
const TimeFormat = "2006-01-02 15:04:05"

type backendWrapper func(r restic.Backend) (restic.Backend, error)

// GlobalOptions hold all global options for restic.
type GlobalOptions struct {
	Repo            string
	RepositoryFile  string
	PasswordFile    string
	PasswordCommand string
	KeyHint         string
	Quiet           bool
	Verbose         int
	NoLock          bool
	JSON            bool
	CacheDir        string
	NoCache         bool
	CleanupCache    bool
	Compression     repository.CompressionMode
	PackSize        uint

	backend.TransportOptions
	limiter.Limits

	password string
	stdout   io.Writer
	stderr   io.Writer

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

var isReadingPassword bool
var internalGlobalCtx context.Context

func init() {
	var cancel context.CancelFunc
	internalGlobalCtx, cancel = context.WithCancel(context.Background())
	AddCleanupHandler(func(code int) (int, error) {
		// Must be called before the unlock cleanup handler to ensure that the latter is
		// not blocked due to limited number of backend connections, see #1434
		cancel()
		return code, nil
	})

	f := cmdRoot.PersistentFlags()
	f.StringVarP(&globalOptions.Repo, "repo", "r", "", "`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)")
	f.StringVarP(&globalOptions.RepositoryFile, "repository-file", "", "", "`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)")
	f.StringVarP(&globalOptions.PasswordFile, "password-file", "p", "", "`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)")
	f.StringVarP(&globalOptions.KeyHint, "key-hint", "", "", "`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)")
	f.StringVarP(&globalOptions.PasswordCommand, "password-command", "", "", "shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)")
	f.BoolVarP(&globalOptions.Quiet, "quiet", "q", false, "do not output comprehensive progress report")
	f.CountVarP(&globalOptions.Verbose, "verbose", "v", "be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)")
	f.BoolVar(&globalOptions.NoLock, "no-lock", false, "do not lock the repository, this allows some operations on read-only repositories")
	f.BoolVarP(&globalOptions.JSON, "json", "", false, "set output mode to JSON for commands that support it")
	f.StringVar(&globalOptions.CacheDir, "cache-dir", "", "set the cache `directory`. (default: use system default cache directory)")
	f.BoolVar(&globalOptions.NoCache, "no-cache", false, "do not use a local cache")
	f.StringSliceVar(&globalOptions.RootCertFilenames, "cacert", nil, "`file` to load root certificates from (default: use system certificates)")
	f.StringVar(&globalOptions.TLSClientCertKeyFilename, "tls-client-cert", "", "path to a `file` containing PEM encoded TLS client certificate and private key")
	f.BoolVar(&globalOptions.InsecureTLS, "insecure-tls", false, "skip TLS certificate verification when connecting to the repository (insecure)")
	f.BoolVar(&globalOptions.CleanupCache, "cleanup-cache", false, "auto remove old cache directories")
	f.Var(&globalOptions.Compression, "compression", "compression mode (only available for repository format version 2), one of (auto|off|max)")
	f.IntVar(&globalOptions.Limits.UploadKb, "limit-upload", 0, "limits uploads to a maximum `rate` in KiB/s. (default: unlimited)")
	f.IntVar(&globalOptions.Limits.DownloadKb, "limit-download", 0, "limits downloads to a maximum `rate` in KiB/s. (default: unlimited)")
	f.UintVar(&globalOptions.PackSize, "pack-size", 0, "set target pack `size` in MiB, created pack files may be larger (default: $RESTIC_PACK_SIZE)")
	f.StringSliceVarP(&globalOptions.Options, "option", "o", []string{}, "set extended option (`key=value`, can be specified multiple times)")
	// Use our "generate" command instead of the cobra provided "completion" command
	cmdRoot.CompletionOptions.DisableDefaultCmd = true

	globalOptions.Repo = os.Getenv("RESTIC_REPOSITORY")
	globalOptions.RepositoryFile = os.Getenv("RESTIC_REPOSITORY_FILE")
	globalOptions.PasswordFile = os.Getenv("RESTIC_PASSWORD_FILE")
	globalOptions.KeyHint = os.Getenv("RESTIC_KEY_HINT")
	globalOptions.PasswordCommand = os.Getenv("RESTIC_PASSWORD_COMMAND")
	comp := os.Getenv("RESTIC_COMPRESSION")
	if comp != "" {
		// ignore error as there's no good way to handle it
		_ = globalOptions.Compression.Set(comp)
	}
	// parse target pack size from env, on error the default value will be used
	targetPackSize, _ := strconv.ParseUint(os.Getenv("RESTIC_PACK_SIZE"), 10, 32)
	globalOptions.PackSize = uint(targetPackSize)

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

// restoreTerminal installs a cleanup handler that restores the previous
// terminal state on exit. This handler is only intended to restore the
// terminal configuration if restic exits after receiving a signal. A regular
// program execution must revert changes to the terminal configuration itself.
// The terminal configuration is only restored while reading a password.
func restoreTerminal() {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return
	}

	fd := int(os.Stdout.Fd())
	state, err := term.GetState(fd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to get terminal state: %v\n", err)
		return
	}

	AddCleanupHandler(func(code int) (int, error) {
		// Restoring the terminal configuration while restic runs in the
		// background, causes restic to get stopped on unix systems with
		// a SIGTTOU signal. Thus only restore the terminal settings if
		// they might have been modified, which is the case while reading
		// a password.
		if !isReadingPassword {
			return code, nil
		}
		err := checkErrno(term.Restore(fd, state))
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to restore terminal state: %v\n", err)
		}
		return code, err
	})
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
}

// Exitf uses Warnf to write the message and then terminates the process with
// the given exit code.
func Exitf(exitcode int, format string, args ...interface{}) {
	if !(strings.HasSuffix(format, "\n")) {
		format += "\n"
	}

	Warnf(format, args...)
	Exit(exitcode)
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
		s, err := textfile.Read(opts.PasswordFile)
		if errors.Is(err, os.ErrNotExist) {
			return "", errors.Fatalf("%s does not exist", opts.PasswordFile)
		}
		return strings.TrimSpace(string(s)), errors.Wrap(err, "Readfile")
	}

	if pwd := os.Getenv(envStr); pwd != "" {
		return pwd, nil
	}

	return "", nil
}

// readPassword reads the password from the given reader directly.
func readPassword(in io.Reader) (password string, err error) {
	sc := bufio.NewScanner(in)
	sc.Scan()

	return sc.Text(), errors.Wrap(err, "Scan")
}

// readPasswordTerminal reads the password from the given reader which must be a
// tty. Prompt is printed on the writer out before attempting to read the
// password.
func readPasswordTerminal(in *os.File, out io.Writer, prompt string) (password string, err error) {
	fmt.Fprint(out, prompt)
	isReadingPassword = true
	buf, err := term.ReadPassword(int(in.Fd()))
	isReadingPassword = false
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
	if opts.password != "" {
		return opts.password, nil
	}

	var (
		password string
		err      error
	)

	if stdinIsTerminal() {
		password, err = readPasswordTerminal(os.Stdin, os.Stderr, prompt)
	} else {
		password, err = readPassword(os.Stdin)
		Verbosef("reading repository password from stdin\n")
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
	if stdinIsTerminal() {
		pw2, err := ReadPassword(gopts, prompt2)
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

	report := func(msg string, err error, d time.Duration) {
		Warnf("%v returned error, retrying after %v: %v\n", msg, d, err)
	}
	success := func(msg string, retries int) {
		Warnf("%v operation successful after %d retries\n", msg, retries)
	}
	be = retry.New(be, 10, report, success)

	// wrap backend if a test specified a hook
	if opts.backendTestHook != nil {
		be, err = opts.backendTestHook(be)
		if err != nil {
			return nil, err
		}
	}

	s, err := repository.New(be, repository.Options{
		Compression: opts.Compression,
		PackSize:    opts.PackSize * 1024 * 1024,
	})
	if err != nil {
		return nil, err
	}

	passwordTriesLeft := 1
	if stdinIsTerminal() && opts.password == "" {
		passwordTriesLeft = 3
	}

	for ; passwordTriesLeft > 0; passwordTriesLeft-- {
		opts.password, err = ReadPassword(opts, "enter password for repository: ")
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
		if errors.IsFatal(err) {
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
			err = fs.RemoveAll(dir)
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
	// only apply options for a particular backend here
	opts = opts.Extract(loc.Scheme)

	switch loc.Scheme {
	case "local":
		cfg := loc.Config.(local.Config)
		if err := opts.Apply(loc.Scheme, &cfg); err != nil {
			return nil, err
		}

		debug.Log("opening local repository at %#v", cfg)
		return cfg, nil

	case "sftp":
		cfg := loc.Config.(sftp.Config)
		if err := opts.Apply(loc.Scheme, &cfg); err != nil {
			return nil, err
		}

		debug.Log("opening sftp repository at %#v", cfg)
		return cfg, nil

	case "s3":
		cfg := loc.Config.(s3.Config)
		if cfg.KeyID == "" {
			cfg.KeyID = os.Getenv("AWS_ACCESS_KEY_ID")
		}

		if cfg.Secret.String() == "" {
			cfg.Secret = options.NewSecretString(os.Getenv("AWS_SECRET_ACCESS_KEY"))
		}

		if cfg.KeyID == "" && cfg.Secret.String() != "" {
			return nil, errors.Fatalf("unable to open S3 backend: Key ID ($AWS_ACCESS_KEY_ID) is empty")
		} else if cfg.KeyID != "" && cfg.Secret.String() == "" {
			return nil, errors.Fatalf("unable to open S3 backend: Secret ($AWS_SECRET_ACCESS_KEY) is empty")
		}

		if cfg.Region == "" {
			cfg.Region = os.Getenv("AWS_DEFAULT_REGION")
		}

		if err := opts.Apply(loc.Scheme, &cfg); err != nil {
			return nil, err
		}

		debug.Log("opening s3 repository at %#v", cfg)
		return cfg, nil

	case "gs":
		cfg := loc.Config.(gs.Config)
		if cfg.ProjectID == "" {
			cfg.ProjectID = os.Getenv("GOOGLE_PROJECT_ID")
		}

		if err := opts.Apply(loc.Scheme, &cfg); err != nil {
			return nil, err
		}

		debug.Log("opening gs repository at %#v", cfg)
		return cfg, nil

	case "azure":
		cfg := loc.Config.(azure.Config)
		if cfg.AccountName == "" {
			cfg.AccountName = os.Getenv("AZURE_ACCOUNT_NAME")
		}

		if cfg.AccountKey.String() == "" {
			cfg.AccountKey = options.NewSecretString(os.Getenv("AZURE_ACCOUNT_KEY"))
		}

		if cfg.AccountSAS.String() == "" {
			cfg.AccountSAS = options.NewSecretString(os.Getenv("AZURE_ACCOUNT_SAS"))
		}

		if err := opts.Apply(loc.Scheme, &cfg); err != nil {
			return nil, err
		}

		debug.Log("opening gs repository at %#v", cfg)
		return cfg, nil

	case "swift":
		cfg := loc.Config.(swift.Config)

		if err := swift.ApplyEnvironment("", &cfg); err != nil {
			return nil, err
		}

		if err := opts.Apply(loc.Scheme, &cfg); err != nil {
			return nil, err
		}

		debug.Log("opening swift repository at %#v", cfg)
		return cfg, nil

	case "b2":
		cfg := loc.Config.(b2.Config)

		if cfg.AccountID == "" {
			cfg.AccountID = os.Getenv("B2_ACCOUNT_ID")
		}

		if cfg.AccountID == "" {
			return nil, errors.Fatalf("unable to open B2 backend: Account ID ($B2_ACCOUNT_ID) is empty")
		}

		if cfg.Key.String() == "" {
			cfg.Key = options.NewSecretString(os.Getenv("B2_ACCOUNT_KEY"))
		}

		if cfg.Key.String() == "" {
			return nil, errors.Fatalf("unable to open B2 backend: Key ($B2_ACCOUNT_KEY) is empty")
		}

		if err := opts.Apply(loc.Scheme, &cfg); err != nil {
			return nil, err
		}

		debug.Log("opening b2 repository at %#v", cfg)
		return cfg, nil
	case "rest":
		cfg := loc.Config.(rest.Config)
		if err := opts.Apply(loc.Scheme, &cfg); err != nil {
			return nil, err
		}

		debug.Log("opening rest repository at %#v", cfg)
		return cfg, nil
	case "rclone":
		cfg := loc.Config.(rclone.Config)
		if err := opts.Apply(loc.Scheme, &cfg); err != nil {
			return nil, err
		}

		debug.Log("opening rest repository at %#v", cfg)
		return cfg, nil
	}

	return nil, errors.Fatalf("invalid backend: %q", loc.Scheme)
}

// Open the backend specified by a location config.
func open(ctx context.Context, s string, gopts GlobalOptions, opts options.Options) (restic.Backend, error) {
	debug.Log("parsing location %v", location.StripPassword(s))
	loc, err := location.Parse(s)
	if err != nil {
		return nil, errors.Fatalf("parsing repository location failed: %v", err)
	}

	var be restic.Backend

	cfg, err := parseConfig(loc, opts)
	if err != nil {
		return nil, err
	}

	rt, err := backend.Transport(globalOptions.TransportOptions)
	if err != nil {
		return nil, err
	}

	// wrap the transport so that the throughput via HTTP is limited
	lim := limiter.NewStaticLimiter(gopts.Limits)
	rt = lim.Transport(rt)

	switch loc.Scheme {
	case "local":
		be, err = local.Open(ctx, cfg.(local.Config))
	case "sftp":
		be, err = sftp.Open(ctx, cfg.(sftp.Config))
	case "s3":
		be, err = s3.Open(ctx, cfg.(s3.Config), rt)
	case "gs":
		be, err = gs.Open(cfg.(gs.Config), rt)
	case "azure":
		be, err = azure.Open(ctx, cfg.(azure.Config), rt)
	case "swift":
		be, err = swift.Open(ctx, cfg.(swift.Config), rt)
	case "b2":
		be, err = b2.Open(ctx, cfg.(b2.Config), rt)
	case "rest":
		be, err = rest.Open(cfg.(rest.Config), rt)
	case "rclone":
		be, err = rclone.Open(cfg.(rclone.Config), lim)

	default:
		return nil, errors.Fatalf("invalid backend: %q", loc.Scheme)
	}

	if err != nil {
		return nil, errors.Fatalf("unable to open repository at %v: %v", location.StripPassword(s), err)
	}

	// wrap backend if a test specified an inner hook
	if gopts.backendInnerTestHook != nil {
		be, err = gopts.backendInnerTestHook(be)
		if err != nil {
			return nil, err
		}
	}

	if loc.Scheme == "local" || loc.Scheme == "sftp" {
		// wrap the backend in a LimitBackend so that the throughput is limited
		be = limiter.LimitBackend(be, lim)
	}

	// check if config is there
	fi, err := be.Stat(ctx, restic.Handle{Type: restic.ConfigFile})
	if err != nil {
		return nil, errors.Fatalf("unable to open config file: %v\nIs there a repository at the following location?\n%v", err, location.StripPassword(s))
	}

	if fi.Size == 0 {
		return nil, errors.New("config file has zero size, invalid repository?")
	}

	return be, nil
}

// Create the backend specified by URI.
func create(ctx context.Context, s string, opts options.Options) (restic.Backend, error) {
	debug.Log("parsing location %v", s)
	loc, err := location.Parse(s)
	if err != nil {
		return nil, err
	}

	cfg, err := parseConfig(loc, opts)
	if err != nil {
		return nil, err
	}

	rt, err := backend.Transport(globalOptions.TransportOptions)
	if err != nil {
		return nil, err
	}

	switch loc.Scheme {
	case "local":
		return local.Create(ctx, cfg.(local.Config))
	case "sftp":
		return sftp.Create(ctx, cfg.(sftp.Config))
	case "s3":
		return s3.Create(ctx, cfg.(s3.Config), rt)
	case "gs":
		return gs.Create(cfg.(gs.Config), rt)
	case "azure":
		return azure.Create(ctx, cfg.(azure.Config), rt)
	case "swift":
		return swift.Open(ctx, cfg.(swift.Config), rt)
	case "b2":
		return b2.Create(ctx, cfg.(b2.Config), rt)
	case "rest":
		return rest.Create(ctx, cfg.(rest.Config), rt)
	case "rclone":
		return rclone.Create(ctx, cfg.(rclone.Config))
	}

	debug.Log("invalid repository scheme: %v", s)
	return nil, errors.Fatalf("invalid scheme %q", loc.Scheme)
}
