// White-box tests of the composition root: cmd/shift2eu-server is a main package,
// so its tests are package main, the only way a test file reaches the package at
// all (Go refuses an import of a main package). The seam is run, the sibling of
// cli.Run: a test passes the flags, a fake lookupEnv and a stderr it reads,
// learns the bound address through the listening callback, and cancels the
// context to shut the server down, so no test starts a process and none needs
// a port of its own beyond 127.0.0.1:0.
//
// run's flags and the shape of its errors are the ones the CLI uses
// (cmd/shift2eu/main.go, internal/cli): one line per startup error, prefixed
// "shift2eu-server: ", and exit code 2 for a usage or configuration error.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	// loopbackListen is the --listen value every test uses: port 0, so the
	// kernel picks a free port and two tests can never collide.
	loopbackListen = "127.0.0.1:0"

	// errorPrefix is how the server prefixes a startup error line on stderr.
	errorPrefix = "shift2eu-server: "

	// tokenEnv is the name of the environment variable the bearer token comes
	// from, never a token. There is no token flag: a token in an argument would
	// show in the process list.
	tokenEnv = "SHIFT2EU_SERVER_TOKEN" //nolint:gosec // this is the variable's name, not a credential

	// startBound is how long a run may take to listen or to return before a
	// test calls it hung: a configuration error returns at once, so only a
	// build under -race on a loaded machine needs a generous bound.
	startBound = 20 * time.Second

	// stopBound is how long a cancelled run may take to return: the graceful
	// shutdown finishes in-flight requests within its own deadline, which no
	// test may hard-code, so this is the bound outside it.
	stopBound = 30 * time.Second

	// requestBound bounds one HTTP request a test makes through the client.
	requestBound = 60 * time.Second

	// readyBound is how long /readyz may take to answer 200 once run is
	// listening: every embedded catalog has to be loaded by then.
	readyBound = 10 * time.Second

	// connCapBound is how long a connection the server must close, or a slot it
	// frees, may take to show its effect: the server closes it at once, so only
	// a build under -race on a loaded machine needs this much.
	connCapBound = 10 * time.Second
)

// syncBuffer is a stderr that run writes to while the test reads it: the
// access log is written by request goroutines, so the buffer must lock.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write appends p, as io.Writer.
func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

// String returns what has been written so far.
func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// lookupEnvOf returns the lookupEnv run takes, over vars: a key the map holds is
// a variable that is set, with that value, and a key it does not hold is a
// variable that is unset, which is what os.LookupEnv reports. The map is copied,
// so no test can change the environment of a run it already handed over, and a
// variable the map holds as "" is set to nothing: the server must tell that apart
// from an unset variable, because ADR 0019 refuses a token source that is
// configured but holds no token.
func lookupEnvOf(vars map[string]string) func(string) (string, bool) {
	copied := make(map[string]string, len(vars))
	for name, value := range vars {
		copied[name] = value
	}

	return func(name string) (string, bool) {
		value, ok := copied[name]

		return value, ok
	}
}

// server is one run of run in a goroutine: the address it listens on, the
// stderr it writes, and the exit code it returns once the context is cancelled.
type server struct {
	// addr is the address a test dials: the host the listener bound, with the
	// port it was given.
	addr string
	// port is that port, as the Host header and the allowed-host list carry it.
	port string
	// stderr is what the server wrote: startup errors and the access log.
	stderr *syncBuffer

	client *http.Client

	mu     sync.Mutex
	code   int
	exited chan struct{}
	cancel context.CancelFunc
}

