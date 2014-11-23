package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/fd0/khepri"
	"github.com/fd0/khepri/backend"
	"github.com/jessevdk/go-flags"
)

var version = "compiled manually"

var opts struct {
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

func readPassword(prompt string) string {
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
	pw := readPassword("enter password for new backend: ")
	pw2 := readPassword("enter password again: ")

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
	}

	args := []string{url.Host}
	if url.User != nil && url.User.Username() != "" {
		args = append(args, "-l")
		args = append(args, url.User.Username())
	}
	args = append(args, "-s")
	args = append(args, "sftp")
	return backend.OpenSFTP(url.Path[1:], "ssh", args...)
}

// Create the backend specified by URI.
func create(u string) (backend.Server, error) {
	url, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	if url.Scheme == "" {
		return backend.CreateLocal(url.Path)
	}

	args := []string{url.Host}
	if url.User != nil && url.User.Username() != "" {
		args = append(args, "-l")
		args = append(args, url.User.Username())
	}
	args = append(args, "-s")
	args = append(args, "sftp")
	return backend.CreateSFTP(url.Path[1:], "ssh", args...)
}

func init() {
	commands = make(map[string]commandFunc)
	commands["backup"] = commandBackup
	commands["restore"] = commandRestore
	commands["list"] = commandList
	commands["snapshots"] = commandSnapshots
	commands["cat"] = commandCat
	commands["ls"] = commandLs

	// set GOMAXPROCS to number of CPUs
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func main() {
	// defer profile.Start(profile.MemProfileRate(100000), profile.ProfilePath(".")).Stop()

	log.SetOutput(os.Stdout)

	opts.Repo = os.Getenv("KHEPRI_REPOSITORY")

	args, err := flags.Parse(&opts)
	if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
		os.Exit(0)
	}

	if opts.Repo == "" {
		fmt.Fprintf(os.Stderr, "no repository specified, use -r or KHEPRI_REPOSITORY variable\n")
		os.Exit(1)
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

	switch cmd {
	case "init":
		err = commandInit(opts.Repo)
		if err != nil {
			errx(1, "error executing command %q: %v", cmd, err)
		}
		return

	case "version":
		fmt.Printf("%v\n", version)
		return
	}

	f, ok := commands[cmd]
	if !ok {
		errx(1, "unknown command: %q\n", cmd)
	}

	// read_password("enter password: ")
	repo, err := open(opts.Repo)
	if err != nil {
		errx(1, "unable to open repo: %v", err)
	}

	key, err := khepri.SearchKey(repo, readPassword("Enter Password for Repository: "))
	if err != nil {
		errx(2, "unable to open repo: %v", err)
	}

	err = f(repo, key, args[1:])
	if err != nil {
		errx(1, "error executing command %q: %v", cmd, err)
	}

	khepri.PoolAlloc()
}
