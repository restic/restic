package main

import (
	"os"

	"github.com/restic/restic/internal/errors"
	"github.com/spf13/pflag"
)

type secondaryRepoOptions struct {
	password string
	// from-repo options
	Repo            string
	RepositoryFile  string
	PasswordFile    string
	PasswordCommand string
	KeyHint         string
	// repo2 options
	LegacyRepo            string
	LegacyRepositoryFile  string
	LegacyPasswordFile    string
	LegacyPasswordCommand string
	LegacyKeyHint         string
}

func initSecondaryRepoOptions(f *pflag.FlagSet, opts *secondaryRepoOptions, repoPrefix string, repoUsage string) {
	f.StringVarP(&opts.LegacyRepo, "repo2", "", os.Getenv("RESTIC_REPOSITORY2"), repoPrefix+" `repository` "+repoUsage+" (default: $RESTIC_REPOSITORY2)")
	f.StringVarP(&opts.LegacyRepositoryFile, "repository-file2", "", os.Getenv("RESTIC_REPOSITORY_FILE2"), "`file` from which to read the "+repoPrefix+" repository location "+repoUsage+" (default: $RESTIC_REPOSITORY_FILE2)")
	f.StringVarP(&opts.LegacyPasswordFile, "password-file2", "", os.Getenv("RESTIC_PASSWORD_FILE2"), "`file` to read the "+repoPrefix+" repository password from (default: $RESTIC_PASSWORD_FILE2)")
	f.StringVarP(&opts.LegacyKeyHint, "key-hint2", "", os.Getenv("RESTIC_KEY_HINT2"), "key ID of key to try decrypting the "+repoPrefix+" repository first (default: $RESTIC_KEY_HINT2)")
	f.StringVarP(&opts.LegacyPasswordCommand, "password-command2", "", os.Getenv("RESTIC_PASSWORD_COMMAND2"), "shell `command` to obtain the "+repoPrefix+" repository password from (default: $RESTIC_PASSWORD_COMMAND2)")

	// hide repo2 options
	_ = f.MarkDeprecated("repo2", "use --repo or --from-repo instead")
	_ = f.MarkDeprecated("repository-file2", "use --repository-file or --from-repository-file instead")
	_ = f.MarkHidden("password-file2")
	_ = f.MarkHidden("key-hint2")
	_ = f.MarkHidden("password-command2")

	f.StringVarP(&opts.Repo, "from-repo", "", os.Getenv("RESTIC_FROM_REPOSITORY"), "source `repository` "+repoUsage+" (default: $RESTIC_FROM_REPOSITORY)")
	f.StringVarP(&opts.RepositoryFile, "from-repository-file", "", os.Getenv("RESTIC_FROM_REPOSITORY_FILE"), "`file` from which to read the source repository location "+repoUsage+" (default: $RESTIC_FROM_REPOSITORY_FILE)")
	f.StringVarP(&opts.PasswordFile, "from-password-file", "", os.Getenv("RESTIC_FROM_PASSWORD_FILE"), "`file` to read the source repository password from (default: $RESTIC_FROM_PASSWORD_FILE)")
	f.StringVarP(&opts.KeyHint, "from-key-hint", "", os.Getenv("RESTIC_FROM_KEY_HINT"), "key ID of key to try decrypting the source repository first (default: $RESTIC_FROM_KEY_HINT)")
	f.StringVarP(&opts.PasswordCommand, "from-password-command", "", os.Getenv("RESTIC_FROM_PASSWORD_COMMAND"), "shell `command` to obtain the source repository password from (default: $RESTIC_FROM_PASSWORD_COMMAND)")
}

func fillSecondaryGlobalOpts(opts secondaryRepoOptions, gopts GlobalOptions, repoPrefix string) (GlobalOptions, bool, error) {
	if opts.Repo == "" && opts.RepositoryFile == "" && opts.LegacyRepo == "" && opts.LegacyRepositoryFile == "" {
		return GlobalOptions{}, false, errors.Fatal("Please specify a source repository location (--from-repo or --from-repository-file)")
	}

	hasFromRepo := opts.Repo != "" || opts.RepositoryFile != "" || opts.PasswordFile != "" ||
		opts.KeyHint != "" || opts.PasswordCommand != ""
	hasRepo2 := opts.LegacyRepo != "" || opts.LegacyRepositoryFile != "" || opts.LegacyPasswordFile != "" ||
		opts.LegacyKeyHint != "" || opts.LegacyPasswordCommand != ""

	if hasFromRepo && hasRepo2 {
		return GlobalOptions{}, false, errors.Fatal("Option groups repo2 and from-repo are mutually exclusive, please specify only one")
	}

	var err error
	dstGopts := gopts
	var pwdEnv string

	if hasFromRepo {
		if opts.Repo != "" && opts.RepositoryFile != "" {
			return GlobalOptions{}, false, errors.Fatal("Options --from-repo and --from-repository-file are mutually exclusive, please specify only one")
		}

		dstGopts.Repo = opts.Repo
		dstGopts.RepositoryFile = opts.RepositoryFile
		dstGopts.PasswordFile = opts.PasswordFile
		dstGopts.PasswordCommand = opts.PasswordCommand
		dstGopts.KeyHint = opts.KeyHint

		pwdEnv = "RESTIC_FROM_PASSWORD"
		repoPrefix = "source"
	} else {
		if opts.LegacyRepo != "" && opts.LegacyRepositoryFile != "" {
			return GlobalOptions{}, false, errors.Fatal("Options --repo2 and --repository-file2 are mutually exclusive, please specify only one")
		}

		dstGopts.Repo = opts.LegacyRepo
		dstGopts.RepositoryFile = opts.LegacyRepositoryFile
		dstGopts.PasswordFile = opts.LegacyPasswordFile
		dstGopts.PasswordCommand = opts.LegacyPasswordCommand
		dstGopts.KeyHint = opts.LegacyKeyHint

		pwdEnv = "RESTIC_PASSWORD2"
	}

	if opts.password != "" {
		dstGopts.password = opts.password
	} else {
		dstGopts.password, err = resolvePassword(dstGopts, pwdEnv)
		if err != nil {
			return GlobalOptions{}, false, err
		}
	}
	dstGopts.password, err = ReadPassword(dstGopts, "enter password for "+repoPrefix+" repository: ")
	if err != nil {
		return GlobalOptions{}, false, err
	}
	return dstGopts, hasFromRepo, nil
}
