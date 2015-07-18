package main

import (
	"fmt"
	"net/http"
	"strconv"
)

type CmdWeb struct {
	global *GlobalOptions
}

func init() {
	_, err := parser.AddCommand("web",
		"serve repository in a web interface",
		"The web command serves the repository in a web interface",
		&CmdWeb{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdWeb) Usage() string {
	return "PORT"
}

func (cmd CmdWeb) Execute(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("port not specified, usage:%s", cmd.Usage())
	}

	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}

	portStr := args[0]
	if port, err := strconv.Atoi(portStr); err != nil || port > 65535 {
		return fmt.Errorf("%s is not a valid port number", port)
	}

	http.ListenAndServe(":"+portStr, nil)
	return nil
}
