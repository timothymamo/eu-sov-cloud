package lint_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestDomainIsPureProbe runs forbidigo with the repository's .golangci.yml on
// a probe domain package (testdata/domainispure/mod). A line that ends in
// // want "domain-is-pure: ..." must be refused by the domain-is-pure entry
// whose msg starts so, and no other line may be. The probe has a line for
// every name an entry's pattern lists, and for a method value, a func value,
// a method expression and an aliased import. fmt.Errorf, the pure formatting
// and parsing calls, the time.Time type and a domain function named println
// pass, and the refused calls in a domain _test.go file and in a CLI file are
// not reported.
func TestDomainIsPureProbe(t *testing.T) {
	t.Parallel()

	runForbidigoProbe(t, forbidigoProbe{dir: "domainispure", tags: []string{"domain-is-pure"}})
}

// domainName is a name as forbidigo renders it, with the import path its pkg
// is matched against.
type domainName struct{ pkg, name string }

// refusedName is a name with the msg prefix of the entry that must refuse it.
type refusedName struct {
	domainName
	entry string
}

// domainRefused lists each name the domain-is-pure group refuses with the msg
// prefix of the entry that refuses it. A method is rendered
// <package>.<type>.<method>, with a pointer receiver dropped, and a builtin
// with no package.
var domainRefused = []refusedName{
	{domainName{"fmt", "fmt.Print"}, "domain-is-pure: fmt.Print*"},
	{domainName{"fmt", "fmt.Printf"}, "domain-is-pure: fmt.Print*"},
	{domainName{"fmt", "fmt.Println"}, "domain-is-pure: fmt.Print*"},
	{domainName{"fmt", "fmt.Scan"}, "domain-is-pure: fmt.Scan*"},
	{domainName{"fmt", "fmt.Scanf"}, "domain-is-pure: fmt.Scan*"},
	{domainName{"fmt", "fmt.Scanln"}, "domain-is-pure: fmt.Scan*"},
	{domainName{"", "print"}, "domain-is-pure: print and println"},
	{domainName{"", "println"}, "domain-is-pure: print and println"},
	{domainName{"time", "time.Now"}, "domain-is-pure: the domain reads no clock"},
	{domainName{"time", "time.Since"}, "domain-is-pure: the domain reads no clock"},
	{domainName{"time", "time.Until"}, "domain-is-pure: the domain reads no clock"},
	{domainName{"time", "time.Sleep"}, "domain-is-pure: the domain reads no clock"},
	{domainName{"time", "time.After"}, "domain-is-pure: the domain reads no clock"},
	{domainName{"time", "time.Tick"}, "domain-is-pure: the domain reads no clock"},
	{domainName{"time", "time.NewTimer"}, "domain-is-pure: the domain reads no clock"},
	{domainName{"time", "time.NewTicker"}, "domain-is-pure: the domain reads no clock"},
	{domainName{"time", "time.AfterFunc"}, "domain-is-pure: the domain reads no clock"},
	{domainName{"time", "time.Timer"}, "domain-is-pure: a time.Timer or time.Ticker"},
	{domainName{"time", "time.Timer.Stop"}, "domain-is-pure: a time.Timer or time.Ticker"},
	{domainName{"time", "time.Timer.Reset"}, "domain-is-pure: a time.Timer or time.Ticker"},
	{domainName{"time", "time.Timer.C"}, "domain-is-pure: a time.Timer or time.Ticker"},
	{domainName{"time", "time.Ticker"}, "domain-is-pure: a time.Timer or time.Ticker"},
	{domainName{"time", "time.Ticker.Stop"}, "domain-is-pure: a time.Timer or time.Ticker"},
	{domainName{"time", "time.Ticker.Reset"}, "domain-is-pure: a time.Timer or time.Ticker"},
	{domainName{"time", "time.Ticker.C"}, "domain-is-pure: a time.Timer or time.Ticker"},
	{domainName{"time", "time.LoadLocation"}, "domain-is-pure: time.LoadLocation"},
	{domainName{"time", "time.Local"}, "domain-is-pure: time.Local reads"},
	{domainName{"time", "time.Time.Local"}, "domain-is-pure: time.Local reads"},
	{domainName{"time", "time.Unix"}, "domain-is-pure: time.Unix*"},
	{domainName{"time", "time.UnixMilli"}, "domain-is-pure: time.Unix*"},
	{domainName{"time", "time.UnixMicro"}, "domain-is-pure: time.Unix*"},
	{domainName{"time", "time.Parse"}, "domain-is-pure: this matches a zone offset"},
	{domainName{"time", "time.Time.UnmarshalJSON"}, "domain-is-pure: this matches a zone offset"},
	{domainName{"time", "time.Time.UnmarshalText"}, "domain-is-pure: this matches a zone offset"},
	{domainName{"time", "time.Time.UnmarshalBinary"}, "domain-is-pure: this matches a zone offset"},
	{domainName{"time", "time.Time.GobDecode"}, "domain-is-pure: this matches a zone offset"},
}

