package reststdiohttp2

import (
	"bufio"
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
	"github.com/restic/restic/internal/backend/reststdiohttp2/stdioconn"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/limiter"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/net/http2"
)

// Backend uses the REST protocol over HTTP2 over a StdioConn to access data through a child process
type Backend struct {
	*rest.Backend
	tr         *http2.Transport
	cmd        *exec.Cmd
	waitCh     <-chan struct{}
	waitResult error
	wg         *sync.WaitGroup
	conn       *stdioconn.StdioConn
	warmupTime time.Duration
	exitTime   time.Duration
	restConfig rest.Config
}

// run starts command with args and initializes the StdioConn.
func run(command string, args ...string) (*stdioconn.StdioConn, *exec.Cmd, *sync.WaitGroup, func() error, error) {
	cmd := exec.Command(command, args...)

	p, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	var wg sync.WaitGroup

	// start goroutine to add a prefix to all messages printed by to stderr by child process
	wg.Add(1)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(p)
		for sc.Scan() {
			fmt.Fprintf(os.Stderr, command+": %v\n", sc.Text())
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

	c := stdioconn.New(stdout, stdin)

	return c, cmd, &wg, bg, nil
}

// wrappedConn adds bandwidth limiting capabilities to the StdioConn by
// wrapping the Read/Write methods.
type wrappedConn struct {
	*stdioconn.StdioConn
	io.Reader
	io.Writer
}

func (c wrappedConn) Read(p []byte) (int, error) {
	return c.Reader.Read(p)
}

func (c wrappedConn) Write(p []byte) (int, error) {
	return c.Writer.Write(p)
}

func wrapConn(c *stdioconn.StdioConn, lim limiter.Limiter) wrappedConn {
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
func New(args []string, lim limiter.Limiter, warmupTime time.Duration, exitTime time.Duration, connections uint) (*Backend, error) {
	arg0, args := args[0], args[1:]
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

	URL, err := url.Parse("http://localhost/")
	if err != nil {
		return nil, err
	}

	restConfig := rest.Config{
		Connections: connections,
		URL:         URL,
	}

	be := &Backend{
		tr:         tr,
		cmd:        cmd,
		waitCh:     waitCh,
		conn:       stdioConn,
		wg:         wg,
		warmupTime: warmupTime,
		exitTime:   exitTime,
		restConfig: restConfig,
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
		Transport: tr,
		Timeout:   be.warmupTime,
	}

	// request a random file which does not exist. we just want to test when
	// the server is able to accept HTTP requests.
	dummyURL := fmt.Sprintf("http://localhost/file-%d", rand.Uint64())

	req, err := http.NewRequest(http.MethodGet, dummyURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", rest.ContentTypeV2)
	req.Cancel = ctx.Done()

	res, err := ctxhttp.Do(ctx, client, req)
	if err != nil {
		bg()
		_ = cmd.Process.Kill()
		return nil, errors.Errorf("error talking HTTP to child process: %v", err)
	}

	debug.Log("HTTP status %q returned, moving instance to background", res.Status)
	bg()

	return be, nil
}

// Open initializes the underlying rest backend for an existing repo
func (be *Backend) Open() error {
	return be.init(rest.Open(be.restConfig, be.tr))
}

// Create initializes the underlying rest backend for a new repo
func (be *Backend) Create() error {
	return be.init(rest.Create(be.restConfig, be.tr))
}

func (be *Backend) init(restBackend *rest.Backend, err error) error {
	if err != nil {
		return err
	}
	be.Backend = restBackend
	return nil
}

// Close terminates the backend.
func (be *Backend) Close() error {
	be.tr.CloseIdleConnections()

	select {
	case <-be.waitCh:
		debug.Log("child process exited")
	case <-time.After(be.exitTime):
		debug.Log("timeout, closing file descriptors")
		err := be.conn.Close()
		if err != nil {
			return err
		}
	}

	be.wg.Wait()
	debug.Log("wait for child process returned: %v", be.waitResult)
	return be.waitResult
}
