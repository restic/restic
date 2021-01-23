package main

import (
	"context"
	"os"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"

	"github.com/spf13/cobra"
)

var cmdInit = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new repository",
	Long: `
The "init" command initializes a new repository.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInit(initOptions, globalOptions, args)
	},
}

// InitOptions bundles all options for the init command.
type InitOptions struct {
	secondaryRepoOptions
	CopyChunkerParameters bool
	HotOnly               bool
}

var initOptions InitOptions

func init() {
	cmdRoot.AddCommand(cmdInit)

	f := cmdInit.Flags()
	initSecondaryRepoOptions(f, &initOptions.secondaryRepoOptions, "secondary", "to copy chunker parameters from")
	f.BoolVar(&initOptions.CopyChunkerParameters, "copy-chunker-params", false, "copy chunker parameters from the secondary repository (useful with the copy command)")
	f.BoolVar(&initOptions.HotOnly, "hot-only", false, "initialize hot repo from existing repo (if --repo-hot is given)")
}

func runInit(opts InitOptions, gopts GlobalOptions, args []string) error {
	if opts.HotOnly {
		if gopts.RepoHot == "" {
			return errors.Fatal("need to specify --repo-hot")
		}
		return runInitHotOnly(gopts)
	}

	chunkerPolynomial, err := maybeReadChunkerPolynomial(opts, gopts)
	if err != nil {
		return err
	}

	repo, err := ReadRepo(gopts)
	if err != nil {
		return err
	}

	be, err := create(repo, gopts.extended)
	if err != nil {
		return errors.Fatalf("create repository at %s failed: %v\n", location.StripPassword(gopts.Repo), err)
	}

	var beHot restic.Backend
	if gopts.RepoHot != "" {
		beHot, err = create(gopts.RepoHot, gopts.extended)
		if err != nil {
			return errors.Fatalf("create repository at %s failed: %v\n", location.StripPassword(gopts.RepoHot), err)
		}
		be = backend.NewHotColdBackend(beHot, be)
	}

	gopts.password, err = ReadPasswordTwice(gopts,
		"enter password for new repository: ",
		"enter password again: ")
	if err != nil {
		return err
	}

	s := repository.New(be)

	err = s.Init(gopts.ctx, gopts.password, chunkerPolynomial)
	if err != nil {
		return errors.Fatalf("create key in repository at %s failed: %v\n", location.StripPassword(gopts.Repo), err)
	}

	Verbosef("created restic repository %v at %s\n", s.Config().ID[:10], location.StripPassword(gopts.Repo))
	if gopts.RepoHot != "" {
		sHot := repository.New(beHot)
		sHot.InitFrom(s)
		cfgHot := s.Config()
		cfgHot.IsHot = true
		err := beHot.Remove(gopts.ctx, restic.Handle{Type: restic.ConfigFile})
		if err != nil {
			return errors.Fatalf("modifying config files in hot repository part at %s failed: %v\n", location.StripPassword(gopts.RepoHot), err)
		}
		_, err = sHot.SaveJSONUnpacked(gopts.ctx, restic.ConfigFile, cfgHot)
		if err != nil {
			return errors.Fatalf("initializing hot repository part at %s failed: %v\n", location.StripPassword(gopts.RepoHot), err)
		}
		Verbosef("created restic hot repository part at %s\n", location.StripPassword(gopts.RepoHot))
	}
	Verbosef("\n")
	Verbosef("Please note that knowledge of your password is required to access\n")
	Verbosef("the repository. Losing your password means that your data is\n")
	Verbosef("irrecoverably lost.\n")

	return nil
}

func maybeReadChunkerPolynomial(opts InitOptions, gopts GlobalOptions) (*chunker.Pol, error) {
	if opts.CopyChunkerParameters {
		otherGopts, err := fillSecondaryGlobalOpts(opts.secondaryRepoOptions, gopts, "secondary")
		if err != nil {
			return nil, err
		}

		otherRepo, err := OpenRepository(otherGopts)
		if err != nil {
			return nil, err
		}

		pol := otherRepo.Config().ChunkerPolynomial
		return &pol, nil
	}

	if opts.Repo != "" {
		return nil, errors.Fatal("Secondary repository must only be specified when copying the chunker parameters")
	}
	return nil, nil
}

func runInitHotOnly(gopts GlobalOptions) error {
	ctx := gopts.ctx

	beHot, err := create(gopts.RepoHot, gopts.extended)
	if err != nil {
		return errors.Fatalf("create repository at %s failed: %v\n", location.StripPassword(gopts.RepoHot), err)
	}
	has, err := beHot.Test(ctx, restic.Handle{Type: restic.ConfigFile})
	if err != nil {
		return err
	}
	if has {
		return errors.Fatalf("hot repository at %sis already initialized", location.StripPassword(gopts.RepoHot))
	}

	gopts.RepoHot = ""
	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}
	lock, err := lockRepo(gopts.ctx, repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}
	be := repo.Backend()

	sHot := repository.New(beHot)
	sHot.InitFrom(repo)
	cfgHot := repo.Config()
	cfgHot.IsHot = true
	_, err = sHot.SaveJSONUnpacked(gopts.ctx, restic.ConfigFile, cfgHot)
	if err != nil {
		return errors.Fatalf("initializing hot repository part at %s failed: %v\n", location.StripPassword(gopts.RepoHot), err)
	}
	Verbosef("created restic hot repository part at %s\n", location.StripPassword(gopts.RepoHot))

	// copy tree packs
	Verbosef("load index files\n")
	err = repo.LoadIndex(gopts.ctx)
	if err != nil {
		return err
	}
	treepacks := restic.NewIDSet()
	for pb := range repo.Index().Each(ctx) {
		if pb.Type == restic.TreeBlob {
			treepacks.Insert(pb.PackID)
		}
	}
	Verbosef("copy tree pack files\n")
	for id := range treepacks {
		h := restic.Handle{Type: restic.PackFile, BT: restic.TreeBlob, Name: id.String()}
		err = copyBackendFile(ctx, be, beHot, h)
		if err != nil {
			return err
		}
	}

	// copy index, snapshots and keys
	for _, t := range []restic.FileType{restic.IndexFile, restic.SnapshotFile, restic.KeyFile} {
		Verbosef("copy %v files\n", t)
		err = be.List(ctx, t, func(fi restic.FileInfo) error {
			h := restic.Handle{Name: fi.Name, Type: t}
			return copyBackendFile(ctx, be, beHot, h)
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func copyBackendFile(ctx context.Context, fromBe, toBe restic.Backend, h restic.Handle) error {
	file, id, _, err := repository.DownloadAndHash(ctx, fromBe, h)
	if err != nil {
		return nil
	}
	defer os.Remove(file.Name())
	defer file.Close()

	if h.Name != id.String() {
		return errors.Errorf("hash does not match id: want %v, got %v", h.Name, id.String())
	}

	rd, err := restic.NewFileReader(file)
	if err != nil {
		return err
	}
	return toBe.Save(ctx, h, rd)
}
