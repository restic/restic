package global

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/cache"
	"github.com/restic/restic/internal/backend/limiter"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/backend/logger"
	"github.com/restic/restic/internal/backend/retry"
	"github.com/restic/restic/internal/backend/sema"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/options"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/textfile"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/spf13/pflag"

	"github.com/restic/restic/internal/errors"
)

// ErrNoRepository is used to report if opening a repository failed due
// to a missing backend storage location or config file
var ErrNoRepository = errors.New("repository does not exist")

const Version = "0.18.1-dev (compiled manually)"

// TimeFormat is the format used for all timestamps printed by restic.
const TimeFormat = "2006-01-02 15:04:05"

type BackendWrapper func(r backend.Backend) (backend.Backend, error)

// Options hold all global options for restic.
type Options struct {
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

	Password string
	Term     ui.Terminal

	Backends                              *location.Registry
	BackendTestHook, BackendInnerTestHook BackendWrapper

	// Verbosity is set as follows:
	//  0 means: don't print any messages except errors, this is used when --quiet is specified
	//  1 is the default: print essential messages
	//  2 means: print more messages, report minor things, this is used when --verbose is specified
	//  3 means: print very detailed debug messages, this is used when --verbose=2 is specified
	Verbosity uint

	Options []string

	Extended options.Options

	// packSizeFlag is used to detect if --pack-size was set (CLI overrides env).
	packSizeFlag *pflag.Flag
}

func (opts *Options) AddFlags(f *pflag.FlagSet) {
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
	const packSizeFlag = "pack-size"
	f.UintVar(&opts.PackSize, packSizeFlag, 0, "set target pack `size` in MiB, created pack files may be larger (default: $RESTIC_PACK_SIZE)")
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
	opts.packSizeFlag = f.Lookup(packSizeFlag)

	if os.Getenv("RESTIC_HTTP_USER_AGENT") != "" {
		opts.HTTPUserAgent = os.Getenv("RESTIC_HTTP_USER_AGENT")
	}
}

func (opts *Options) PreRun(needsPassword bool) error {
	if envVal := os.Getenv("RESTIC_PACK_SIZE"); envVal != "" && !opts.packSizeFlag.Changed {
		targetPackSize, err := strconv.ParseUint(envVal, 10, 32)
		if err != nil {
			// Failing fast here keeps backups from running for a long time with the wrong pack size.
			return errors.Fatalf("invalid value for RESTIC_PACK_SIZE %q: %v", envVal, err)
		}
		opts.PackSize = uint(targetPackSize)
	}

	// set verbosity, default is one
	opts.Verbosity = 1
	if opts.Quiet && opts.Verbose > 0 {
		return errors.Fatal("--quiet and --verbose cannot be specified at the same time")
	}

	switch {
	case opts.Verbose >= 2:
		opts.Verbosity = 3
	case opts.Verbose > 0:
		opts.Verbosity = 2
	case opts.Quiet:
		opts.Verbosity = 0
	}

	// parse extended options
	extendedOpts, err := options.Parse(opts.Options)
	if err != nil {
		return err
	}
	opts.Extended = extendedOpts
	if !needsPassword {
		return nil
	}
	pwd, err := resolvePassword(opts, "RESTIC_PASSWORD")
	if err != nil {
		return errors.Fatalf("Resolving password failed: %v", err)
	}
	opts.Password = pwd
	return nil
}

// resolvePassword determines the password to be used for opening the repository.
func resolvePassword(opts *Options, envStr string) (string, error) {
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
		return LoadPasswordFromFile(opts.PasswordFile)
	}

	if pwd := os.Getenv(envStr); pwd != "" {
		return pwd, nil
	}

	return "", nil
}

// LoadPasswordFromFile loads a password from a file while stripping a BOM and
// converting the password to UTF-8.
func LoadPasswordFromFile(pwdFile string) (string, error) {
	s, err := textfile.Read(pwdFile)
	if errors.Is(err, os.ErrNotExist) {
		return "", errors.Fatalf("%s does not exist", pwdFile)
	}
	return strings.TrimSpace(string(s)), errors.Wrap(err, "Readfile")
}

