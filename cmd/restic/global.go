package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	"github.com/restic/restic/internal/terminal"
	"github.com/restic/restic/internal/textfile"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/spf13/pflag"

	"github.com/restic/restic/internal/errors"
)

// ErrNoRepository is used to report if opening a repository failed due
// to a missing backend storage location or config file
var ErrNoRepository = errors.New("repository does not exist")

var version = "0.18.1-dev (compiled manually)"

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

func (opts *GlobalOptions) AddFlags(f *pflag.FlagSet) {
	f.StringVarP(&opts.Repo, "repo", "r", "", "`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)")
	f.StringVarP(&opts.RepositoryFile, "repository-file", "", "", "`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)")
	f.StringVarP(&opts.PasswordFile, "password-file", "p", "", "`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)")
	f.StringVarP(&opts.KeyHint, "key-hint", "", "", "`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)")
	f.StringVarP(&opts.PasswordCommand, "password-command", "", "", "shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)")
	f.BoolVarP(&opts.Quiet, "quiet", "q", false, "do not output comprehensive progress report")
	// use empty parameter name as `-v, --verbose n` instead of the correct `--verbose=n` is confusing
	f.CountVarP(&opts.Verbose, "verbose", "v", "be verbose (specify multiple times or a level using --verbose=n``, max level/times is 2)")
	f.BoolVar(&opts.NoLock, "no-lock", false, "do not lock the repository, this allows some operations on read-only repositories")
	f.DurationVar(&opts.RetryLock, "retry-lock", 0, "retry to lock the repository if it is already locked, takes a value like 5m or 2h (default: no retries)")
	f.BoolVarP(&opts.JSON, "json", "", false, "set output mode to JSON for commands that support it")
	f.StringVar(&opts.CacheDir, "cache-dir", "", "set the cache `directory`. (default: use system default cache directory)")
	f.BoolVar(&opts.NoCache, "no-cache", false, "do not use a local cache")
	f.StringSliceVar(&opts.RootCertFilenames, "cacert", nil, "`file` to load root certificates from (default: use system certificates or $RESTIC_CACERT)")
	f.StringVar(&opts.TLSClientCertKeyFilename, "tls-client-cert", "", "path to a `file` containing PEM encoded TLS client certificate and private key (default: $RESTIC_TLS_CLIENT_CERT)")
	f.BoolVar(&opts.InsecureNoPassword, "insecure-no-password", false, "use an empty password for the repository, must be passed to every restic command (insecure)")
	f.BoolVar(&opts.InsecureTLS, "insecure-tls", false, "skip TLS certificate verification when connecting to the repository (insecure)")
	f.BoolVar(&opts.CleanupCache, "cleanup-cache", false, "auto remove old cache directories")
	f.Var(&opts.Compression, "compression", "compression mode (only available for repository format version 2), one of (auto|off|fastest|better|max) (default: $RESTIC_COMPRESSION)")
	f.BoolVar(&opts.NoExtraVerify, "no-extra-verify", false, "skip additional verification of data before upload (see documentation)")
	f.IntVar(&opts.Limits.UploadKb, "limit-upload", 0, "limits uploads to a maximum `rate` in KiB/s. (default: unlimited)")
	f.IntVar(&opts.Limits.DownloadKb, "limit-download", 0, "limits downloads to a maximum `rate` in KiB/s. (default: unlimited)")
	f.UintVar(&opts.PackSize, "pack-size", 0, "set target pack `size` in MiB, created pack files may be larger (default: $RESTIC_PACK_SIZE)")
	f.StringSliceVarP(&opts.Options, "option", "o", []string{}, "set extended option (`key=value`, can be specified multiple times)")
	f.StringVar(&opts.HTTPUserAgent, "http-user-agent", "", "set a http user agent for outgoing http requests")
	f.DurationVar(&opts.StuckRequestTimeout, "stuck-request-timeout", 5*time.Minute, "`duration` after which to retry stuck requests")

	opts.Repo = os.Getenv("RESTIC_REPOSITORY")
	opts.RepositoryFile = os.Getenv("RESTIC_REPOSITORY_FILE")
	opts.PasswordFile = os.Getenv("RESTIC_PASSWORD_FILE")
	opts.KeyHint = os.Getenv("RESTIC_KEY_HINT")
	opts.PasswordCommand = os.Getenv("RESTIC_PASSWORD_COMMAND")
	if os.Getenv("RESTIC_CACERT") != "" {
		opts.RootCertFilenames = strings.Split(os.Getenv("RESTIC_CACERT"), ",")
	}
	opts.TLSClientCertKeyFilename = os.Getenv("RESTIC_TLS_CLIENT_CERT")
	comp := os.Getenv("RESTIC_COMPRESSION")
	if comp != "" {
		// ignore error as there's no good way to handle it
		_ = opts.Compression.Set(comp)
	}
	// parse target pack size from env, on error the default value will be used
	targetPackSize, _ := strconv.ParseUint(os.Getenv("RESTIC_PACK_SIZE"), 10, 32)
	opts.PackSize = uint(targetPackSize)

	if os.Getenv("RESTIC_HTTP_USER_AGENT") != "" {
		opts.HTTPUserAgent = os.Getenv("RESTIC_HTTP_USER_AGENT")
	}
}

