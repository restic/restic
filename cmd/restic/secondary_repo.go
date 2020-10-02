package main

import (
	"os"

	"github.com/restic/restic/internal/errors"
	"github.com/spf13/pflag"
)

type secondaryRepoOptions struct {
	Repo            string
	password        string
	PasswordFile    string
	PasswordCommand string
	KeyHint         string
}

func initSecondaryRepoOptions(f *pflag.FlagSet, opts *secondaryRepoOptions, repoPrefix string, repoUsage string) {
	f.StringVarP(&opts.Repo, "repo2", "", os.Getenv("RESTIC_REPOSITORY2"), repoPrefix+" `repository` "+repoUsage+" (default: $RESTIC_REPOSITORY2)")
	f.StringVarP(&opts.PasswordFile, "password-file2", "", os.Getenv("RESTIC_PASSWORD_FILE2"), "`file` to read the "+repoPrefix+" repository password from (default: $RESTIC_PASSWORD_FILE2)")
	f.StringVarP(&opts.KeyHint, "key-hint2", "", os.Getenv("RESTIC_KEY_HINT2"), "key ID of key to try decrypting the "+repoPrefix+" repository first (default: $RESTIC_KEY_HINT2)")
	f.StringVarP(&opts.PasswordCommand, "password-command2", "", os.Getenv("RESTIC_PASSWORD_COMMAND2"), "shell `command` to obtain the "+repoPrefix+" repository password from (default: $RESTIC_PASSWORD_COMMAND2)")
}

func fillSecondaryGlobalOpts(opts secondaryRepoOptions, gopts GlobalOptions, repoPrefix string) (GlobalOptions, error) {
	if opts.Repo == "" {
		return GlobalOptions{}, errors.Fatal("Please specify a " + repoPrefix + " repository location (--repo2)")
	}
	var err error
	dstGopts := gopts
	dstGopts.Repo = opts.Repo
	dstGopts.PasswordFile = opts.PasswordFile
	dstGopts.PasswordCommand = opts.PasswordCommand
	dstGopts.KeyHint = opts.KeyHint
	if opts.password != "" {
		dstGopts.password = opts.password
	} else {
		dstGopts.password, err = resolvePassword(dstGopts, "RESTIC_PASSWORD2")
		if err != nil {
			return GlobalOptions{}, err
		}
	}
	dstGopts.password, err = ReadPassword(dstGopts, "enter password for "+repoPrefix+" repository: ")
	if err != nil {
		return GlobalOptions{}, err
	}
	return dstGopts, nil
}