// startServer runs the server with the given listen address, environment and
// extra flags, and returns it once run has called listening. A run that returns
// instead, with an error, fails the test with its exit code and stderr. The
// server is stopped when the test ends, so a failing test leaks no goroutine.
func startServer(t *testing.T, listen string, vars map[string]string, args ...string) *server {
	t.Helper()

	if listen == "" {
		listen = loopbackListen
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &server{
		stderr: &syncBuffer{},
		client: &http.Client{Timeout: requestBound},
		exited: make(chan struct{}),
		cancel: cancel,
	}

	listening := make(chan net.Addr, 1)
	argv := append([]string{"--listen", listen}, args...)

	go func() {
		code := run(ctx, argv, lookupEnvOf(vars), s.stderr, func(addr net.Addr) { listening <- addr })

		s.mu.Lock()
		s.code = code
		s.mu.Unlock()

		close(s.exited)
	}()

	t.Cleanup(func() {
		cancel()
		<-s.exited
	})

	select {
	case addr := <-listening:
		tcp, ok := addr.(*net.TCPAddr)
		if !ok {
			t.Fatalf("run called listening with %v, want a *net.TCPAddr", addr)
		}

		s.port = strconv.Itoa(tcp.Port)
		// The address a test dials: the loopback address the listener bound, so
		// that a name resolving to another loopback address is still reached,
		// and 127.0.0.1 for a listener on 0.0.0.0 or ::, which a test reaches on
		// loopback and never off the machine.
		host := "127.0.0.1"
		if tcp.IP != nil && tcp.IP.IsLoopback() {
			host = tcp.IP.String()
		}

		s.addr = net.JoinHostPort(host, s.port)
	case <-s.exited:
		t.Fatalf("run(%v) returned %d before listening; stderr = %q", argv, s.exitCode(), s.stderr.String())
	case <-time.After(startBound):
		t.Fatalf("run(%v) neither listened nor returned within %s; stderr = %q", argv, startBound, s.stderr.String())
	}

	return s
}

// exitCode returns the code run returned; it is valid once exited is closed.
func (s *server) exitCode() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.code
}

// waitForLine polls stderr until it holds a line starting with prefix and returns
// that line, failing the test when no such line appears within startBound: a
// startup line may be written from the goroutine of the run, after the address is
// announced.
func (s *server) waitForLine(t *testing.T, prefix string) string {
	t.Helper()

	deadline := time.Now().Add(startBound)

	for {
		for _, line := range strings.Split(s.stderr.String(), "\n") {
			if strings.HasPrefix(line, prefix) {
				return line
			}
		}

		if time.Now().After(deadline) {
			t.Fatalf("stderr holds no line starting with %q within %s; stderr = %q", prefix, startBound, s.stderr.String())
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// stop cancels the context and returns the code run returned, failing the test
// when the run does not return within stopBound.
func (s *server) stop(t *testing.T) int {
	t.Helper()

	s.cancel()

	select {
	case <-s.exited:
		return s.exitCode()
	case <-time.After(stopBound):
		t.Fatalf("run did not return within %s of a cancelled context; stderr = %q", stopBound, s.stderr.String())
	}

	return 0
}

// answer is one response a test read: its status, its headers and its body. The
// body is read and the connection released by (*server).do, so no test holds an
// unclosed one.
type answer struct {
	status int
	header http.Header
	body   []byte
}

// do sends req to the server and returns the answer. An absolute or relative URL
// is filled in from the server's address; the Host a request sends is whatever
// req.Host holds.
func (s *server) do(t *testing.T, req *http.Request) answer {
	t.Helper()

	if !req.URL.IsAbs() {
		req.URL.Scheme = "http"
		req.URL.Host = s.addr
	}

	resp, err := s.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s: reading the response body: %v", req.Method, req.URL.Path, err)
	}

	return answer{status: resp.StatusCode, header: resp.Header, body: body}
}

// get sends GET path to the server and returns the status and the body.
func (s *server) get(t *testing.T, path string) (int, []byte) {
	t.Helper()

	got := s.do(t, newRequest(t, http.MethodGet, path, nil))

	return got.status, got.body
}

// newRequest returns a request of method and path with an optional body, ready
// for (*server).do.
func newRequest(t *testing.T, method, path string, body []byte) *http.Request {
	t.Helper()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, path, reader)
	if err != nil {
		t.Fatalf("building the %s %s request: %v", method, path, err)
	}

	return req
}

// assessRequest returns the request body POST /v1/assessments takes: the
// inventory document and the target keys.
func assessRequest(t *testing.T, document []byte, targets ...string) []byte {
	t.Helper()

	body, err := json.Marshal(struct {
		Inventory json.RawMessage `json:"inventory"`
		Targets   []string        `json:"targets"`
	}{Inventory: document, Targets: targets})
	if err != nil {
		t.Fatalf("encoding the request body: %v", err)
	}

	return body
}

// fixtureInventory returns the sanitised inventory document the e2e suite and
// the parity test share, as the request body holds it: an inventory of several
// categories and fits, so a report is the whole renderer's work.
func fixtureInventory(t *testing.T) []byte {
	t.Helper()

	document, err := os.ReadFile(filepath.Join("..", "..", "test", "e2e", "testdata", "inventory.json"))
	if err != nil {
		t.Fatalf("reading the shared inventory fixture: %v", err)
	}

	return document
}

// postAssessment posts body to /v1/assessments as the reports' JSON and returns
// the status and the returned document. The request needs no token: every test
// that uses it runs the server without one.
func (s *server) postAssessment(t *testing.T, body []byte) (int, []byte) {
	t.Helper()

	req := newRequest(t, http.MethodPost, "/v1/assessments", body)
	req.Header.Set("Content-Type", "application/json")

	got := s.do(t, req)

	return got.status, got.body
}

// dial opens a TCP connection to the server and returns it with a buffered
// reader over it, for the tests that must speak HTTP themselves. The connection
// is closed when the test ends.
func dial(t *testing.T, s *server) (net.Conn, *bufio.Reader) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", s.addr, startBound)
	if err != nil {
		t.Fatalf("dial %s: %v", s.addr, err)
	}

	t.Cleanup(func() { _ = conn.Close() })

	return conn, bufio.NewReader(conn)
}

