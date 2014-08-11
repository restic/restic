package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"syscall"

	"github.com/fd0/khepri"
)

func restore_file(repo *khepri.Repository, node *khepri.Node, path string) (err error) {
	switch node.Type {
	case "file":
		// TODO: handle hard links
		rd, err := repo.Get(khepri.TYPE_BLOB, node.Content)
		if err != nil {
			return err
		}

		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
		defer f.Close()
		if err != nil {
			return err
		}

		_, err = io.Copy(f, rd)
		if err != nil {
			return err
		}

	case "symlink":
		err = os.Symlink(node.LinkTarget, path)
		if err != nil {
			return err
		}

		err = os.Lchown(path, int(node.UID), int(node.GID))
		if err != nil {
			return err
		}

		f, err := os.OpenFile(path, khepri.O_PATH|syscall.O_NOFOLLOW, 0600)
		defer f.Close()
		if err != nil {
			return err
		}

		var utimes = []syscall.Timeval{
			syscall.NsecToTimeval(node.AccessTime.UnixNano()),
			syscall.NsecToTimeval(node.ModTime.UnixNano()),
		}
		err = syscall.Futimes(int(f.Fd()), utimes)
		if err != nil {
			return err
		}

		return nil
	case "dev":
		err = syscall.Mknod(path, syscall.S_IFBLK|0600, int(node.Device))
		if err != nil {
			return err
		}
	case "chardev":
		err = syscall.Mknod(path, syscall.S_IFCHR|0600, int(node.Device))
		if err != nil {
			return err
		}
	case "fifo":
		err = syscall.Mkfifo(path, 0600)
		if err != nil {
			return err
		}
	case "socket":
		// nothing to do, we do not restore sockets
	default:
		return fmt.Errorf("filetype %q not implemented!\n", node.Type)
	}

	err = os.Chmod(path, node.Mode)
	if err != nil {
		return err
	}

	err = os.Chown(path, int(node.UID), int(node.GID))
	if err != nil {
		return err
	}

	err = os.Chtimes(path, node.AccessTime, node.ModTime)
	if err != nil {
		return err
	}

	return nil
}

func restore_subtree(repo *khepri.Repository, tree *khepri.Tree, path string) {
	fmt.Printf("restore_subtree(%s)\n", path)

	for _, node := range tree.Nodes {
		nodepath := filepath.Join(path, node.Name)
		// fmt.Printf("%s:%s\n", node.Type, nodepath)

		if node.Type == "dir" {
			err := os.Mkdir(nodepath, 0700)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				continue
			}

			err = os.Chmod(nodepath, node.Mode)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				continue
			}

			err = os.Chown(nodepath, int(node.UID), int(node.GID))
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				continue
			}

			restore_subtree(repo, node.Tree, filepath.Join(path, node.Name))

			err = os.Chtimes(nodepath, node.AccessTime, node.ModTime)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				continue
			}

		} else {
			err := restore_file(repo, node, nodepath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				continue
			}
		}
	}
}

func commandRestore(repo *khepri.Repository, args []string) error {
	if len(args) != 2 {
		return errors.New("usage: restore ID dir")
	}

	id, err := khepri.ParseID(args[0])
	if err != nil {
		errx(1, "invalid id %q: %v", args[0], err)
	}

	target := args[1]

	err = os.MkdirAll(target, 0700)
	if err != nil {
		return err
	}

	sn, err := khepri.LoadSnapshot(repo, id)
	if err != nil {
		log.Fatalf("error loading snapshot %s", id)
	}

	tree, err := khepri.NewTreeFromRepo(repo, sn.Content)
	if err != nil {
		log.Fatalf("error loading tree %s", sn.Content)
	}

	restore_subtree(repo, tree, target)

	log.Printf("%q restored to %q\n", id, target)

	return nil
}
