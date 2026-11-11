package httpapi

import (
	"context"
	"crypto/subtle"
	"errors"
	"maps"
	"net/http"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/timothymamo/eu-sov-cloud/internal/app/analysis"
)

// Deps are the adapters the API runs with. The composition root
// (cmd/eusov-server) builds them; the API never constructs an adapter itself.
// New refuses a Deps that lacks any of them.
type Deps struct {
	// DecodeRequest strictly decodes the body of POST /v1/assessments into the
	// inventory document, as raw bytes, and the target keys in the order sent
	// (assessmentrequest.Decode).
	DecodeRequest func(body []byte) (inventory []byte, targets []string, err error)
	// NewInventorySource returns the inventory source over one inventory
	// document (inventoryfile.FromBytes).
	NewInventorySource func(doc []byte) analysis.InventorySource
	// Targets maps every supported target key to the constructor of its catalog
	// source, as cli.Deps.Targets does.
	Targets map[string]func() analysis.CatalogSource
	// JSON is the representation application/json selects: the JSON report
	// (ADR 0018).
	JSON Format
	// Markdown is the representation text/markdown selects: the Markdown
	// report.
	Markdown Format
}

// Format is one representation of a report: the constructors of its
// reporters. A request builds only the one it needs.
type Format struct {
	// NewReporter returns the reporter of an assessment against one target.
	NewReporter func() analysis.Reporter
	// NewComparisonReporter returns the reporter of a comparison of several
	// targets.
	NewComparisonReporter func() analysis.ComparisonReporter
}

// Option configures the handler New returns.
type Option func(*config)

// defaultMaxBodyBytes is the body cap without WithMaxBodyBytes: the inventory
// reader's 10 MiB cap plus 64 KiB for the request envelope, the value of
// assessmentrequest.MaxBodySize, which this package may not import.
const defaultMaxBodyBytes = 10<<20 + 64<<10

// config is what the options set.
type config struct {
	maxBodyBytes  int64
	maxConcurrent int
	token         string
	origins       map[string]bool
	readiness     func(context.Context) error
}

// WithMaxBodyBytes caps the body of POST /v1/assessments at n bytes; a larger
// body is 413 payload_too_large. A value below 1 keeps the default, the
// inventory reader's cap plus 64 KiB.
func WithMaxBodyBytes(n int64) Option {
	return func(c *config) {
		if n > 0 {
			c.maxBodyBytes = n
		}
	}
}

// WithMaxConcurrent sets how many assessments run at once; one more is refused
// with 503 busy. The default is runtime.NumCPU(); a value below 1 means 1.
func WithMaxConcurrent(n int) Option {
	return func(c *config) {
		c.maxConcurrent = max(n, 1)
	}
}

// WithBearerToken requires Authorization: Bearer <token> on every /v1/
// request. An empty token turns authentication off, the default.
func WithBearerToken(token string) Option {
	return func(c *config) {
		c.token = token
	}
}

// WithAllowedOrigins turns CORS on for exactly these origins. Without it, or
// with no non-empty origin, CORS is off.
func WithAllowedOrigins(origins ...string) Option {
	return func(c *config) {
		for _, origin := range origins {
			if origin != "" {
				c.origins[origin] = true
			}
		}
	}
}

// WithReadiness sets the check GET /readyz runs: nil from it is ready, an
// error is 503 not_ready. Without it the API is always ready.
func WithReadiness(check func(context.Context) error) Option {
	return func(c *config) {
		c.readiness = check
	}
}

// errIncompleteDeps is returned by New for a Deps that lacks an adapter: a
// wiring mistake in the composition root.
var errIncompleteDeps = errors.New("httpapi: incomplete deps")

// route is one path the API serves: its one method and its handler.
type route struct {
	method  string
	handler func(w http.ResponseWriter, r *http.Request)
	// versioned is true for a /v1/ route: the bearer token and CORS apply.
	versioned bool
}

// api is the handler New returns.
type api struct {
	deps   Deps
	cfg    config
	keys   []string
	slots  chan struct{}
	routes map[string]route
}