// TestServerHealth pins the health endpoints of the running server: /healthz
// answers 200 for as long as the process serves, and /readyz answers 200 once
// run is listening, which means every target's embedded catalog has loaded.
// Both are open: a platform's probe carries no token. The server is stopped by
// the test's cleanup, which is where the graceful shutdown of
// TestServerGracefulShutdown pins the exit code.
func TestServerHealth(t *testing.T) {
	t.Parallel()

	s := startServer(t, loopbackListen, nil)

	t.Run("healthz", func(t *testing.T) {
		t.Parallel()

		status, body := s.get(t, "/healthz")

		if status != http.StatusOK {
			t.Errorf("GET /healthz = %d, want %d; body: %q", status, http.StatusOK, body)
		}
	})

	t.Run("readyz", func(t *testing.T) {
		t.Parallel()

		status, body := readyStatus(t, s)

		if status != http.StatusOK {
			t.Errorf("GET /readyz = %d, want %d within %s of listening; body: %q",
				status, http.StatusOK, readyBound, body)
		}
	})
}

// readyStatus polls GET /readyz until it answers 200 or readyBound passes, and
// returns the last answer: a readiness check that loads the catalogs itself may
// answer 503 once before it has them.
func readyStatus(t *testing.T, s *server) (int, []byte) {
	t.Helper()

	status, body := s.get(t, "/readyz")

	for deadline := time.Now().Add(readyBound); status != http.StatusOK && time.Now().Before(deadline); {
		time.Sleep(100 * time.Millisecond)

		status, body = s.get(t, "/readyz")
	}

	return status, body
}

