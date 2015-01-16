package main

import (
	"fmt"
	"runtime"

	"github.com/restic/restic"
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
	fmt.Printf("restic version %s, lib %v on %v\n", version, restic.Version, runtime.Version())
	for _, s := range features {
		fmt.Printf("  %s\n", s)
	}
	for _, s := range restic.Features {
		fmt.Printf("  %s\n", s)
	}

	return nil
}
