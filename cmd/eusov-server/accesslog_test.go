// White-box tests of the access log the composition root writes to stderr: the
// method, the path, the status, the duration and the size, and nothing of the
// request. An inventory describes someone's infrastructure (CLAUDE.md rule 2),
// so a log line that quotes a value, a query or a header would leak it to
// whatever collects the process's stderr (ADR 0019).
package main

import (
	"bytes"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// accessMarker is the value the test plants wherever a request carries content
// a log must not hold: an inventory ID, a resource type and a region, the query
// string, the Authorization header and another header.
const accessMarker = "m3accessmarker9f2"

// durationToken matches a duration as a log line writes it: a number with a unit
// time.Duration.String() produces, such as 12.3ms, 1.000123s or 250µs.
var durationToken = regexp.MustCompile(`\d+(?:\.\d+)?(?:ns|µs|μs|us|ms|s)\b`)

// TestServerAccessLog pins the access log of an assessment: one line per
// request holding the method, the path, the status, a duration and the size of
// the answer, and no value of the request at all, neither from the body (an
// inventory ID, a resource type, a region) nor from the query string nor from a
// header.
//
// The assertion on the size is the answer's body length, which is what a reader
// of the log needs to tell a rendered report from an error document; the log
// line must carry it as a value of its own.
func TestServerAccessLog(t *testing.T) {
	t.Parallel()

	// The shared fixture is the base, with the marker in place of an ID, a
	// resource type and a region, so the report the request renders holds it and
	// a log that wrote any of the report's content would show it.
	document := []byte(strings.NewReplacer(
		"partial-example", accessMarker+"-id",
		"AWS::EC2::Instance", accessMarker+"-type",
		"eu-west-1", accessMarker+"-region",
	).Replace(string(fixtureInventory(t))))

	s := startServer(t, loopbackListen, nil)

	req := newRequest(t, http.MethodPost, "/v1/assessments?probe="+accessMarker, assessRequest(t, document, "ovhcloud"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessMarker+"-authorization")
	req.Header.Set("X-Probe", accessMarker+"-header")

	got := s.do(t, req)

	if got.status != http.StatusOK {
		t.Fatalf("POST /v1/assessments = %d, want %d; body: %q; stderr = %q",
			got.status, http.StatusOK, got.body, s.stderr.String())
	}

	// The marker reached the report, so the log had something to leak and this
	// test cannot pass on an assessment that echoed nothing.
	if !bytes.Contains(got.body, []byte(accessMarker)) {
		t.Fatalf("the report does not hold the marker %q, so the log had nothing to leak:\n%s", accessMarker, got.body)
	}

	// Read the log before the server is stopped: the shutdown may write a line
	// of its own, which is no access log line.
	logged := s.stderr.String()

	if strings.Contains(logged, accessMarker) {
		t.Errorf("the log holds the marker %q, so it quotes the request:\n%s", accessMarker, logged)
	}

	accessLines := startLines(logged)

	if len(accessLines) == 0 {
		t.Fatalf("the log holds no access log line for the request; stderr = %q", logged)
	}

	for _, line := range accessLines {
		if !holdsValue(line, http.MethodPost) {
			t.Errorf("the access log line %q does not hold the method %s", line, http.MethodPost)
		}

		if !strings.Contains(line, "/v1/assessments") {
			t.Errorf("the access log line %q does not hold the path", line)
		}

		if !holdsValue(line, strconv.Itoa(got.status)) {
			t.Errorf("the access log line %q does not hold the status %d", line, got.status)
		}

		if !durationToken.MatchString(line) {
			t.Errorf("the access log line %q does not hold a duration", line)
		}

		if !holdsValue(line, strconv.Itoa(len(got.body))) {
			t.Errorf("the access log line %q does not hold the size %d of the answer", line, len(got.body))
		}

		// The path alone: the query string carries request content.
		if strings.Contains(line, "?") {
			t.Errorf("the access log line %q holds a query string, want the path only", line)
		}
	}

	// A second request adds a second line, so the log is per request rather than
	// one line for the run.
	if status, _ := s.get(t, "/healthz"); status != http.StatusOK {
		t.Errorf("GET /healthz = %d, want %d", status, http.StatusOK)
	}

	if got, want := len(startLines(s.stderr.String())), len(accessLines)+1; got != want {
		t.Errorf("the log holds %d access log lines after two requests, want %d:\n%s",
			got, want, s.stderr.String())
	}

	if code := s.stop(t); code != 0 {
		t.Errorf("run returned %d after a cancelled context, want 0; stderr = %q", code, s.stderr.String())
	}
}

// startLines returns the lines of stderr that are no startup line: the access
// log lines. The startup line is the one that announces the address, which a
// test needs before it can send a request.
func startLines(stderr string) []string {
	var lines []string

	for _, line := range strings.Split(stderr, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		if _, announced, ok := strings.Cut(line, "listening on "); ok && announced != "" {
			continue
		}

		lines = append(lines, line)
	}

	return lines
}

// holdsValue reports whether line holds want as a value of its own: a field
// alone, or one behind a key and a separator, so "status=200", `"status":200`,
// "size 12" and "12" all hold their value.
func holdsValue(line, want string) bool {
	for _, field := range strings.Fields(line) {
		trimmed := strings.Trim(field, `"',[]{}() `)
		if trimmed == want {
			return true
		}

		for _, separator := range []string{"=", ":"} {
			if _, value, ok := strings.Cut(trimmed, separator); ok && strings.Trim(value, `"',`) == want {
				return true
			}
		}
	}

	return false
}