// TestServerLoopbackRule pins the rule ADR 0019 gives the server: listening on
// anything but loopback needs a bearer token, or the operator's explicit
// --insecure-no-auth, because a server on a public address without one is an
// open proxy for the assessment of anyone's inventory. A refused configuration
// returns 2 before it listens, so no port is ever opened.
func TestServerLoopbackRule(t *testing.T) {
	t.Parallel()

	t.Run("non-loopback without a token is refused", func(t *testing.T) {
		t.Parallel()

		got := runOnce(t, nil, "--listen", "0.0.0.0:0")

		if got.listened {
			t.Fatalf("run listened on %v, want it to refuse 0.0.0.0:0 without a token", got.addr)
		}

		requireStartupError(t, got)
	})

	tests := []struct {
		name string
		vars map[string]string
		args []string
	}{
		{
			name: "non-loopback with --insecure-no-auth starts",
			args: []string{"--insecure-no-auth"},
		},
		{
			name: "non-loopback with a token starts",
			vars: map[string]string{tokenEnv: validToken},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := startServer(t, "0.0.0.0:0", tc.vars, tc.args...)

			if status, body := s.get(t, "/healthz"); status != http.StatusOK {
				t.Errorf("GET /healthz = %d, want %d; body: %q", status, http.StatusOK, body)
			}

			if code := s.stop(t); code != 0 {
				t.Errorf("run returned %d after a cancelled context, want 0; stderr = %q", code, s.stderr.String())
			}
		})
	}

	t.Run("a loopback listener needs neither", func(t *testing.T) {
		t.Parallel()

		s := startServer(t, loopbackListen, nil)

		if status, body := s.get(t, "/healthz"); status != http.StatusOK {
			t.Errorf("GET /healthz = %d, want %d; body: %q", status, http.StatusOK, body)
		}

		// The token variable is not set at all: the API answers on loopback
		// without an Authorization header, which an unset variable must never
		// turn into the empty-source startup error.
		if status, body := s.get(t, "/v1/targets"); status != http.StatusOK {
			t.Errorf("GET /v1/targets without %s set = %d, want %d; body: %q",
				tokenEnv, status, http.StatusOK, body)
		}
	})
}

// TestServerCheckAuth pins the rule the listener must satisfy, factored out of
// setUp so it can be applied twice: before the port is opened, to the --listen
// value, and after it, to the address the listener bound (audit M1). Loopback
// needs neither a credential nor an override; anything else needs the token or
// the operator's explicit --insecure-no-auth, and the refusal names the
// address, both token sources and the override, never a token.
func TestServerCheckAuth(t *testing.T) {
	t.Parallel()

	// addr is an address in the documentation range, so a failure message never
	// quotes a real one.
	const addr = "192.0.2.10:8080"

	tests := []struct {
		name           string
		loopback       bool
		hasToken       bool
		insecureNoAuth bool
		wantErr        bool
	}{
		{name: "loopback without a token", loopback: true},
		{name: "loopback with a token", loopback: true, hasToken: true},
		{name: "loopback with --insecure-no-auth", loopback: true, insecureNoAuth: true},
		{name: "non-loopback with a token", hasToken: true},
		{name: "non-loopback with --insecure-no-auth", insecureNoAuth: true},
		{name: "non-loopback with a token and --insecure-no-auth", hasToken: true, insecureNoAuth: true},
		{name: "non-loopback without a token or --insecure-no-auth", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := checkAuth(tc.loopback, tc.hasToken, tc.insecureNoAuth, addr)

			if !tc.wantErr {
				if err != nil {
					t.Fatalf("checkAuth(loopback=%v, token=%v, insecureNoAuth=%v, %s) = %v, want nil",
						tc.loopback, tc.hasToken, tc.insecureNoAuth, addr, err)
				}

				return
			}

			if err == nil {
				t.Fatalf("checkAuth(loopback=%v, token=%v, insecureNoAuth=%v, %s) = nil, want the non-loopback rule to refuse it",
					tc.loopback, tc.hasToken, tc.insecureNoAuth, addr)
			}

			for _, want := range []string{addr, tokenEnv, "--token-file", "--insecure-no-auth"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the refusal %q does not name %q", err, want)
				}
			}
		})
	}
}

