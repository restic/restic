package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/fd0/khepri"
	"github.com/jessevdk/go-flags"
)

var Opts struct {
	Repo string `short:"r" long:"repo"    description:"Repository directory to backup to/restor from"`
}

func errx(code int, format string, data ...interface{}) {
	if len(format) > 0 && format[len(format)-1] != '\n' {
		format += "\n"
	}
	fmt.Fprintf(os.Stderr, format, data...)
	os.Exit(code)
}

type commandFunc func(*khepri.Repository, []string) error

var commands map[string]commandFunc

func init() {
	commands = make(map[string]commandFunc)
	commands["backup"] = commandBackup
	commands["restore"] = commandRestore
	commands["list"] = commandList
	commands["snapshots"] = commandSnapshots
	commands["fsck"] = commandFsck
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
		cmds := []string{}
		for k := range commands {
			cmds = append(cmds, k)
		}
		sort.Strings(cmds)
		fmt.Printf("nothing to do, available commands: [%v]\n", strings.Join(cmds, "|"))
		os.Exit(0)
	}

	cmd := args[0]

	f, ok := commands[cmd]
	if !ok {
		errx(1, "unknown command: %q\n", cmd)
	}

	repo, err := khepri.NewRepository(Opts.Repo)

	if err != nil {
		errx(1, "unable to create/open repo: %v", err)
	}

	err = f(repo, args[1:])
	if err != nil {
		errx(1, "error executing command %q: %v", cmd, err)
	}
}
