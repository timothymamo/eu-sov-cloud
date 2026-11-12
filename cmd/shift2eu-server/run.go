package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/timothymamo/shift2eu/internal/httpapi"
)

// The http.Server limits (ADR 0019). A request's body is bounded by
// bodyReadTimeout from the moment its concurrency slot is taken, which the API
// sets on the connection in place of readTimeout; the rest of a request by these.
const (
	// readHeaderTimeout bounds how long a client may take to send the request
	// line and the headers: a client that stalls there holds a connection, not
	// a slot, and is closed after it.
	readHeaderTimeout = 10 * time.Second

	// readTimeout bounds reading a whole request, headers and body. It is no
	// less than bodyReadTimeout, as ADR 0019 requires.
	readTimeout = 30 * time.Second

	// bodyReadTimeout bounds reading the body of POST /v1/assessments once its
	// slot is taken (httpapi.WithBodyReadTimeout), 30 s as ADR 0019 settled.
	bodyReadTimeout = 30 * time.Second

	// writeTimeout bounds a request from the end of its headers to the end of
	// its answer: the body read, the assessment (about 0.4 s for the largest on
	// one core) and the write, with room for a slow reader of the answer.
	writeTimeout = 60 * time.Second

	// idleTimeout bounds how long a kept-alive connection waits for its next
	// request.
	idleTimeout = 60 * time.Second

	// maxHeaderBytes caps the request line and the headers; a larger one is
	// 431. The API reads Host, Authorization, Content-Type, Accept and Origin
	// only, which need far less (ADR 0019).
	maxHeaderBytes = 16 << 10

	// shutdownTimeout bounds the graceful shutdown: how long the requests in
	// flight may take to finish once the context is cancelled, below the 30 s a
	// container platform usually waits between SIGTERM and SIGKILL.
	shutdownTimeout = 25 * time.Second
)

// Exit codes, as the CLI's (internal/cli).
const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

// config is what the flags and the environment configure.
type config struct {
	listen         string
	tokenFile      string
	concurrency    int
	maxConnections int
	corsOrigins    stringList
	allowedHosts   stringList
	insecureNoAuth bool
}

// run is the server: it reads the flags in args and the environment through
// lookupEnv, listens, calls listening (when not nil) with the bound address,
// and serves the API until ctx is cancelled, then shuts down gracefully. It
// writes startup errors, prefixed "shift2eu-server: ", and the access log to
// stderr. It returns 0 after a graceful shutdown, 2 for a usage or
// configuration error, before anything listens, and 1 for a runtime failure.
func run(ctx context.Context, args []string, lookupEnv func(string) (string, bool),
	stderr io.Writer, listening func(addr net.Addr),
) int {
	stderr = &lockedWriter{w: stderr}

	cfg, code, ok := parseFlags(args, stderr)
	if !ok {
		return code
	}

	set, err := setUp(cfg, lookupEnv)
	if err != nil {
		printError(stderr, err)

		if errors.Is(err, errListen) {
			return exitFailure
		}

		return exitUsage
	}

	ln := newLimitListener(set.ln, cfg.maxConnections)

	srv := &http.Server{
		Handler:           accessLog(set.handler, stderr, time.Now),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          log.New(stderr, "shift2eu-server: ", 0),
	}

	for _, warning := range set.warnings {
		_, _ = fmt.Fprintf(stderr, "shift2eu-server: warning: %s\n", warning)
	}

	_, _ = fmt.Fprintf(stderr, "shift2eu-server: listening on %s\n", ln.Addr())
	_, _ = fmt.Fprintf(stderr, "shift2eu-server: concurrency %d\n", set.concurrency)

	if listening != nil {
		listening(ln.Addr())
	}

	return serve(ctx, srv, ln, stderr)
}

// errMaxConnections is the configuration error of a cap below one.
var errMaxConnections = errors.New("--max-connections must be 1 or more")

// parseFlags reads args into a config. It returns false with the exit code
// when the run ends here: 0 for -h, 2 for a usage error, which the flag package
// or this function has already reported on stderr.
func parseFlags(args []string, stderr io.Writer) (config, int, bool) {
	var cfg config

	fs := flag.NewFlagSet("shift2eu-server", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.listen, "listen", "127.0.0.1:8080",
		"`address` to listen on, host:port; a non-loopback address needs a token or --insecure-no-auth")
	fs.StringVar(&cfg.tokenFile, "token-file", "",
		"read the bearer token from the file at `path` (or set "+tokenEnvVar+"; never both)")
	fs.IntVar(&cfg.concurrency, "concurrency", 0,
		"serve at most `n` assessments at once; 0 sizes it from the CPUs and GOMEMLIMIT")
	fs.IntVar(&cfg.maxConnections, "max-connections", defaultMaxConnections,
		"hold at most `n` connections open; one over the cap is closed at once")
	fs.Var(&cfg.corsOrigins, "cors-origin", "allow CORS from this `origin` (repeatable); CORS is off without one")
	fs.Var(&cfg.allowedHosts, "allowed-host", "serve only this `host` in the Host header (repeatable)")
	fs.BoolVar(&cfg.insecureNoAuth, "insecure-no-auth", false,
		"listen on a non-loopback address without a bearer token")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return cfg, exitOK, false
		}

		return cfg, exitUsage, false
	}

	if fs.NArg() > 0 {
		printError(stderr, errors.New("unexpected arguments; shift2eu-server takes flags only"))

		return cfg, exitUsage, false
	}

	if cfg.concurrency < 0 {
		printError(stderr, errors.New("--concurrency must be 0 (the default) or more"))

		return cfg, exitUsage, false
	}

	if cfg.maxConnections < 1 {
		printError(stderr, errMaxConnections)

		return cfg, exitUsage, false
	}

	return cfg, exitOK, true
}