// TestServerLocalhostListen pins the loopback decision for the name localhost: a
// listener on localhost:0 is a loopback listener, so the server starts without a
// token and serves, with and without the operator's --allowed-host. The decision
// must rest on the address the listener bound, not on the name alone: a name that
// resolves elsewhere still gets the non-loopback rule (audit M1).
func TestServerLocalhostListen(t *testing.T) {
	t.Parallel()

	t.Run("without --allowed-host", func(t *testing.T) {
		t.Parallel()

		s := startServer(t, "localhost:0", nil)

		if status, body := s.get(t, "/healthz"); status != http.StatusOK {
			t.Errorf("GET /healthz on localhost = %d, want %d; body: %q", status, http.StatusOK, body)
		}

		// The allow-list a tokenless loopback listener resolves answers its own
		// loopback names.
		req := newRequest(t, http.MethodGet, "/v1/targets", nil)
		req.Host = "localhost:" + s.port

		if got := s.do(t, req); got.status != http.StatusOK {
			t.Errorf("GET /v1/targets with Host localhost:%s on localhost = %d, want %d; body: %q",
				s.port, got.status, http.StatusOK, got.body)
		}
	})

	t.Run("with --allowed-host", func(t *testing.T) {
		t.Parallel()

		s := startServer(t, "localhost:0", nil, "--allowed-host", "app.example.com")

		req := newRequest(t, http.MethodGet, "/v1/targets", nil)
		req.Host = "app.example.com"

		if got := s.do(t, req); got.status != http.StatusOK {
			t.Errorf("GET /v1/targets with Host app.example.com on localhost = %d, want %d; body: %q",
				got.status, http.StatusOK, got.body)
		}

		if code := s.stop(t); code != 0 {
			t.Errorf("run returned %d after a cancelled context, want 0; stderr = %q", code, s.stderr.String())
		}
	})
}

// TestServerInsecureNoAuthWarning pins the warning of a server that serves
// beyond loopback without authentication (audit L1): one stderr line at startup,
// naming the address it serves, so an operator cannot leave such a server in
// place without a trace. With a token configured the server authenticates and
// says nothing.
func TestServerInsecureNoAuthWarning(t *testing.T) {
	t.Parallel()

	const warning = "shift2eu-server: warning: serving without authentication on "

	t.Run("a non-loopback listener warns", func(t *testing.T) {
		t.Parallel()

		s := startServer(t, "0.0.0.0:0", nil, "--insecure-no-auth")

		line := s.waitForLine(t, "shift2eu-server: warning: ")
		if !strings.HasPrefix(line, warning) {
			t.Fatalf("the warning line = %q, want it to start with %q", line, warning)
		}

		host, port, err := net.SplitHostPort(strings.TrimPrefix(line, warning))
		if err != nil {
			t.Fatalf("the warning line %q does not end in a host:port address: %v", line, err)
		}

		if host == "" || port != s.port {
			t.Errorf("the warning line %q names %q with the port %q, want the bound address with the port %q",
				line, host, port, s.port)
		}

		if code := s.stop(t); code != 0 {
			t.Errorf("run returned %d after a cancelled context, want 0; stderr = %q", code, s.stderr.String())
		}
	})

	t.Run("a token configures authentication and warns not", func(t *testing.T) {
		t.Parallel()

		s := startServer(t, "0.0.0.0:0", map[string]string{tokenEnv: validToken})

		if status, body := s.get(t, "/healthz"); status != http.StatusOK {
			t.Errorf("GET /healthz = %d, want %d; body: %q", status, http.StatusOK, body)
		}

		if logged := s.stderr.String(); strings.Contains(logged, "warning:") {
			t.Errorf("stderr = %q, want no warning: the server authenticates with a token", logged)
		}
	})
}

