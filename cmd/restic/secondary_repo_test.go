package main

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

// TestFillSecondaryGlobalOpts tests valid and invalid data on fillSecondaryGlobalOpts-function
func TestFillSecondaryGlobalOpts(t *testing.T) {
	//secondaryRepoTestCase defines a struct for test cases
	type secondaryRepoTestCase struct {
		Opts     secondaryRepoOptions
		DstGOpts GlobalOptions
		FromRepo bool
	}

	//validSecondaryRepoTestCases is a list with test cases that must pass
	var validSecondaryRepoTestCases = []secondaryRepoTestCase{
		{
			// Test if Repo and Password are parsed correctly.
			Opts: secondaryRepoOptions{
				Repo:     "backupDst",
				password: "secretDst",
			},
			DstGOpts: GlobalOptions{
				Repo:     "backupDst",
				password: "secretDst",
			},
			FromRepo: true,
		},
		{
			// Test if RepositoryFile and PasswordFile are parsed correctly.
			Opts: secondaryRepoOptions{
				RepositoryFile: "backupDst",
				PasswordFile:   "passwordFileDst",
			},
			DstGOpts: GlobalOptions{
				RepositoryFile: "backupDst",
				password:       "secretDst",
				PasswordFile:   "passwordFileDst",
			},
			FromRepo: true,
		},
		{
			// Test if RepositoryFile and PasswordCommand are parsed correctly.
			Opts: secondaryRepoOptions{
				RepositoryFile:  "backupDst",
				PasswordCommand: "echo secretDst",
			},
			DstGOpts: GlobalOptions{
				RepositoryFile:  "backupDst",
				password:        "secretDst",
				PasswordCommand: "echo secretDst",
			},
			FromRepo: true,
		},
		{
			// Test if LegacyRepo and Password are parsed correctly.
			Opts: secondaryRepoOptions{
				LegacyRepo: "backupDst",
				password:   "secretDst",
			},
			DstGOpts: GlobalOptions{
				Repo:     "backupDst",
				password: "secretDst",
			},
		},
		{
			// Test if LegacyRepositoryFile and LegacyPasswordFile are parsed correctly.
			Opts: secondaryRepoOptions{
				LegacyRepositoryFile: "backupDst",
				LegacyPasswordFile:   "passwordFileDst",
			},
			DstGOpts: GlobalOptions{
				RepositoryFile: "backupDst",
				password:       "secretDst",
				PasswordFile:   "passwordFileDst",
			},
		},
		{
			// Test if LegacyRepositoryFile and LegacyPasswordCommand are parsed correctly.
			Opts: secondaryRepoOptions{
				LegacyRepositoryFile:  "backupDst",
				LegacyPasswordCommand: "echo secretDst",
			},
			DstGOpts: GlobalOptions{
				RepositoryFile:  "backupDst",
				password:        "secretDst",
				PasswordCommand: "echo secretDst",
			},
		},
	}

	//invalidSecondaryRepoTestCases is a list with test cases that must fail
	var invalidSecondaryRepoTestCases = []secondaryRepoTestCase{
		{
			// Test must fail on no repo given.
			Opts: secondaryRepoOptions{},
		},
		{
			// Test must fail as Repo and RepositoryFile are both given
			Opts: secondaryRepoOptions{
				Repo:           "backupDst",
				RepositoryFile: "backupDst",
			},
		},
		{
			// Test must fail as PasswordFile and PasswordCommand are both given
			Opts: secondaryRepoOptions{
				Repo:            "backupDst",
				PasswordFile:    "passwordFileDst",
				PasswordCommand: "notEmpty",
			},
		},
		{
			// Test must fail as PasswordFile does not exist
			Opts: secondaryRepoOptions{
				Repo:         "backupDst",
				PasswordFile: "NonExistingFile",
			},
		},
		{
			// Test must fail as PasswordCommand does not exist
			Opts: secondaryRepoOptions{
				Repo:            "backupDst",
				PasswordCommand: "notEmpty",
			},
		},
		{
			// Test must fail as no password is given.
			Opts: secondaryRepoOptions{
				Repo: "backupDst",
			},
		},
		{
			// Test must fail as current and legacy options are mixed
			Opts: secondaryRepoOptions{
				Repo:       "backupDst",
				LegacyRepo: "backupDst",
			},
		},
		{
			// Test must fail as current and legacy options are mixed
			Opts: secondaryRepoOptions{
				Repo:                  "backupDst",
				LegacyPasswordCommand: "notEmpty",
			},
		},
	}

	//gOpts defines the Global options used in the secondary repository tests
	var gOpts = GlobalOptions{
		Repo:           "backupSrc",
		RepositoryFile: "backupSrc",
		password:       "secretSrc",
		PasswordFile:   "passwordFileSrc",
	}

	//Create temp dir to create password file.
	dir, cleanup := rtest.TempDir(t)
	defer cleanup()

	cleanup = rtest.Chdir(t, dir)
	defer cleanup()

	//Create temporary password file
	err := ioutil.WriteFile(filepath.Join(dir, "passwordFileDst"), []byte("secretDst"), 0666)
	rtest.OK(t, err)

	// Test all valid cases
	for _, testCase := range validSecondaryRepoTestCases {
		DstGOpts, isFromRepo, err := fillSecondaryGlobalOpts(testCase.Opts, gOpts, "destination")
		rtest.OK(t, err)
		rtest.Equals(t, DstGOpts, testCase.DstGOpts)
		rtest.Equals(t, isFromRepo, testCase.FromRepo)
	}

	// Test all invalid cases
	for _, testCase := range invalidSecondaryRepoTestCases {
		_, _, err := fillSecondaryGlobalOpts(testCase.Opts, gOpts, "destination")
		rtest.Assert(t, err != nil, "Expected error, but function did not return an error")
	}
}