// errListen wraps a failure to open the listener, such as an address in use:
// a runtime failure, not a configuration error.
var errListen = errors.New("listen")

// setup is what setUp builds for run.
type setup struct {
	handler     http.Handler
	ln          net.Listener
	concurrency int
	// warnings are the startup warnings, each a line without its prefix.
	warnings []string
}

// setUp checks the configuration, opens the listener and builds the handler.
// Every check that can fail without the listener runs before it is opened, so a
// refused configuration never opens a port; the one that needs it, that a
// loopback listener bound a loopback address, closes it.
func setUp(cfg config, lookupEnv func(string) (string, bool)) (setup, error) {
	tok, err := readToken(lookupEnv, cfg.tokenFile)
	if err != nil {
		return setup{}, err
	}

	loopback, err := isLoopbackListen(cfg.listen)
	if err != nil {
		return setup{}, fmt.Errorf("--listen: %w", err)
	}

	if err := checkAuth(loopback, tok.set, cfg.insecureNoAuth, cfg.listen); err != nil {
		return setup{}, err
	}

	d := deps()
	ready := &readiness{targets: d.Targets}

	concurrency := cfg.concurrency
	if concurrency == 0 {
		concurrency = defaultConcurrency(runtime.GOMAXPROCS(0), memoryBudget(debug.SetMemoryLimit(-1)))
	}

	opts := []httpapi.Option{
		httpapi.WithMaxConcurrent(concurrency),
		httpapi.WithBodyReadTimeout(bodyReadTimeout),
		httpapi.WithReadiness(ready.check),
	}

	if tok.set {
		opts = append(opts, httpapi.WithBearerToken(tok.token))
	}

	if len(cfg.corsOrigins) > 0 {
		opts = append(opts, httpapi.WithAllowedOrigins(cfg.corsOrigins...))
	}

	if len(cfg.allowedHosts) > 0 {
		opts = append(opts, httpapi.WithAllowedHosts(cfg.allowedHosts...))
	}

	// Build once before listening, so an invalid option is refused before a
	// port is opened.
	handler, err := httpapi.New(d, opts...)
	if err != nil {
		return setup{}, fmt.Errorf("configure the API: %w", err)
	}

	ln, err := net.Listen("tcp", cfg.listen)
	if err != nil {
		return setup{}, fmt.Errorf("%w: %w", errListen, err)
	}

	// The name localhost is trusted only as far as the address it bound: a
	// resolver that maps it beyond loopback makes the listener non-loopback.
	loopback = loopbackBound(loopback, ln.Addr())

	if err := checkAuth(loopback, tok.set, cfg.insecureNoAuth, ln.Addr().String()); err != nil {
		_ = ln.Close()

		return setup{}, err
	}

	// On loopback without a token, and without an allow-list of the operator's,
	// the server answers only to its own loopback names (ADR 0019).
	if loopback && !tok.set && len(cfg.allowedHosts) == 0 {
		hosts, err := loopbackHosts(ln.Addr())
		if err == nil {
			handler, err = httpapi.New(d, append(opts, httpapi.WithAllowedHosts(hosts...))...)
		}

		if err != nil {
			_ = ln.Close()

			return setup{}, fmt.Errorf("configure the API: %w", err)
		}
	}

	var warnings []string

	if tok.warning != "" {
		warnings = append(warnings, tok.warning)
	}

	if !loopback && !tok.set {
		warnings = append(warnings, "serving without authentication on "+ln.Addr().String())
	}

	return setup{handler: handler, ln: ln, concurrency: concurrency, warnings: warnings}, nil
}

// checkAuth applies the non-loopback rule: a listener on addr that is not
// loopback needs a bearer token or --insecure-no-auth.
func checkAuth(loopback, hasToken, insecureNoAuth bool, addr string) error {
	if loopback || hasToken || insecureNoAuth {
		return nil
	}

	return fmt.Errorf("refusing to listen on the non-loopback address %s without a bearer token; "+
		"set %s or --token-file, or pass --insecure-no-auth", addr, tokenEnvVar)
}

// serve serves srv on ln until ctx is cancelled, then shuts it down: it stops
// accepting connections and waits up to shutdownTimeout for the requests in
// flight. It returns 0 after a graceful shutdown, and 1 when serving fails or
// the requests in flight outlast the deadline, which closes them.
func serve(ctx context.Context, srv *http.Server, ln net.Listener, stderr io.Writer) int {
	served := make(chan error, 1)

	go func() { served <- srv.Serve(ln) }()

	select {
	case err := <-served:
		printError(stderr, fmt.Errorf("serve: %w", err))

		return exitFailure
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		_ = srv.Close()

		printError(stderr, fmt.Errorf("shut down: the requests in flight did not finish within %s: %w", shutdownTimeout, err))

		return exitFailure
	}

	if err := <-served; err != nil && !errors.Is(err, http.ErrServerClosed) {
		printError(stderr, fmt.Errorf("serve: %w", err))

		return exitFailure
	}

	return exitOK
}

// printError writes err as one line, prefixed "shift2eu-server: ".
func printError(stderr io.Writer, err error) {
	_, _ = fmt.Fprintf(stderr, "shift2eu-server: %s\n", strings.ReplaceAll(err.Error(), "\n", " "))
}
