package main

import (
	"bytes"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func Test_PrintFunctionsRespectsGlobalStdout(t *testing.T) {
	gopts := globalOptions
	defer func() {
		globalOptions = gopts
	}()

	buf := bytes.NewBuffer(nil)
	globalOptions.stdout = buf

	for _, p := range []func(){
		func() { Println("message") },
		func() { Print("message\n") },
		func() { Printf("mes%s\n", "sage") },
	} {
		p()
		rtest.Equals(t, "message\n", buf.String())
		buf.Reset()
	}
}
