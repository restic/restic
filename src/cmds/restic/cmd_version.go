package main

import (
	"fmt"
	"runtime"
)

type CmdVersion struct{}

func init() {
	_, err := parser.AddCommand("version",
		"display version",
		"The version command displays detailed information about the version",
		&CmdVersion{})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdVersion) Execute(args []string) error {
	fmt.Printf("restic %s\ncompiled at %s with %v\n",
		version, compiledAt, runtime.Version())

	return nil
}
