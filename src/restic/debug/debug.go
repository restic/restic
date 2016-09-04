// +build debug

package debug

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"restic/fs"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"restic/errors"
)

type process struct {
	tag       string
	goroutine int
}

var opts struct {
	logger *log.Logger
	tags   map[string]bool
	last   map[process]time.Time
	m      sync.Mutex
}

// make sure that all the initialization happens before the init() functions
// are called, cf https://golang.org/ref/spec#Package_initialization
var _ = initDebug()

func initDebug() bool {
	initDebugLogger()
	initDebugTags()

	fmt.Fprintf(os.Stderr, "debug enabled\n")

	return true
}

func initDebugLogger() {
	debugfile := os.Getenv("DEBUG_LOG")
	if debugfile == "" {
		return
	}

	fmt.Fprintf(os.Stderr, "debug log file %v\n", debugfile)

	f, err := fs.OpenFile(debugfile, os.O_WRONLY|os.O_APPEND, 0600)

	if err == nil {
		_, err = f.Seek(2, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to seek to the end of %v: %v\n", debugfile, err)
			os.Exit(3)
		}
	}

	if err != nil && os.IsNotExist(errors.Cause(err)) {
		f, err = fs.OpenFile(debugfile, os.O_WRONLY|os.O_CREATE, 0600)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to open debug log file: %v\n", err)
		os.Exit(2)
	}

	opts.logger = log.New(f, "", log.LstdFlags)
}

func initDebugTags() {
	opts.tags = make(map[string]bool)
	opts.last = make(map[process]time.Time)

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

// taken from https://github.com/VividCortex/trace
func goroutineNum() int {
	b := make([]byte, 20)
	runtime.Stack(b, false)
	var num int

	fmt.Sscanf(string(b), "goroutine %d ", &num)
	return num
}

// taken from https://github.com/VividCortex/trace
func getPosition(goroutine int) string {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		return ""
	}

	return fmt.Sprintf("%3d %s:%d", goroutine, filepath.Base(file), line)
}

var maxTagLen = 10

func Log(tag string, f string, args ...interface{}) {
	opts.m.Lock()
	defer opts.m.Unlock()

	goroutine := goroutineNum()

	last, ok := opts.last[process{tag, goroutine}]
	if !ok {
		last = time.Now()
	}
	current := time.Now()
	opts.last[process{tag, goroutine}] = current

	if len(f) == 0 || f[len(f)-1] != '\n' {
		f += "\n"
	}

	if len(tag) > maxTagLen {
		maxTagLen = len(tag)
	}

	formatStringTag := "%2.3f [%" + strconv.FormatInt(int64(maxTagLen), 10) + "s]"
	formatString := fmt.Sprintf(formatStringTag+" %s %s", current.Sub(last).Seconds(), tag, getPosition(goroutine), f)

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
