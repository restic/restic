package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"sort"
	"strings"

	"code.google.com/p/go.crypto/ssh/terminal"

	"github.com/fd0/khepri"
	"github.com/fd0/khepri/backend"
	"github.com/jessevdk/go-flags"
)

var Opts struct {
	Repo string `short:"r" long:"repo"    description:"Repository directory to backup to/restore from"`
}

func errx(code int, format string, data ...interface{}) {
	if len(format) > 0 && format[len(format)-1] != '\n' {
		format += "\n"
	}
	fmt.Fprintf(os.Stderr, format, data...)
	os.Exit(code)
}

type commandFunc func(backend.Server, *khepri.Key, []string) error

var commands map[string]commandFunc

func read_password(prompt string) string {
	p := os.Getenv("KHEPRI_PASSWORD")
	if p != "" {
		return p
	}

	fmt.Print(prompt)
	pw, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		errx(2, "unable to read password: %v", err)
	}
	fmt.Println()

	return string(pw)
}

func commandInit(repo string) error {
	pw := read_password("enter password for new backend: ")
	pw2 := read_password("enter password again: ")

	if pw != pw2 {
		errx(1, "passwords do not match")
	}

	be, err := create(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating backend at %s failed: %v\n", repo, err)
		os.Exit(1)
	}

	_, err = khepri.CreateKey(be, pw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating key in backend at %s failed: %v\n", repo, err)
		os.Exit(1)
	}

	fmt.Printf("created khepri backend at %s\n", be.Location())

	return nil
}

// Open the backend specified by URI.
// Valid formats are:
// * /foo/bar -> local repository at /foo/bar
// * sftp://user@host/foo/bar -> remote sftp repository on host for user at path foo/bar
// * sftp://host//tmp/backup -> remote sftp repository on host at path /tmp/backup
func open(u string) (backend.Server, error) {
	url, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	if url.Scheme == "" {
		return backend.OpenLocal(url.Path)
	} else {
		args := []string{url.Host}
		if url.User != nil && url.User.Username() != "" {
			args = append(args, "-l")
			args = append(args, url.User.Username())
		}
		args = append(args, "-s")
		args = append(args, "sftp")
		return backend.OpenSFTP(url.Path[1:], "ssh", args...)
	}
}

// Create the backend specified by URI.
func create(u string) (backend.Server, error) {
	url, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	if url.Scheme == "" {
		return backend.CreateLocal(url.Path)
	} else {
		args := []string{url.Host}
		if url.User != nil && url.User.Username() != "" {
			args = append(args, "-l")
			args = append(args, url.User.Username())
		}
		args = append(args, "-s")
		args = append(args, "sftp")
		return backend.CreateSFTP(url.Path[1:], "ssh", args...)
	}
}

func init() {
	commands = make(map[string]commandFunc)
	commands["backup"] = commandBackup
	commands["restore"] = commandRestore
	commands["list"] = commandList
	commands["snapshots"] = commandSnapshots
}

func main() {
	log.SetOutput(os.Stdout)

	Opts.Repo = os.Getenv("KHEPRI_REPOSITORY")
	if Opts.Repo == "" {
		Opts.Repo = "khepri-backup"
	}

	args, err := flags.Parse(&Opts)
	if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
		os.Exit(0)
	}

	if len(args) == 0 {
		cmds := []string{"init"}
		for k := range commands {
			cmds = append(cmds, k)
		}
		sort.Strings(cmds)
		fmt.Printf("nothing to do, available commands: [%v]\n", strings.Join(cmds, "|"))
		os.Exit(0)
	}

	cmd := args[0]

	if cmd == "init" {
		err = commandInit(Opts.Repo)
		if err != nil {
			errx(1, "error executing command %q: %v", cmd, err)
		}

		return
	}

	f, ok := commands[cmd]
	if !ok {
		errx(1, "unknown command: %q\n", cmd)
	}

	// read_password("enter password: ")
	repo, err := open(Opts.Repo)
	if err != nil {
		errx(1, "unable to open repo: %v", err)
	}

	key, err := khepri.SearchKey(repo, read_password("Enter Password for Repository: "))
	if err != nil {
		errx(2, "unable to open repo: %v", err)
	}

	err = f(repo, key, args[1:])
	if err != nil {
		errx(1, "error executing command %q: %v", cmd, err)
	}
}