func (opts *GlobalOptions) PreRun(needsPassword bool) error {
	// set verbosity, default is one
	opts.verbosity = 1
	if opts.Quiet && opts.Verbose > 0 {
		return errors.Fatal("--quiet and --verbose cannot be specified at the same time")
	}

	switch {
	case opts.Verbose >= 2:
		opts.verbosity = 3
	case opts.Verbose > 0:
		opts.verbosity = 2
	case opts.Quiet:
		opts.verbosity = 0
	}

	// parse extended options
	extendedOpts, err := options.Parse(opts.Options)
	if err != nil {
		return err
	}
	opts.extended = extendedOpts
	if !needsPassword {
		return nil
	}
	pwd, err := resolvePassword(opts, "RESTIC_PASSWORD")
	if err != nil {
		return errors.Fatal(fmt.Sprintf("Resolving password failed: %v\n", err))
	}
	opts.password = pwd
	return nil
}

var globalOptions = GlobalOptions{
	stdout:   os.Stdout,
	stderr:   os.Stderr,
	backends: collectBackends(),
}

func collectBackends() *location.Registry {
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
	return backends
}

// resolvePassword determines the password to be used for opening the repository.
func resolvePassword(opts *GlobalOptions, envStr string) (string, error) {
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
		return strings.TrimSpace(string(output)), nil
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

// ReadPassword reads the password from a password file, the environment
// variable RESTIC_PASSWORD or prompts the user. If the context is canceled,
// the function leaks the password reading goroutine.
func ReadPassword(ctx context.Context, opts GlobalOptions, prompt string, printer progress.Printer) (string, error) {
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

	if terminal.StdinIsTerminal() {
		password, err = terminal.ReadPassword(ctx, os.Stdin, os.Stderr, prompt)
	} else {
		if terminal.StdoutIsTerminal() {
			printer.P("reading repository password from stdin")
		}
		password, err = readPassword(os.Stdin)
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
func ReadPasswordTwice(ctx context.Context, gopts GlobalOptions, prompt1, prompt2 string, printer progress.Printer) (string, error) {
	pw1, err := ReadPassword(ctx, gopts, prompt1, printer)
	if err != nil {
		return "", err
	}
	if terminal.StdinIsTerminal() {
		pw2, err := ReadPassword(ctx, gopts, prompt2, printer)
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
func OpenRepository(ctx context.Context, opts GlobalOptions, printer progress.Printer) (*repository.Repository, error) {
	repo, err := ReadRepo(opts)
	if err != nil {
		return nil, err
	}

	be, err := open(ctx, repo, opts, opts.extended, printer)
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
	if terminal.StdinIsTerminal() && opts.password == "" && !opts.InsecureNoPassword {
		passwordTriesLeft = 3
	}

	for ; passwordTriesLeft > 0; passwordTriesLeft-- {
		opts.password, err = ReadPassword(ctx, opts, "enter password for repository: ", printer)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil && passwordTriesLeft > 1 {
			opts.password = ""
			printer.E("%s. Try again", err)
		}
		if err != nil {
			continue
		}

		err = s.SearchKey(ctx, opts.password, maxKeys, opts.KeyHint)
		if err != nil && passwordTriesLeft > 1 {
			opts.password = ""
			printer.E("%s. Try again", err)
		}
	}
	if err != nil {
		if errors.IsFatal(err) || errors.Is(err, repository.ErrNoKeyFound) {
			return nil, err
		}
		return nil, errors.Fatalf("%s", err)
	}

	if terminal.StdoutIsTerminal() && !opts.JSON {
		id := s.Config().ID
		if len(id) > 8 {
			id = id[:8]
		}
		if !opts.JSON {
			extra := ""
			if s.Config().Version >= 2 {
				extra = ", compression level " + opts.Compression.String()
			}
			printer.P("repository %v opened (version %v%s)", id, s.Config().Version, extra)
		}
	}

	if opts.NoCache {
		return s, nil
	}

	c, err := cache.New(s.Config().ID, opts.CacheDir)
	if err != nil {
		printer.E("unable to open cache: %v", err)
		return s, nil
	}

	if c.Created && !opts.JSON && terminal.StdoutIsTerminal() {
		printer.P("created new cache in %v", c.Base)
	}

	// start using the cache
	s.UseCache(c)

	oldCacheDirs, err := cache.Old(c.Base)
	if err != nil {
		printer.E("unable to find old cache directories: %v", err)
	}

	// nothing more to do if no old cache dirs could be found
	if len(oldCacheDirs) == 0 {
		return s, nil
	}

	// cleanup old cache dirs if instructed to do so
	if opts.CleanupCache {
		if terminal.StdoutIsTerminal() && !opts.JSON {
			printer.P("removing %d old cache dirs from %v", len(oldCacheDirs), c.Base)
		}
		for _, item := range oldCacheDirs {
			dir := filepath.Join(c.Base, item.Name())
			err = os.RemoveAll(dir)
			if err != nil {
				printer.E("unable to remove %v: %v", dir, err)
			}
		}
	} else {
		if terminal.StdoutIsTerminal() {
			printer.P("found %d old cache directories in %v, run `restic cache --cleanup` to remove them",
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

func innerOpen(ctx context.Context, s string, gopts GlobalOptions, opts options.Options, create bool, printer progress.Printer) (backend.Backend, error) {
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
		//nolint:staticcheck // capitalized error string is intentional
		return nil, fmt.Errorf("Fatal: %w at %v: %v", ErrNoRepository, location.StripPassword(gopts.backends, s), err)
	}
	if err != nil {
		if create {
			// init already wraps the error message
			return nil, err
		}
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
			printer.E("%v returned error, retrying after %v: %v", msg, d, err)
		} else {
			printer.E("%v failed: %v", msg, err)
		}
	}
	success := func(msg string, retries int) {
		printer.E("%v operation successful after %d retries", msg, retries)
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
func open(ctx context.Context, s string, gopts GlobalOptions, opts options.Options, printer progress.Printer) (backend.Backend, error) {
	be, err := innerOpen(ctx, s, gopts, opts, false, printer)
	if err != nil {
		return nil, err
	}

	// check if config is there
	fi, err := be.Stat(ctx, backend.Handle{Type: restic.ConfigFile})
	if be.IsNotExist(err) {
		//nolint:staticcheck // capitalized error string is intentional
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
func create(ctx context.Context, s string, gopts GlobalOptions, opts options.Options, printer progress.Printer) (backend.Backend, error) {
	return innerOpen(ctx, s, gopts, opts, true, printer)
}
