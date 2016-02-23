package main

import (
	"fmt"
	"os"
	"path/filepath"

	"restic"
	"restic/backend"
	"restic/repository"
)

type CmdLs struct {
	Long bool `short:"l" long:"long" description:"Use a long listing format showing size and mode"`

	global *GlobalOptions
}

func init() {
	_, err := parser.AddCommand("ls",
		"list files",
		"The ls command lists all files and directories in a snapshot",
		&CmdLs{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdLs) printNode(prefix string, n *restic.Node) string {
	if !cmd.Long {
		return filepath.Join(prefix, n.Name)
	}

	switch n.Type {
	case "file":
		return fmt.Sprintf("%s %5d %5d %6d %s %s",
			n.Mode, n.UID, n.GID, n.Size, n.ModTime, filepath.Join(prefix, n.Name))
	case "dir":
		return fmt.Sprintf("%s %5d %5d %6d %s %s",
			n.Mode|os.ModeDir, n.UID, n.GID, n.Size, n.ModTime, filepath.Join(prefix, n.Name))
	case "symlink":
		return fmt.Sprintf("%s %5d %5d %6d %s %s -> %s",
			n.Mode|os.ModeSymlink, n.UID, n.GID, n.Size, n.ModTime, filepath.Join(prefix, n.Name), n.LinkTarget)
	default:
		return fmt.Sprintf("<Node(%s) %s>", n.Type, n.Name)
	}
}

func (cmd CmdLs) printTree(prefix string, repo *repository.Repository, id backend.ID) error {
	tree, err := restic.LoadTree(repo, id)
	if err != nil {
		return err
	}

	for _, entry := range tree.Nodes {
		cmd.global.Printf(cmd.printNode(prefix, entry) + "\n")

		if entry.Type == "dir" && entry.Subtree != nil {
			err = cmd.printTree(filepath.Join(prefix, entry.Name), repo, *entry.Subtree)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (cmd CmdLs) Usage() string {
	return "snapshot-ID [DIR]"
}

func (cmd CmdLs) Execute(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("wrong number of arguments, Usage: %s", cmd.Usage())
	}

	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}

	err = repo.LoadIndex()
	if err != nil {
		return err
	}

	id, err := restic.FindSnapshot(repo, args[0])
	if err != nil {
		return err
	}

	sn, err := restic.LoadSnapshot(repo, id)
	if err != nil {
		return err
	}

	cmd.global.Verbosef("snapshot of %v at %s:\n", sn.Paths, sn.Time)

	return cmd.printTree("", repo, *sn.Tree)
}