// TestServerConnectionCap pins --max-connections (audit M2): the server holds at
// most the number of connections the operator names, a connection over the cap
// is closed at once rather than queued for a slot, a slot is free again once a
// connection closes, and a cap of zero or less is a configuration error. The
// request-header cap is pinned too: a header far over it is refused rather than
// served.
func TestServerConnectionCap(t *testing.T) {
	t.Parallel()

	t.Run("a connection over the cap is closed at once", func(t *testing.T) {
		t.Parallel()

		s := startServer(t, loopbackListen, nil, "--max-connections", "2")

		held := make([]net.Conn, 0, 2)

		for range 2 {
			conn, _ := dial(t, s)
			held = append(held, conn)
		}

		// The cap is full, so the third connection is closed without an answer
		// instead of waiting for a slot.
		over, overReader := dial(t, s)

		if err := over.SetReadDeadline(time.Now().Add(connCapBound)); err != nil {
			t.Fatalf("setting the read deadline of the connection over the cap: %v", err)
		}

		switch b, err := overReader.ReadByte(); {
		case err == nil:
			t.Fatalf("the connection over the cap got the byte %q, want the server to close it without an answer", string(b))
		case readTimedOut(err):
			t.Fatalf("the connection over the cap was still open after %s, want the server to close it at once rather than queue it", connCapBound)
		}

		// Closing a connection frees its slot for the next client.
		if err := held[0].Close(); err != nil {
			t.Fatalf("closing one of the connections holding the cap: %v", err)
		}

		deadline := time.Now().Add(connCapBound)

		var (
			last   error
			served bool
		)

		for time.Now().Before(deadline) {
			status, err := s.probe(http.MethodGet, "/healthz", nil)
			if err == nil && status == http.StatusOK {
				served = true

				break
			}

			last = err
			time.Sleep(pollInterval)
		}

		if !served {
			t.Fatalf("no request was served within %s of a connection closing, so the cap never frees a slot: %v; stderr = %q",
				connCapBound, last, s.stderr.String())
		}

		if code := s.stop(t); code != 0 {
			t.Errorf("run returned %d after a cancelled context, want 0; stderr = %q", code, s.stderr.String())
		}
	})

	t.Run("a request header over the cap is not served", func(t *testing.T) {
		t.Parallel()

		s := startServer(t, loopbackListen, nil)

		conn, reader := dial(t, s)

		// One header far over the 16 KiB cap the server must set: the request is
		// refused, never served. A refusal may reset a connection that still
		// holds unread bytes, so a broken read is a refusal as well.
		oversized := strings.Repeat("a", 32<<10)

		if err := conn.SetWriteDeadline(time.Now().Add(connCapBound)); err != nil {
			t.Fatalf("setting the write deadline of the oversized request: %v", err)
		}

		request := fmt.Sprintf("GET /healthz HTTP/1.1\r\nHost: %s\r\nX-Probe: %s\r\n\r\n", s.addr, oversized)

		if _, err := conn.Write([]byte(request)); err != nil {
			t.Fatalf("writing the request with a %d-byte header: %v", len(oversized), err)
		}

		if err := conn.SetReadDeadline(time.Now().Add(connCapBound)); err != nil {
			t.Fatalf("setting the read deadline of the oversized request: %v", err)
		}

		resp, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
		if err != nil {
			if readTimedOut(err) {
				t.Fatalf("the server neither answered nor closed the connection within %s, want it to refuse a %d-byte request header",
					connCapBound, len(oversized))
			}

			return
		}

		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusRequestHeaderFieldsTooLarge {
			t.Errorf("GET /healthz with a %d-byte request header = %d, want %d: the server caps the request headers",
				len(oversized), resp.StatusCode, http.StatusRequestHeaderFieldsTooLarge)
		}
	})

	tests := []string{"0", "-1"}

	for _, value := range tests {
		t.Run("--max-connections "+value+" is a configuration error", func(t *testing.T) {
			t.Parallel()

			got := runOnce(t, nil, "--listen", loopbackListen, "--max-connections", value)

			if got.listened {
				t.Fatalf("run listened on %v with --max-connections %s, want a startup error", got.addr, value)
			}

			requireStartupError(t, got)

			if !strings.Contains(got.stderr, "max-connections") {
				t.Errorf("stderr = %q, want it to name --max-connections", got.stderr)
			}
		})
	}
}

// readTimedOut reports whether err is a deadline that passed on a connection,
// which is how a test sees a server that neither answered nor closed.
func readTimedOut(err error) bool {
	var netErr net.Error

	return errors.As(err, &netErr) && netErr.Timeout()
}

