package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/timothymamo/shift2eu/internal/app/analysis"
	"github.com/timothymamo/shift2eu/internal/domain/inventory"
)

// allTargets is the target value that selects every supported target.
const allTargets = "all"

// inventoryName is the name a report gives the inventory: a request body
// carries no file name (analysis.ReportMeta).
const inventoryName = "inventory.json"

// The media types of the two representations of a report.
const (
	jsonMediaType     = "application/json"
	markdownMediaType = "text/markdown; charset=utf-8"
)

// assess serves POST /v1/assessments: it checks the media types, takes a slot,
// reads and decodes the body, validates the targets, runs the analysis or the
// comparison into memory and only then writes the report.
func (a *api) assess(w http.ResponseWriter, r *http.Request) {
	if !isJSONRequest(r) {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type",
			"the request body must be application/json in utf-8")

		return
	}

	format, mediaType, ok := a.negotiate(r)
	if !ok {
		writeError(w, http.StatusNotAcceptable, "not_acceptable",
			"the Accept header allows neither application/json nor text/markdown")

		return
	}

	select {
	case a.slots <- struct{}{}:
		defer func() { <-a.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusServiceUnavailable, "busy", "too many assessments are running; retry later")

		return
	}

	body, status := a.readBody(w, r)
	if status != 0 {
		if status == http.StatusRequestEntityTooLarge {
			writeError(w, status, "payload_too_large",
				"the request body is larger than "+strconv.FormatInt(a.cfg.maxBodyBytes, 10)+" bytes")
		} else {
			writeError(w, status, "invalid_request", "the request body could not be read")
		}

		return
	}

	doc, keys, err := a.deps.DecodeRequest(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", escape(err.Error()))

		return
	}

	selected, err := a.selectTargets(keys)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_targets", err.Error())

		return
	}

	report, err := a.run(r.Context(), doc, selected, format)
	if err != nil {
		var invalid *inventoryError
		if errors.As(err, &invalid) && r.Context().Err() == nil {
			writeError(w, http.StatusBadRequest, "invalid_inventory", escape(invalid.err.Error()))

			return
		}

		writeInternal(w)

		return
	}

	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Length", strconv.Itoa(len(report)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(report) // A client gone mid-write can be told nothing.
}

// isJSONRequest reports whether the request's Content-Type is
// application/json, with no charset parameter or charset utf-8 (compared
// ignoring case): the body is decoded as UTF-8 only.
func isJSONRequest(r *http.Request) bool {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != jsonMediaType {
		return false
	}

	charset, ok := params["charset"]

	return !ok || strings.EqualFold(charset, "utf-8")
}

// readBody reads the whole body, up to the cap and within the body read
// timeout. It returns a status other than 0 when it cannot: 413 for a body
// over the cap, 400 for one that cannot be read in time or at all.
//
// The cap is http.MaxBytesReader's, which counts the bytes read, so a chunked
// body or one that states a smaller ContentLength is refused too; a stated
// ContentLength over the cap only saves the read. The read deadline is set on
// the server's connection through http.ResponseController; a writer that
// cannot take one (a test recorder) reads without it. A body read in full
// clears the deadline. A failed read leaves it, and asks the server to close
// the connection: the server drains an unread body after the handler, which
// must not wait on a client that stalled.
func (a *api) readBody(w http.ResponseWriter, r *http.Request) ([]byte, int) {
	if r.ContentLength > a.cfg.maxBodyBytes {
		return nil, http.StatusRequestEntityTooLarge
	}

	controller := http.NewResponseController(w)
	deadline := controller.SetReadDeadline(time.Now().Add(a.cfg.bodyReadTimeout)) == nil

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, a.cfg.maxBodyBytes))
	if err == nil && deadline {
		_ = controller.SetReadDeadline(time.Time{})
	}

	if err != nil {
		w.Header().Set("Connection", "close")

		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, http.StatusRequestEntityTooLarge
		}

		return nil, http.StatusBadRequest
	}

	return body, 0
}

// selectTargets applies the CLI's target rules to keys: one or more distinct,
// supported, non-empty keys, or "all" alone for every supported key. It
// returns the selected keys sorted. An error message names a bad key by its
// position, never by its value, and ends with the supported keys.
func (a *api) selectTargets(keys []string) ([]string, error) {
	supported := "supported targets: " + strings.Join(a.keys, ", ")

	if len(keys) == 0 {
		return nil, errors.New("no target given; " + supported)
	}

	if len(keys) == 1 && keys[0] == allTargets {
		return slices.Clone(a.keys), nil
	}

	selected := make([]string, 0, len(keys))

	for i, key := range keys {
		switch {
		case key == "":
			return nil, fmt.Errorf("the target at index %d is empty; %s", i, supported)
		case key == allTargets:
			return nil, fmt.Errorf("%q cannot be combined with other targets; %s", allTargets, supported)
		case slices.Contains(selected, key):
			return nil, fmt.Errorf("the target at index %d is given more than once; %s", i, supported)
		}

		if _, ok := a.deps.Targets[key]; !ok {
			return nil, fmt.Errorf("the target at index %d is not supported; %s", i, supported)
		}

		selected = append(selected, key)
	}

	slices.Sort(selected)

	return selected, nil
}

