// +build debug

package restic

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

var debugLogger = initDebugLogger()
var debugTags = make(map[string]bool)
var debugBreak = make(map[string]bool)

func initDebugLogger() *log.Logger {
	// create new log file
	filename := fmt.Sprintf("restic-lib-debug-%d-%s",
		os.Getpid(), time.Now().Format("20060201-150405"))
	debugfile := filepath.Join(os.TempDir(), filename)
	f, err := os.OpenFile(debugfile, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to create debug log file: %v", err)
		os.Exit(2)
	}

	// open logger
	l := log.New(f, "DEBUG: ", log.LstdFlags)
	fmt.Fprintf(os.Stderr, "debug log for restic library activated, writing log file %s\n", debugfile)

	// defaults
	debugTags["break"] = true

	// initialize tags
	env := os.Getenv("DEBUG_TAGS")
	if len(env) > 0 {
		tags := []string{}

		for _, tag := range strings.Split(env, ",") {
			t := strings.TrimSpace(tag)
			val := true
			if t[0] == '-' {
				val = false
				t = t[1:]
			} else if t[0] == '+' {
				val = true
				t = t[1:]
			}

			// test pattern
			_, err := path.Match(t, "")
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: invalid pattern %q: %v\n", t, err)
				os.Exit(5)
			}

			debugTags[t] = val
			tags = append(tags, tag)
		}

		fmt.Fprintf(os.Stderr, "debug log enabled for: %v\n", tags)
	}

	// initialize break tags
	env = os.Getenv("DEBUG_BREAK")
	if len(env) > 0 {
		breaks := []string{}

		for _, tag := range strings.Split(env, ",") {
			t := strings.TrimSpace(tag)
			debugBreak[t] = true
			breaks = append(breaks, t)
		}

		fmt.Fprintf(os.Stderr, "debug breaks enabled for: %v\n", breaks)
	}

	return l
}

func debug(tag string, f string, args ...interface{}) {
	dbgprint := func() {
		fmt.Fprintf(os.Stderr, tag+": "+f, args...)
	}

	debugLogger.Printf(f, args...)

	// check if tag is enabled directly
	if v, ok := debugTags[tag]; ok {
		if v {
			dbgprint()
		}
		return
	}

	// check for globbing
	for k, v := range debugTags {
		if m, _ := path.Match(k, tag); m {
			if v {
				dbgprint()
			}
			return
		}
	}

	// check if tag "all" is enabled
	if v, ok := debugTags["all"]; ok && v {
		dbgprint()
	}
}

func debug_break(tag string) {
	// check if breaking is enabled
	if v, ok := debugBreak[tag]; !ok || !v {
		return
	}

	_, file, line, _ := runtime.Caller(1)
	debug("break", "stopping process %d at %s (%v:%v)\n", os.Getpid(), tag, file, line)
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		panic(err)
	}

	err = p.Signal(syscall.SIGSTOP)
	if err != nil {
		panic(err)
	}
}