// TestServerListenFailure pins a listener that cannot open (review S2): an
// address already in use is a runtime failure, exit 1 with one line prefixed
// "shift2eu-server: ", not the exit 2 of a configuration error and not a silent
// exit 0.
func TestServerListenFailure(t *testing.T) {
	t.Parallel()

	held, err := net.Listen("tcp", loopbackListen)
	if err != nil {
		t.Fatalf("listening on %s to hold the address: %v", loopbackListen, err)
	}

	t.Cleanup(func() { _ = held.Close() })

	got := runOnce(t, nil, "--listen", held.Addr().String())

	if got.listened {
		t.Fatalf("run listened on %v beside the listener that holds the address", got.addr)
	}

	if got.code != 1 {
		t.Errorf("exit code = %d, want 1 for a listener that cannot open; stderr = %q", got.code, got.stderr)
	}

	lines := strings.Split(strings.TrimSuffix(got.stderr, "\n"), "\n")
	if len(lines) != 1 || lines[0] == "" {
		t.Fatalf("stderr = %q, want exactly one line", got.stderr)
	}

	if !strings.HasPrefix(lines[0], errorPrefix) {
		t.Errorf("stderr = %q, want it to start with %q", got.stderr, errorPrefix)
	}

	if !strings.Contains(lines[0], held.Addr().String()) {
		t.Errorf("the error line %q does not name the address %s, so the operator cannot tell which listener failed",
			lines[0], held.Addr())
	}
}

// TestServerConcurrencyLine pins the effective concurrency the server reports
// after it announces its address (review S5): a run that sized itself from the
// CPUs and GOMEMLIMIT says what it picked, so the operator can see it rather
// than infer it.
func TestServerConcurrencyLine(t *testing.T) {
	t.Parallel()

	const (
		prefix    = "shift2eu-server: concurrency "
		listening = "shift2eu-server: listening on "
	)

	tests := []struct {
		name string
		args []string
		// want is the value the line must hold; 0 means the computed default,
		// which the bounds below cover.
		want int
	}{
		{name: "--concurrency names it", args: []string{"--concurrency", "3"}, want: 3},
		{name: "the computed default", args: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := startServer(t, loopbackListen, nil, tc.args...)

			line := s.waitForLine(t, prefix)

			value, err := strconv.Atoi(strings.TrimPrefix(line, prefix))
			if err != nil {
				t.Fatalf("the concurrency line %q holds no number: %v; stderr = %q", line, err, s.stderr.String())
			}

			if tc.want != 0 && value != tc.want {
				t.Errorf("the concurrency line %q names %d, want %d", line, value, tc.want)
			}

			if value < 1 || value > runtime.GOMAXPROCS(0) {
				t.Errorf("the concurrency line %q names %d, want between 1 and the CPU count %d",
					line, value, runtime.GOMAXPROCS(0))
			}

			// The operator reads the two lines in the order the server writes
			// them: the address first, then what serves it.
			logged := s.stderr.String()

			announced := strings.Index(logged, listening)
			if announced < 0 {
				t.Fatalf("stderr holds no listening line; stderr = %q", logged)
			}

			if reported := strings.Index(logged, prefix); reported < announced {
				t.Errorf("the concurrency line comes before the listening line; stderr = %q", logged)
			}

			if code := s.stop(t); code != 0 {
				t.Errorf("run returned %d after a cancelled context, want 0; stderr = %q", code, s.stderr.String())
			}
		})
	}
}

