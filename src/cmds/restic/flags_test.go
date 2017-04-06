package main

import (
	"testing"
)

// TestFlags checks for double defined flags, the commands will panic on
// ParseFlags() when a shorthand flag is defined twice.
func TestFlags(t *testing.T) {
	type FlagParser interface {
		ParseFlags([]string) error
	}

	for _, cmd := range cmdRoot.Commands() {
		t.Run(cmd.Name(), func(t *testing.T) {
			err := cmd.ParseFlags([]string{"--help"})
			if err.Error() == "pflag: help requested" {
				err = nil
			}

			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
