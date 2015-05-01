// +build debug

package debug

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
)

var opts struct {
	logger *log.Logger
	tags   map[string]bool
	breaks map[string]bool
	m      sync.Mutex
}

// make sure that all the initialization happens before the init() functions
// are called, cf https://golang.org/ref/spec#Package_initialization
var _ = initDebug()

func initDebug() bool {
	initDebugLogger()
	initDebugTags()
	initDebugBreaks()

	fmt.Fprintf(os.Stderr, "debug enabled\n")

	return true
}

func initDebugLogger() {
	debugfile := os.Getenv("DEBUG_LOG")
	if debugfile == "" {
		return
	}

	fmt.Fprintf(os.Stderr, "debug log file %v\n", debugfile)

	f, err := os.OpenFile(debugfile, os.O_WRONLY|os.O_APPEND, 0600)

	if err == nil {
		_, err = f.Seek(2, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to seek to the end of %v: %v\n", debugfile, err)
			os.Exit(3)
		}
	}

	if err != nil && os.IsNotExist(err) {
		f, err = os.OpenFile(debugfile, os.O_WRONLY|os.O_CREATE, 0600)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to open debug log file: %v\n", err)
		os.Exit(2)
	}

	opts.logger = log.New(f, "", log.LstdFlags)
}

func initDebugTags() {
	opts.tags = make(map[string]bool)

	// defaults
	opts.tags["break"] = true

	// initialize tags
	env := os.Getenv("DEBUG_TAGS")
	if len(env) == 0 {
		return
	}

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

		opts.tags[t] = val
		tags = append(tags, tag)
	}

	fmt.Fprintf(os.Stderr, "debug log enabled for: %v\n", tags)
}

func initDebugBreaks() {
	opts.breaks = make(map[string]bool)

	env := os.Getenv("DEBUG_BREAK")
	if len(env) == 0 {
		return
	}

	breaks := []string{}

	for _, tag := range strings.Split(env, ",") {
		t := strings.TrimSpace(tag)
		opts.breaks[t] = true
		breaks = append(breaks, t)
	}

	fmt.Fprintf(os.Stderr, "debug breaks enabled for: %v\n", breaks)
}

// taken from https://github.com/VividCortex/trace
func goroutineNum() int {
	b := make([]byte, 20)
	runtime.Stack(b, false)
	var num int

	fmt.Sscanf(string(b), "goroutine %d ", &num)
	return num
}

// taken from https://github.com/VividCortex/trace
func getPosition() string {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		return ""
	}

	goroutine := goroutineNum()

	return fmt.Sprintf("%3d %s:%3d", goroutine, filepath.Base(file), line)
}

func Log(tag string, f string, args ...interface{}) {
	opts.m.Lock()
	defer opts.m.Unlock()

	if f[len(f)-1] != '\n' {
		f += "\n"
	}

	formatString := fmt.Sprintf("[% 25s] %-20s %s", tag, getPosition(), f)

	dbgprint := func() {
		fmt.Fprintf(os.Stderr, formatString, args...)
	}

	if opts.logger != nil {
		opts.logger.Printf(formatString, args...)
	}

	// check if tag is enabled directly
	if v, ok := opts.tags[tag]; ok {
		if v {
			dbgprint()
		}
		return
	}

	// check for globbing
	for k, v := range opts.tags {
		if m, _ := path.Match(k, tag); m {
			if v {
				dbgprint()
			}
			return
		}
	}

	// check if tag "all" is enabled
	if v, ok := opts.tags["all"]; ok && v {
		dbgprint()
	}
}

// Break stops the program if the debug tag is active and the string in tag is
// contained in the DEBUG_BREAK environment variable.
func Break(tag string) {
	// check if breaking is enabled
	if v, ok := opts.breaks[tag]; !ok || !v {
		return
	}

	_, file, line, _ := runtime.Caller(1)
	Log("break", "stopping process %d at %s (%v:%v)\n", os.Getpid(), tag, file, line)
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		panic(err)
	}

	err = p.Signal(syscall.SIGSTOP)
	if err != nil {
		panic(err)
	}
}

// BreakIf stops the program if the debug tag is active and the string in tag
// is contained in the DEBUG_BREAK environment variable and the return value of
// fn is true.
func BreakIf(tag string, fn func() bool) {
	// check if breaking is enabled
	if v, ok := opts.breaks[tag]; !ok || !v {
		return
	}

	if fn() {
		Break(tag)
	}
}