// readPassword reads the password from a password file, the environment
// variable RESTIC_PASSWORD or prompts the user. If the context is canceled,
// the function leaks the password reading goroutine.
func readPassword(ctx context.Context, gopts Options, prompt string) (string, error) {
	if gopts.InsecureNoPassword {
		if gopts.Password != "" {
			return "", errors.Fatal("--insecure-no-password must not be specified together with providing a password via a cli option or environment variable")
		}
		return "", nil
	}

	if gopts.Password != "" {
		return gopts.Password, nil
	}

	password, err := gopts.Term.ReadPassword(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("unable to read password: %w", err)
	}

	if len(password) == 0 {
		return "", errors.Fatal("an empty password is not allowed by default. Pass the flag `--insecure-no-password` to restic to disable this check")
	}

	return password, nil
}

// ReadPasswordTwice calls ReadPassword two times and returns an error when the
// passwords don't match. If the context is canceled, the function leaks the
// password reading goroutine.
func ReadPasswordTwice(ctx context.Context, gopts Options, prompt1, prompt2 string) (string, error) {
	pw1, err := readPassword(ctx, gopts, prompt1)
	if err != nil {
		return "", err
	}
	if gopts.Term.InputIsTerminal() {
		pw2, err := readPassword(ctx, gopts, prompt2)
		if err != nil {
			return "", err
		}

		if pw1 != pw2 {
			return "", errors.Fatal("passwords do not match")
		}
	}

	return pw1, nil
}

