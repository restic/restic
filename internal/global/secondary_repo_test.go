package global

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

// TestFillSecondaryGlobalOpts tests valid and invalid data on fillSecondaryGlobalOpts-function
func TestFillSecondaryGlobalOpts(t *testing.T) {
	//secondaryRepoTestCase defines a struct for test cases
	type secondaryRepoTestCase struct {
		Opts     SecondaryRepoOptions
		DstGOpts Options
		FromRepo bool
	}

	//validSecondaryRepoTestCases is a list with test cases that must pass
	var validSecondaryRepoTestCases = []secondaryRepoTestCase{
		{
			// Test if Repo and Password are parsed correctly.
			Opts: SecondaryRepoOptions{
				Repo:     "backupDst",
				Password: "secretDst",
			},
			DstGOpts: Options{
				Repo:     "backupDst",
				Password: "secretDst",
			},
			FromRepo: true,
		},
		{
			// Test if RepositoryFile and PasswordFile are parsed correctly.
			Opts: SecondaryRepoOptions{
				RepositoryFile: "backupDst",
				PasswordFile:   "passwordFileDst",
			},
			DstGOpts: Options{
				RepositoryFile: "backupDst",
				Password:       "secretDst",
				PasswordFile:   "passwordFileDst",
			},
			FromRepo: true,
		},
		{
			// Test if RepositoryFile and PasswordCommand are parsed correctly.
			Opts: SecondaryRepoOptions{
				RepositoryFile:  "backupDst",
				PasswordCommand: "echo secretDst",
			},
			DstGOpts: Options{
				RepositoryFile:  "backupDst",
				Password:        "secretDst",
				PasswordCommand: "echo secretDst",
			},
			FromRepo: true,
		},
		{
			// Test if LegacyRepo and Password are parsed correctly.
			Opts: SecondaryRepoOptions{
				LegacyRepo: "backupDst",
				Password:   "secretDst",
			},
			DstGOpts: Options{
				Repo:     "backupDst",
				Password: "secretDst",
			},
		},
		{
			// Test if LegacyRepositoryFile and LegacyPasswordFile are parsed correctly.
			Opts: SecondaryRepoOptions{
				LegacyRepositoryFile: "backupDst",
				LegacyPasswordFile:   "passwordFileDst",
			},
			DstGOpts: Options{
				RepositoryFile: "backupDst",
				Password:       "secretDst",
				PasswordFile:   "passwordFileDst",
			},
		},
		{
			// Test if LegacyRepositoryFile and LegacyPasswordCommand are parsed correctly.
			Opts: SecondaryRepoOptions{
				LegacyRepositoryFile:  "backupDst",
				LegacyPasswordCommand: "echo secretDst",
			},
			DstGOpts: Options{
				RepositoryFile:  "backupDst",
				Password:        "secretDst",
				PasswordCommand: "echo secretDst",
			},
		},
	}

	//invalidSecondaryRepoTestCases is a list with test cases that must fail
	var invalidSecondaryRepoTestCases = []secondaryRepoTestCase{
		{
			// Test must fail on no repo given.
			Opts: SecondaryRepoOptions{},
		},
		{
			// Test must fail as Repo and RepositoryFile are both given
			Opts: SecondaryRepoOptions{
				Repo:           "backupDst",
				RepositoryFile: "backupDst",
			},
		},
		{
			// Test must fail as PasswordFile and PasswordCommand are both given
			Opts: SecondaryRepoOptions{
				Repo:            "backupDst",
				PasswordFile:    "passwordFileDst",
				PasswordCommand: "notEmpty",
			},
		},
		{
			// Test must fail as PasswordFile does not exist
			Opts: SecondaryRepoOptions{
				Repo:         "backupDst",
				PasswordFile: "NonExistingFile",
			},
		},
		{
			// Test must fail as PasswordCommand does not exist
			Opts: SecondaryRepoOptions{
				Repo:            "backupDst",
				PasswordCommand: "notEmpty",
			},
		},
		{
			// Test must fail as current and legacy options are mixed
			Opts: SecondaryRepoOptions{
				Repo:       "backupDst",
				LegacyRepo: "backupDst",
			},
		},
		{
			// Test must fail as current and legacy options are mixed
			Opts: SecondaryRepoOptions{
				Repo:                  "backupDst",
				LegacyPasswordCommand: "notEmpty",
			},
		},
	}

	//gOpts defines the Global options used in the secondary repository tests
	var gOpts = Options{
		Repo:           "backupSrc",
		RepositoryFile: "backupSrc",
		Password:       "secretSrc",
		PasswordFile:   "passwordFileSrc",
	}

	//Create temp dir to create password file.
	dir := rtest.TempDir(t)
	cleanup := rtest.Chdir(t, dir)
	defer cleanup()

	//Create temporary password file
	err := os.WriteFile(filepath.Join(dir, "passwordFileDst"), []byte("secretDst"), 0666)
	rtest.OK(t, err)

	// Test all valid cases
	for _, testCase := range validSecondaryRepoTestCases {
		DstGOpts, isFromRepo, err := testCase.Opts.FillGlobalOpts(context.TODO(), gOpts, "destination")
		rtest.OK(t, err)
		rtest.Equals(t, DstGOpts, testCase.DstGOpts)
		rtest.Equals(t, isFromRepo, testCase.FromRepo)
	}

	// Test all invalid cases
	for _, testCase := range invalidSecondaryRepoTestCases {
		_, _, err := testCase.Opts.FillGlobalOpts(context.TODO(), gOpts, "destination")
		rtest.Assert(t, err != nil, "Expected error, but function did not return an error")
	}
}