// domainAllowed are names the domain needs, which no domain-is-pure entry may
// refuse: they take or return the data, or carry an explicit location. The
// names that only extend a refused one (time.Timers, time.Nowhere) catch a
// pattern that lost its $.
var domainAllowed = []domainName{
	{"fmt", "fmt.Errorf"},
	{"fmt", "fmt.Sprint"},
	{"fmt", "fmt.Sprintf"},
	{"fmt", "fmt.Sprintln"},
	{"fmt", "fmt.Sscan"},
	{"fmt", "fmt.Sscanf"},
	{"fmt", "fmt.Sscanln"},
	{"fmt", "fmt.Fscan"},
	{"fmt", "fmt.Fscanf"},
	{"fmt", "fmt.Fscanln"},
	{"fmt", "fmt.Fprint"},
	{"fmt", "fmt.Fprintf"},
	{"fmt", "fmt.Fprintln"},
	{"fmt", "fmt.Stringer"},
	{"fmt", "fmt.Printer"},
	{"fmt", "fmt.Scanner"},
	{"time", "time.ParseInLocation"},
	{"time", "time.FixedZone"},
	{"time", "time.Date"},
	{"time", "time.UTC"},
	{"time", "time.Time"},
	{"time", "time.Duration"},
	{"time", "time.Location"},
	{"time", "time.ParseDuration"},
	{"time", "time.RFC3339"},
	{"time", "time.Month"},
	{"time", "time.Time.Unix"},
	{"time", "time.Time.UnixMilli"},
	{"time", "time.Time.UnixMicro"},
	{"time", "time.Time.Format"},
	{"time", "time.Time.UTC"},
	{"time", "time.Time.In"},
	{"time", "time.Time.MarshalJSON"},
	{"time", "time.Time.Add"},
	{"time", "time.Time.Sub"},
	{"time", "time.Timers"},
	{"time", "time.Tickers"},
	{"time", "time.Nowhere"},
	{"time", "time.Unixish"},
	{"time", "time.Parser"},
	{"time", "time.Locale"},
	// A domain function named println has a package, so the builtins' entry
	// (pkg ^$) does not refuse it.
	{"github.com/timothymamo/shift2eu/internal/domain/shadow", "println"},
	{"github.com/timothymamo/shift2eu/internal/domain/shadow", "shadow.println"},
}

// TestDomainIsPurePatterns checks the entries of forbidigo's domain-is-pure
// group, read from .golangci.yml, offline and without golangci-lint: each name
// the group must refuse is refused by the entry that should refuse it, no name
// the domain needs is refused by any, and every entry refuses a listed name.
// The probe proves how forbidigo renders a name; this pins every alternative
// of every pattern, so narrowing one fails without golangci-lint.
func TestDomainIsPurePatterns(t *testing.T) {
	t.Parallel()

	const tag = "domain-is-pure: "
	type compiled struct {
		pattern, pkg *regexp.Regexp
		msg          string
	}
	var entries []compiled
	for _, e := range forbidigoEntries(t, moduleRoot(t)) {
		if !strings.HasPrefix(e.msg, tag) {
			continue
		}
		pattern, err := regexp.Compile(e.pattern)
		if err != nil {
			t.Fatalf(".golangci.yml:%d: pattern: %v", e.line, err)
		}
		pkg, err := regexp.Compile(e.pkg)
		if err != nil {
			t.Fatalf(".golangci.yml:%d: pkg: %v", e.line, err)
		}
		entries = append(entries, compiled{pattern, pkg, e.msg})
	}
	if len(entries) == 0 {
		t.Fatal(".golangci.yml holds no domain-is-pure entry")
	}
	refusedBy := func(n domainName) []string {
		var msgs []string
		for _, e := range entries {
			if e.pkg.MatchString(n.pkg) && e.pattern.MatchString(n.name) {
				msgs = append(msgs, e.msg)
			}
		}
		return msgs
	}

	for _, r := range domainRefused {
		msgs := refusedBy(r.domainName)
		if !slices.ContainsFunc(msgs, func(m string) bool { return strings.HasPrefix(m, r.entry) }) {
			t.Errorf("%s (pkg %q) must be refused by the entry with msg %q; refused by %q", r.name, r.pkg, r.entry, msgs)
		}
	}
	for _, n := range domainAllowed {
		if msgs := refusedBy(n); len(msgs) > 0 {
			t.Errorf("%s (pkg %q) is allowed in the domain, but %q refuses it", n.name, n.pkg, msgs)
		}
	}
	for _, e := range entries {
		refuses := func(r refusedName) bool { return e.pkg.MatchString(r.pkg) && e.pattern.MatchString(r.name) }
		if !slices.ContainsFunc(domainRefused, refuses) {
			t.Errorf("the entry with msg %q refuses no name in domainRefused; add the names it refuses", e.msg)
		}
	}
}
