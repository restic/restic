package rclone

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/rest"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/limiter"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/net/http2"
)

// Backend is used to access data stored somewhere via rclone.
type Backend struct {
	*rest.Backend
	tr         *http2.Transport
	cmd        *exec.Cmd
	waitCh     <-chan struct{}
	waitResult error
	wg         *sync.WaitGroup
	conn       *StdioConn
}

// run starts command with args and initializes the StdioConn.
func run(command string, args ...string) (*StdioConn, *exec.Cmd, *sync.WaitGroup, func() error, error) {
	cmd := exec.Command(command, args...)

	p, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	var wg sync.WaitGroup

	// start goroutine to add a prefix to all messages printed by to stderr by rclone
	wg.Add(1)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(p)
		for sc.Scan() {
			fmt.Fprintf(os.Stderr, "rclone: %v\n", sc.Text())
		}
	}()

	r, stdin, err := os.Pipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	stdout, w, err := os.Pipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	cmd.Stdin = r
	cmd.Stdout = w

	bg, err := backend.StartForeground(cmd)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	c := &StdioConn{
		stdin:  stdout,
		stdout: stdin,
		cmd:    cmd,
	}

	return c, cmd, &wg, bg, nil
}

// wrappedConn adds bandwidth limiting capabilities to the StdioConn by
// wrapping the Read/Write methods.
type wrappedConn struct {
	*StdioConn
	io.Reader
	io.Writer
}

func (c wrappedConn) Read(p []byte) (int, error) {
	return c.Reader.Read(p)
}

func (c wrappedConn) Write(p []byte) (int, error) {
	return c.Writer.Write(p)
}

func wrapConn(c *StdioConn, lim limiter.Limiter) wrappedConn {
	wc := wrappedConn{
		StdioConn: c,
		Reader:    c,
		Writer:    c,
	}
	if lim != nil {
		wc.Reader = lim.Downstream(c)
		wc.Writer = lim.UpstreamWriter(c)
	}

	return wc
}

// New initializes a Backend and starts the process.
func New(cfg Config, lim limiter.Limiter) (*Backend, error) {
	var (
		args []string
		err  error
	)

	// build program args, start with the program
	if cfg.Program != "" {
		a, err := backend.SplitShellStrings(cfg.Program)
		if err != nil {
			return nil, err
		}
		args = append(args, a...)
	} else {
		args = append(args, "rclone")
	}

	// then add the arguments
	if cfg.Args != "" {
		a, err := backend.SplitShellStrings(cfg.Args)
		if err != nil {
			return nil, err
		}

		args = append(args, a...)
	} else {
		args = append(args,
			"serve", "restic", "--stdio",
			"--b2-hard-delete", "--drive-use-trash=false")
	}

	// finally, add the remote
	args = append(args, cfg.Remote)
	arg0, args := args[0], args[1:]

	debug.Log("running command: %v %v", arg0, args)
	stdioConn, cmd, wg, bg, err := run(arg0, args...)
	if err != nil {
		return nil, err
	}

	var conn net.Conn = stdioConn
	if lim != nil {
		conn = wrapConn(stdioConn, lim)
	}

	dialCount := 0
	tr := &http2.Transport{
		AllowHTTP: true, // this is not really HTTP, just stdin/stdout
		DialTLS: func(network, address string, cfg *tls.Config) (net.Conn, error) {
			debug.Log("new connection requested, %v %v", network, address)
			if dialCount > 0 {
				panic("dial count > 0")
			}
			dialCount++
			return conn, nil
		},
	}

	waitCh := make(chan struct{})
	be := &Backend{
		tr:     tr,
		cmd:    cmd,
		waitCh: waitCh,
		conn:   stdioConn,
		wg:     wg,
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		debug.Log("waiting for error result")
		err := cmd.Wait()
		debug.Log("Wait returned %v", err)
		be.waitResult = err
		close(waitCh)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()
		debug.Log("monitoring command to cancel first HTTP request context")
		select {
		case <-ctx.Done():
			debug.Log("context has been cancelled, returning")
		case <-be.waitCh:
			debug.Log("command has exited, cancelling context")
			cancel()
		}
	}()

	// send an HTTP request to the base URL, see if the server is there
	client := &http.Client{
		Transport: debug.RoundTripper(tr),
		Timeout:   60 * time.Second,
	}

	// request a random file which does not exist. we just want to test when
	// rclone is able to accept HTTP requests.
	url := fmt.Sprintf("http://localhost/file-%d", rand.Uint64())

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", rest.ContentTypeV2)
	req.Cancel = ctx.Done()

	res, err := ctxhttp.Do(ctx, client, req)
	if err != nil {
		bg()
		_ = cmd.Process.Kill()
		return nil, errors.Errorf("error talking HTTP to rclone: %v", err)
	}

	debug.Log("HTTP status %q returned, moving instance to background", res.Status)
	bg()

	return be, nil
}

// Open starts an rclone process with the given config.
func Open(cfg Config, lim limiter.Limiter) (*Backend, error) {
	be, err := New(cfg, lim)
	if err != nil {
		return nil, err
	}

	url, err := url.Parse("http://localhost/")
	if err != nil {
		return nil, err
	}

	restConfig := rest.Config{
		Connections: cfg.Connections,
		URL:         url,
	}

	restBackend, err := rest.Open(restConfig, debug.RoundTripper(be.tr))
	if err != nil {
		return nil, err
	}

	be.Backend = restBackend
	return be, nil
}

// Create initializes a new restic repo with clone.
func Create(cfg Config) (*Backend, error) {
	be, err := New(cfg, nil)
	if err != nil {
		return nil, err
	}

	debug.Log("new backend created")

	url, err := url.Parse("http://localhost/")
	if err != nil {
		return nil, err
	}

	restConfig := rest.Config{
		Connections: 20,
		URL:         url,
	}

	restBackend, err := rest.Create(restConfig, debug.RoundTripper(be.tr))
	if err != nil {
		_ = be.Close()
		return nil, err
	}

	be.Backend = restBackend
	return be, nil
}

const waitForExit = 5 * time.Second

// Close terminates the backend.
func (be *Backend) Close() error {
	debug.Log("exiting rclone")
	be.tr.CloseIdleConnections()

	select {
	case <-be.waitCh:
		debug.Log("rclone exited")
	case <-time.After(waitForExit):
		debug.Log("timeout, closing file descriptors")
		err := be.conn.Close()
		if err != nil {
			return err
		}
	}

	be.wg.Wait()
	debug.Log("wait for rclone returned: %v", be.waitResult)
	return be.waitResult
}
