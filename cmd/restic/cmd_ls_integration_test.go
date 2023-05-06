package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func testRunLs(t testing.TB, gopts GlobalOptions, snapshotID string) []string {
	buf := bytes.NewBuffer(nil)
	globalOptions.stdout = buf
	quiet := globalOptions.Quiet
	globalOptions.Quiet = true
	defer func() {
		globalOptions.stdout = os.Stdout
		globalOptions.Quiet = quiet
	}()

	opts := LsOptions{}

	rtest.OK(t, runLs(context.TODO(), opts, gopts, []string{snapshotID}))

	return strings.Split(buf.String(), "\n")
}
