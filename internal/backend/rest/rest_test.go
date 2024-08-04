package rest_test

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend/rest"
	"github.com/restic/restic/internal/backend/test"
	rtest "github.com/restic/restic/internal/test"
)

var (
	serverStartedRE = regexp.MustCompile("^start server on (.*)$")
)

func runRESTServer(ctx context.Context, t testing.TB, dir, reqListenAddr string) (*url.URL, func()) {
	srv, err := exec.LookPath("rest-server")
	if err != nil {
		t.Skip(err)
	}

	// create our own context, so that our cleanup can cancel and wait for completion
	// this will ensure any open ports, open unix sockets etc are properly closed
	processCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(processCtx, srv, "--no-auth", "--path", dir, "--listen", reqListenAddr)

	// this cancel func is called by when the process context is done
	cmd.Cancel = func() error {
		// we execute in a Go-routine as we know the caller will
		// be waiting on a .Wait() regardless
		go func() {
			// try to send a graceful termination signal
			if cmd.Process.Signal(syscall.SIGTERM) == nil {
				// if we succeed, then wait a few seconds
				time.Sleep(2 * time.Second)
			}
			// and then make sure it's killed either way, ignoring any error code
			_ = cmd.Process.Kill()
		}()
		return nil
	}

	// this is the cleanup function that we return the caller,
	// which will cancel our process context, and then wait for it to finish
	cleanup := func() {
		cancel()
		_ = cmd.Wait()
	}

	// but in-case we don't finish this method, e.g. by calling t.Fatal()
	// we also defer a call to clean it up ourselves, guarded by a flag to
	// indicate that we returned the function to the caller to deal with.
	callerWillCleanUp := false
	defer func() {
		if !callerWillCleanUp {
			cleanup()
		}
	}()

	// send stdout to our std out
	cmd.Stdout = os.Stdout

	// capture stderr with a pipe, as we want to examine this output
	// to determine when the server is started and listening.
	cmdErr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}

	// start the rest-server
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// create a channel to receive the actual listen address on
	listenAddrCh := make(chan string)
	go func() {
		defer close(listenAddrCh)
		matched := false
		br := bufio.NewReader(cmdErr)
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				// we ignore errors, as code that relies on this
				// will happily fail via timeout and empty closed
				// channel.
				return
			}

			line = strings.Trim(line, "\r\n")
			if !matched {
				// look for the server started message, and return the address
				// that it's listening on
				matchedServerListen := serverStartedRE.FindSubmatch([]byte(line))
				if len(matchedServerListen) == 2 {
					listenAddrCh <- string(matchedServerListen[1])
					matched = true
				}
			}
			fmt.Fprintln(os.Stdout, line) // print all output to console
		}
	}()

	// wait for us to get an address,
	// or the parent context to cancel,
	// or for us to timeout
	var actualListenAddr string
	select {
	case <-processCtx.Done():
		t.Fatal(context.Canceled)
	case <-time.NewTimer(2 * time.Second).C:
		t.Fatal(context.DeadlineExceeded)
	case a, ok := <-listenAddrCh:
		if !ok {
			t.Fatal(context.Canceled)
		}
		actualListenAddr = a
	}

	// this translate the address that the server is listening on
	// to a URL suitable for us to connect to
	var addrToConnectTo string
	if strings.HasPrefix(reqListenAddr, "unix:") {
		addrToConnectTo = fmt.Sprintf("http+unix://%s:/restic-test/", actualListenAddr)
	} else {
		// while we may listen on 0.0.0.0, we connect to localhost
		addrToConnectTo = fmt.Sprintf("http://%s/restic-test/", strings.Replace(actualListenAddr, "0.0.0.0", "localhost", 1))
	}

	// parse to a URL
	url, err := url.Parse(addrToConnectTo)
	if err != nil {
		t.Fatal(err)
	}

	// indicate that we've completed successfully, and that the caller
	// is responsible for calling cleanup
	callerWillCleanUp = true
	return url, cleanup
}

func newTestSuite(url *url.URL, minimalData bool) *test.Suite[rest.Config] {
	return &test.Suite[rest.Config]{
		MinimalData: minimalData,

		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (*rest.Config, error) {
			cfg := rest.NewConfig()
			cfg.URL = url
			return &cfg, nil
		},

		Factory: rest.NewFactory(),
	}
}

func TestBackendREST(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/rest.TestBackendREST")
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir := rtest.TempDir(t)
	serverURL, cleanup := runRESTServer(ctx, t, dir, ":0")
	defer cleanup()

	newTestSuite(serverURL, false).RunTests(t)
}

func TestBackendRESTExternalServer(t *testing.T) {
	repostr := os.Getenv("RESTIC_TEST_REST_REPOSITORY")
	if repostr == "" {
		t.Skipf("environment variable %v not set", "RESTIC_TEST_REST_REPOSITORY")
	}

	cfg, err := rest.ParseConfig(repostr)
	if err != nil {
		t.Fatal(err)
	}

	newTestSuite(cfg.URL, true).RunTests(t)
}

func BenchmarkBackendREST(t *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir := rtest.TempDir(t)
	serverURL, cleanup := runRESTServer(ctx, t, dir, ":0")
	defer cleanup()

	newTestSuite(serverURL, false).RunBenchmarks(t)
}