// TestServerGracefulShutdown pins the shutdown: cancelling the context stops the
// listener and lets a request that is already being served finish, so a client
// hears the answer rather than a broken connection, and run then returns 0. The
// request is held in flight by sending half of its body before the cancellation
// and the rest after it: the slot is taken and the body is being read, which no
// other state a test can reach reports.
func TestServerGracefulShutdown(t *testing.T) {
	t.Parallel()

	s := startServer(t, loopbackListen, nil)

	body := assessRequest(t, fixtureInventory(t), "ovhcloud")
	half := len(body) / 2

	conn, reader := dial(t, s)

	request := fmt.Sprintf("POST /v1/assessments HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\n"+
		"Content-Length: %d\r\n\r\n", s.addr, len(body))

	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("writing the request headers: %v", err)
	}

	if _, err := conn.Write(body[:half]); err != nil {
		t.Fatalf("writing the first half of the body: %v", err)
	}

	// Give the handler the moment it needs to take its slot and start reading;
	// the cancellation below must land while the request is in flight.
	time.Sleep(200 * time.Millisecond)

	s.cancel()

	if _, err := conn.Write(body[half:]); err != nil {
		t.Fatalf("writing the rest of the body after the cancellation: %v", err)
	}

	resp, err := http.ReadResponse(reader, &http.Request{Method: http.MethodPost})
	if err != nil {
		t.Fatalf("reading the response of the in-flight request: %v; stderr = %q", err, s.stderr.String())
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("the in-flight request got %d after the cancellation, want %d; stderr = %q",
			resp.StatusCode, http.StatusOK, s.stderr.String())
	}

	if code := s.stop(t); code != 0 {
		t.Errorf("run returned %d after a graceful shutdown, want 0; stderr = %q", code, s.stderr.String())
	}
}

// runResult is one run that a test expects to end without listening.
type runResult struct {
	code int
	// stderr is what the run wrote.
	stderr string
	// listened reports whether run called listening, and addr is the address it
	// reported.
	listened bool
	addr     net.Addr
}

// runOnce runs the server to completion and returns its exit code and stderr. A
// run that listens is cancelled at once and reported as listened, so a test
// whose case must be refused names that as a failure rather than hanging.
func runOnce(t *testing.T, vars map[string]string, args ...string) runResult {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stderr syncBuffer

	listening := make(chan net.Addr, 1)
	exited := make(chan int, 1)

	go func() {
		exited <- run(ctx, args, lookupEnvOf(vars), &stderr, func(addr net.Addr) { listening <- addr })
	}()

	select {
	case code := <-exited:
		return runResult{code: code, stderr: stderr.String()}
	case addr := <-listening:
		cancel()

		return runResult{code: <-exited, stderr: stderr.String(), listened: true, addr: addr}
	case <-time.After(startBound):
		cancel()

		t.Fatalf("run(%v) neither listened nor returned within %s; stderr = %q", args, startBound, stderr.String())

		return runResult{}
	}
}

// requireStartupError fails the test unless got is a configuration or usage error
// the server reports in its own words: exit code 2, and exactly one stderr line,
// which starts with errorPrefix. It is the shape of every error the composition
// root raises itself, as the CLI's are (internal/cli).
func requireStartupError(t *testing.T, got runResult) {
	t.Helper()

	if got.code != 2 {
		t.Errorf("exit code = %d, want 2; stderr = %q", got.code, got.stderr)
	}

	lines := strings.Split(strings.TrimSuffix(got.stderr, "\n"), "\n")
	if len(lines) != 1 || lines[0] == "" {
		t.Fatalf("stderr = %q, want exactly one line", got.stderr)
	}

	if !strings.HasPrefix(lines[0], errorPrefix) {
		t.Errorf("stderr = %q, want it to start with %q", got.stderr, errorPrefix)
	}
}

// requireFlagError fails the test unless got is a flag the command line does not
// accept: exit code 2, nothing listened, and a first stderr line that names the
// flag. The flag package writes the usage text after that line, so only the line
// is pinned here: the usage text is the CLI's convention, not this test's.
func requireFlagError(t *testing.T, got runResult, flagName string) {
	t.Helper()

	if got.code != 2 {
		t.Errorf("exit code = %d, want 2; stderr = %q", got.code, got.stderr)
	}

	firstLine, _, _ := strings.Cut(got.stderr, "\n")

	if !strings.Contains(firstLine, flagName) {
		t.Errorf("the first stderr line %q, want it to name the flag %q", firstLine, flagName)
	}
}