// New returns the handler of the API over deps, configured by opts. It returns
// an error, and no handler, when deps lacks any field or has no target.
func New(deps Deps, opts ...Option) (http.Handler, error) {
	if err := checkDeps(deps); err != nil {
		return nil, err
	}

	cfg := config{
		maxBodyBytes:  defaultMaxBodyBytes,
		maxConcurrent: max(runtime.NumCPU(), 1),
		origins:       make(map[string]bool),
	}

	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	a := &api{
		deps:  deps,
		cfg:   cfg,
		keys:  slices.Sorted(maps.Keys(deps.Targets)),
		slots: make(chan struct{}, cfg.maxConcurrent),
	}

	a.routes = map[string]route{
		"/v1/assessments": {method: http.MethodPost, handler: a.assess, versioned: true},
		"/v1/targets":     {method: http.MethodGet, handler: a.targets, versioned: true},
		"/healthz":        {method: http.MethodGet, handler: a.healthz},
		"/readyz":         {method: http.MethodGet, handler: a.readyz},
	}

	return a, nil
}

// checkDeps reports the first field deps lacks.
func checkDeps(deps Deps) error {
	missing := ""

	switch {
	case deps.DecodeRequest == nil:
		missing = "DecodeRequest"
	case deps.NewInventorySource == nil:
		missing = "NewInventorySource"
	case len(deps.Targets) == 0:
		missing = "Targets"
	case deps.JSON.NewReporter == nil:
		missing = "JSON.NewReporter"
	case deps.JSON.NewComparisonReporter == nil:
		missing = "JSON.NewComparisonReporter"
	case deps.Markdown.NewReporter == nil:
		missing = "Markdown.NewReporter"
	case deps.Markdown.NewComparisonReporter == nil:
		missing = "Markdown.NewComparisonReporter"
	}

	if missing != "" {
		return errors.New(errIncompleteDeps.Error() + ": no " + missing)
	}

	for key, newCatalog := range deps.Targets {
		if key == "" || key == allTargets || newCatalog == nil {
			return errors.New(errIncompleteDeps.Error() + ": a target with an empty or reserved key or no constructor")
		}
	}

	return nil
}

// ServeHTTP answers one request: the hardening headers, CORS, routing, the
// bearer token, then the route's handler, with any panic answered as 500.
func (a *api) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	w := &recordingWriter{ResponseWriter: rw}

	header := w.Header()
	header.Set("Cache-Control", "no-store")
	header.Set("X-Content-Type-Options", "nosniff")

	defer func() {
		if recovered := recover(); recovered != nil {
			// http.ErrAbortHandler asks the server to abort the response: let it.
			if err, ok := recovered.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(recovered)
			}

			if !w.wroteHeader {
				writeInternal(w)
			}
		}
	}()

	rt, known := a.routes[r.URL.Path]
	if !known {
		writeError(w, http.StatusNotFound, "not_found", "no such resource")

		return
	}

	allowedOrigin := rt.versioned && a.originAllowed(r)
	if allowedOrigin {
		header.Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		header.Add("Vary", "Origin")

		if r.Method == http.MethodOptions {
			header.Set("Access-Control-Allow-Methods", rt.method)
			header.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept")
			header.Set("Access-Control-Max-Age", strconv.Itoa(preflightMaxAge))
			w.WriteHeader(http.StatusNoContent)

			return
		}
	}

	if r.Method != rt.method {
		header.Set("Allow", rt.method)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "the method is not allowed on this resource")

		return
	}

	if rt.versioned && !a.authorized(r) {
		header.Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "unauthorized", "a valid bearer token is required")

		return
	}

	rt.handler(w, r)
}

// preflightMaxAge is how many seconds a browser may cache a preflight answer.
const preflightMaxAge = 600

// originAllowed reports whether r carries an Origin exactly in the configured
// list.
func (a *api) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")

	return origin != "" && a.cfg.origins[origin]
}

// bearerPrefix is the scheme of the Authorization header, with its separator.
const bearerPrefix = "Bearer "

// authorized reports whether r may use a /v1/ route: always without a token,
// otherwise only with Authorization: Bearer <token>, compared in constant time.
func (a *api) authorized(r *http.Request) bool {
	if a.cfg.token == "" {
		return true
	}

	value := r.Header.Get("Authorization")
	if len(value) < len(bearerPrefix) || !strings.EqualFold(value[:len(bearerPrefix)], bearerPrefix) {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(value[len(bearerPrefix):]), []byte(a.cfg.token)) == 1
}

// recordingWriter remembers whether the header was written, so a recovered
// panic answers 500 only when nothing was sent yet.
type recordingWriter struct {
	http.ResponseWriter

	wroteHeader bool
}

func (w *recordingWriter) WriteHeader(status int) {
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	w.wroteHeader = true

	return w.ResponseWriter.Write(p)
}

// Unwrap gives http.ResponseController and http.MaxBytesReader the writer of
// the server.
func (w *recordingWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