// run builds the use case for the selected targets, one target's analysis or
// a comparison of several, runs it and returns the whole report. An error the
// inventory source returns is an *inventoryError.
func (a *api) run(ctx context.Context, doc []byte, keys []string, format Format) ([]byte, error) {
	source := inventorySource{inner: a.deps.NewInventorySource(doc)}
	meta := analysis.ReportMeta{InventoryName: inventoryName}

	var report bytes.Buffer

	if len(keys) == 1 {
		single, err := analysis.New(source, a.deps.Targets[keys[0]](), format.NewReporter())
		if err != nil {
			return nil, err
		}

		if err := single.Run(ctx, &report, meta); err != nil {
			return nil, err
		}

		return report.Bytes(), nil
	}

	targets := make([]analysis.Target, 0, len(keys))
	for _, key := range keys {
		targets = append(targets, analysis.Target{Key: key, Catalog: a.deps.Targets[key]()})
	}

	comparison, err := analysis.NewComparison(source, targets, format.NewComparisonReporter())
	if err != nil {
		return nil, err
	}

	if err := comparison.Run(ctx, &report, meta); err != nil {
		return nil, err
	}

	return report.Bytes(), nil
}

// inventorySource marks every error of the source it wraps as an
// *inventoryError, so the handler tells a refused inventory (400) from any
// other failure (500).
type inventorySource struct {
	inner analysis.InventorySource
}

func (s inventorySource) Inventory(ctx context.Context) (inventory.Inventory, error) {
	inv, err := s.inner.Inventory(ctx)
	if err != nil {
		return inventory.Inventory{}, &inventoryError{err: err}
	}

	return inv, nil
}

// inventoryError is an error of the inventory source.
type inventoryError struct {
	err error
}

func (e *inventoryError) Error() string { return e.err.Error() }

func (e *inventoryError) Unwrap() error { return e.err }

// negotiate picks the representation the Accept header prefers: the format,
// its Content-Type, and false when neither is acceptable. JSON wins a tie, and
// is the answer without an Accept header.
func (a *api) negotiate(r *http.Request) (Format, string, bool) {
	accept := strings.Join(r.Header.Values("Accept"), ",")
	if strings.TrimSpace(accept) == "" {
		return a.deps.JSON, jsonMediaType, true
	}

	ranges := parseAccept(accept)
	jsonQ := quality(ranges, "application", "json")
	markdownQ := quality(ranges, "text", "markdown")

	switch {
	case jsonQ > 0 && jsonQ >= markdownQ:
		return a.deps.JSON, jsonMediaType, true
	case markdownQ > 0:
		return a.deps.Markdown, markdownMediaType, true
	default:
		return Format{}, "", false
	}
}

// mediaRange is one entry of an Accept header.
type mediaRange struct {
	typ, subtype string
	q            float64
}

// parseAccept parses an Accept header leniently: an entry that is not a media
// range, or has an invalid q (not a number in [0, 1], NaN included), is
// skipped.
func parseAccept(accept string) []mediaRange {
	var ranges []mediaRange

	for entry := range strings.SplitSeq(accept, ",") {
		params := strings.Split(entry, ";")

		typ, subtype, ok := strings.Cut(strings.ToLower(strings.TrimSpace(params[0])), "/")
		if !ok || typ == "" || subtype == "" || (typ == "*" && subtype != "*") {
			continue
		}

		q, valid := 1.0, true

		for _, param := range params[1:] {
			name, value, _ := strings.Cut(strings.TrimSpace(param), "=")
			if !strings.EqualFold(strings.TrimSpace(name), "q") {
				continue
			}

			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			// Written so that NaN, which ParseFloat accepts, fails it too.
			if err != nil || !(parsed >= 0 && parsed <= 1) {
				valid = false

				break
			}

			q = parsed
		}

		if valid {
			ranges = append(ranges, mediaRange{typ: typ, subtype: subtype, q: q})
		}
	}

	return ranges
}

// quality is the q the most specific range matching typ/subtype gives it: an
// exact match beats typ/*, which beats */*. It is 0 when nothing matches.
func quality(ranges []mediaRange, typ, subtype string) float64 {
	best, specificity := 0.0, 0

	for _, mr := range ranges {
		var s int

		switch {
		case mr.typ == typ && mr.subtype == subtype:
			s = 3
		case mr.typ == typ && mr.subtype == "*":
			s = 2
		case mr.typ == "*" && mr.subtype == "*":
			s = 1
		default:
			continue
		}

		if s > specificity || (s == specificity && mr.q > best) {
			best, specificity = mr.q, s
		}
	}

	return best
}