func readRepo(gopts Options) (string, error) {
	if gopts.Repo == "" && gopts.RepositoryFile == "" {
		return "", errors.Fatal("Please specify repository location (-r or --repository-file)")
	}

	repo := gopts.Repo
	if gopts.RepositoryFile != "" {
		if repo != "" {
			return "", errors.Fatal("Options -r and --repository-file are mutually exclusive, please specify only one")
		}

		s, err := textfile.Read(gopts.RepositoryFile)
		if errors.Is(err, os.ErrNotExist) {
			return "", errors.Fatalf("%s does not exist", gopts.RepositoryFile)
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
func OpenRepository(ctx context.Context, gopts Options, printer progress.Printer) (*repository.Repository, error) {
	repo, err := readRepo(gopts)
	if err != nil {
		return nil, err
	}

	be, err := innerOpenBackend(ctx, repo, gopts, gopts.Extended, false, printer)
	if err != nil {
		return nil, err
	}

	err = hasRepositoryConfig(ctx, be, repo, gopts)
	if err != nil {
		return nil, err
	}

	s, err := createRepositoryInstance(be, gopts)
	if err != nil {
		return nil, err
	}

	err = decryptRepository(ctx, s, &gopts, printer)
	if err != nil {
		return nil, err
	}

	printRepositoryInfo(s, gopts, printer)

	if gopts.NoCache {
		return s, nil
	}

	err = setupCache(s, gopts, printer)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// hasRepositoryConfig checks if the repository config file exists and is not empty.
func hasRepositoryConfig(ctx context.Context, be backend.Backend, repo string, gopts Options) error {
	fi, err := be.Stat(ctx, backend.Handle{Type: restic.ConfigFile})
	if be.IsNotExist(err) {
		//nolint:staticcheck // capitalized error string is intentional
		return fmt.Errorf("Fatal: %w: unable to open config file: %v\nIs there a repository at the following location?\n%v", ErrNoRepository, err, location.StripPassword(gopts.Backends, repo))
	}
	if err != nil {
		return errors.Fatalf("unable to open config file: %v\n%v", err, location.StripPassword(gopts.Backends, repo))
	}

	if fi.Size == 0 {
		return errors.New("config file has zero size, invalid repository?")
	}

	return nil
}

// createRepositoryInstance creates a new repository instance with the given options.
func createRepositoryInstance(be backend.Backend, gopts Options) (*repository.Repository, error) {
	s, err := repository.New(be, repository.Options{
		Compression:   gopts.Compression,
		PackSize:      gopts.PackSize * 1024 * 1024,
		NoExtraVerify: gopts.NoExtraVerify,
	})
	if err != nil {
		return nil, errors.Fatalf("%s", err)
	}
	return s, nil
}

// decryptRepository handles password reading and decrypts the repository.
func decryptRepository(ctx context.Context, s *repository.Repository, gopts *Options, printer progress.Printer) error {
	passwordTriesLeft := 1
	if gopts.Term.InputIsTerminal() && gopts.Password == "" && !gopts.InsecureNoPassword {
		passwordTriesLeft = 3
	}

	var err error
	for ; passwordTriesLeft > 0; passwordTriesLeft-- {
		gopts.Password, err = readPassword(ctx, *gopts, "enter password for repository: ")
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil && passwordTriesLeft > 1 {
			gopts.Password = ""
			printer.E("%s. Try again", err)
		}
		if err != nil {
			continue
		}

		err = s.SearchKey(ctx, gopts.Password, maxKeys, gopts.KeyHint)
		if err != nil && passwordTriesLeft > 1 {
			gopts.Password = ""
			printer.E("%s. Try again", err)
		}
	}
	if err != nil {
		if errors.IsFatal(err) || errors.Is(err, repository.ErrNoKeyFound) {
			return err
		}
		return errors.Fatalf("%s", err)
	}

	return nil
}

// printRepositoryInfo displays the repository ID, version and compression level.
func printRepositoryInfo(s *repository.Repository, gopts Options, printer progress.Printer) {
	id := s.Config().ID
	if len(id) > 8 {
		id = id[:8]
	}
	extra := ""
	if s.Config().Version >= 2 {
		extra = ", compression level " + gopts.Compression.String()
	}
	printer.PT("repository %v opened (version %v%s)", id, s.Config().Version, extra)
}

// setupCache creates a new cache and removes old cache directories if instructed to do so.
func setupCache(s *repository.Repository, gopts Options, printer progress.Printer) error {
	c, err := cache.New(s.Config().ID, gopts.CacheDir)
	if err != nil {
		printer.E("unable to open cache: %v", err)
		return err
	}

	if c.Created {
		printer.PT("created new cache in %v", c.Base)
	}

	// start using the cache
	s.UseCache(c, printer.E)

	oldCacheDirs, err := cache.Old(c.Base)
	if err != nil {
		printer.E("unable to find old cache directories: %v", err)
	}

	// nothing more to do if no old cache dirs could be found
	if len(oldCacheDirs) == 0 {
		return nil
	}

	// cleanup old cache dirs if instructed to do so
	if gopts.CleanupCache {
		printer.PT("removing %d old cache dirs from %v", len(oldCacheDirs), c.Base)
		for _, item := range oldCacheDirs {
			dir := filepath.Join(c.Base, item.Name())
			err = os.RemoveAll(dir)
			if err != nil {
				printer.E("unable to remove %v: %v", dir, err)
			}
		}
	} else {
		printer.PT("found %d old cache directories in %v, run `restic cache --cleanup` to remove them",
			len(oldCacheDirs), c.Base)
	}
	return nil
}

// CreateRepository a repository with the given version and chunker polynomial.
func CreateRepository(ctx context.Context, gopts Options, version uint, chunkerPolynomial *chunker.Pol, printer progress.Printer) (*repository.Repository, error) {
	if version < restic.MinRepoVersion || version > restic.MaxRepoVersion {
		return nil, errors.Fatalf("only repository versions between %v and %v are allowed", restic.MinRepoVersion, restic.MaxRepoVersion)
	}

	repo, err := readRepo(gopts)
	if err != nil {
		return nil, err
	}

	gopts.Password, err = ReadPasswordTwice(ctx, gopts,
		"enter password for new repository: ",
		"enter password again: ")
	if err != nil {
		return nil, err
	}

	be, err := innerOpenBackend(ctx, repo, gopts, gopts.Extended, true, printer)
	if err != nil {
		return nil, errors.Fatalf("create repository at %s failed: %v", location.StripPassword(gopts.Backends, repo), err)
	}

	s, err := createRepositoryInstance(be, gopts)
	if err != nil {
		return nil, err
	}

	err = s.Init(ctx, version, gopts.Password, chunkerPolynomial)
	if err != nil {
		return nil, errors.Fatalf("create key in repository at %s failed: %v", location.StripPassword(gopts.Backends, repo), err)
	}

	return s, nil
}

func innerOpenBackend(ctx context.Context, s string, gopts Options, opts options.Options, create bool, printer progress.Printer) (backend.Backend, error) {
	debug.Log("parsing location %v", location.StripPassword(gopts.Backends, s))

	scheme, cfg, err := parseConfig(gopts.Backends, s, opts)
	if err != nil {
		return nil, err
	}

	rt, lim, err := setupTransport(gopts)
	if err != nil {
		return nil, err
	}

	be, err := createOrOpenBackend(ctx, scheme, cfg, rt, lim, gopts, s, create, printer)
	if err != nil {
		return nil, err
	}

	be, err = wrapBackend(be, gopts, printer)
	if err != nil {
		return nil, err
	}

	return be, nil
}

// parseConfig parses the repository location and extended options and returns the scheme and configuration.
func parseConfig(backends *location.Registry, s string, opts options.Options) (string, interface{}, error) {
	loc, err := location.Parse(backends, s)
	if err != nil {
		return "", nil, errors.Fatalf("parsing repository location failed: %v", err)
	}

	cfg := loc.Config
	if cfg, ok := cfg.(backend.ApplyEnvironmenter); ok {
		cfg.ApplyEnvironment("")
	}

	// only apply options for a particular backend here
	opts = opts.Extract(loc.Scheme)
	if err := opts.Apply(loc.Scheme, cfg); err != nil {
		return "", nil, err
	}

	debug.Log("opening %v repository at %#v", loc.Scheme, cfg)
	return loc.Scheme, cfg, nil
}

// setupTransport creates and configures the transport with rate limiting.
func setupTransport(gopts Options) (http.RoundTripper, limiter.Limiter, error) {
	rt, err := backend.Transport(gopts.TransportOptions)
	if err != nil {
		return nil, nil, errors.Fatalf("%s", err)
	}

	// wrap the transport so that the throughput via HTTP is limited
	lim := limiter.NewStaticLimiter(gopts.Limits)
	rt = lim.Transport(rt)

	return rt, lim, nil
}

// createOrOpenBackend creates or opens a backend using the appropriate factory method.
func createOrOpenBackend(ctx context.Context, scheme string, cfg interface{}, rt http.RoundTripper, lim limiter.Limiter, gopts Options, s string, create bool, printer progress.Printer) (backend.Backend, error) {
	factory := gopts.Backends.Lookup(scheme)
	if factory == nil {
		return nil, errors.Fatalf("invalid backend: %q", scheme)
	}

	var be backend.Backend
	var err error
	if create {
		be, err = factory.Create(ctx, cfg, rt, lim, printer.E)
	} else {
		be, err = factory.Open(ctx, cfg, rt, lim, printer.E)
	}

	if errors.Is(err, backend.ErrNoRepository) {
		//nolint:staticcheck // capitalized error string is intentional
		return nil, fmt.Errorf("Fatal: %w at %v: %v", ErrNoRepository, location.StripPassword(gopts.Backends, s), err)
	}
	if err != nil {
		if create {
			// init already wraps the error message
			return nil, err
		}
		return nil, errors.Fatalf("unable to open repository at %v: %v", location.StripPassword(gopts.Backends, s), err)
	}

	return be, nil
}

// wrapBackend applies debug logging, test hooks, and retry wrapper to the backend.
func wrapBackend(be backend.Backend, gopts Options, printer progress.Printer) (backend.Backend, error) {
	// wrap with debug logging and connection limiting
	be = logger.New(sema.NewBackend(be))

	// wrap backend if a test specified an inner hook
	if gopts.BackendInnerTestHook != nil {
		var err error
		be, err = gopts.BackendInnerTestHook(be)
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
	if gopts.BackendTestHook != nil {
		var err error
		be, err = gopts.BackendTestHook(be)
		if err != nil {
			return nil, err
		}
	}

	return be, nil
}
