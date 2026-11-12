package main

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// lockedWriter serialises the writes of the access log and the startup lines,
// so two requests that end at once never interleave their lines.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// Write writes p as one call to the underlying writer.
func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.w.Write(p)
}

// accessLog wraps next so that every request writes one line to log once it is
// answered:
//
//	eusov-server: access method=POST path="/v1/assessments" status=200 duration=12.3ms size=1234
//
// The line holds the method, the path, the status, the duration and the size
// of the answer's body, and nothing else: never the query string, a header, the
// body, an inventory value or the report (ADR 0019). The path is quoted, so a
// path that holds a line feed or a control character cannot forge a line.
func accessLog(next http.Handler, log io.Writer, now func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := now()
		rec := &statusRecorder{ResponseWriter: w}

		defer func() {
			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}

			_, _ = fmt.Fprintf(log, "eusov-server: access method=%s path=%s status=%d duration=%s size=%d\n",
				r.Method, strconv.Quote(r.URL.Path), status, now().Sub(start), rec.size)
		}()

		next.ServeHTTP(rec, r)
	})
}

// statusRecorder remembers the status and counts the body bytes a handler
// writes.
type statusRecorder struct {
	http.ResponseWriter

	status int
	size   int64
}

// WriteHeader records the first status written and passes it on.
func (w *statusRecorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}

	w.ResponseWriter.WriteHeader(status)
}

// Write counts the bytes written; a write before WriteHeader is a 200.
func (w *statusRecorder) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}

	n, err := w.ResponseWriter.Write(p)
	w.size += int64(n)

	return n, err
}

// Unwrap gives http.ResponseController, through which the API sets the body's
// read deadline, and http.MaxBytesReader the writer of the server.
func (w *statusRecorder) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
