// Package probe is a domain package for TestDomainIsPureProbe. Each line that
// ends in a want comment holds a name that forbidigo's domain-is-pure group
// refuses in a domain production file, and the comment names the start of
// the msg of the entry that refuses it. There is a line for every name an
// entry's pattern lists, so narrowing a pattern fails the probe. Every other
// line must pass.
package probe

import (
	"fmt"
	"time"
)

// The entries' msg prefixes, as the want comments spell them:
//
//	"domain-is-pure: fmt.Print*"
//	"domain-is-pure: fmt.Scan*"
//	"domain-is-pure: print and println"
//	"domain-is-pure: the domain reads no clock"
//	"domain-is-pure: a time.Timer or time.Ticker"
//	"domain-is-pure: time.LoadLocation"
//	"domain-is-pure: time.Local reads"
//	"domain-is-pure: time.Unix*"
//	"domain-is-pure: this matches a zone offset"

// Stdio prints to stdout and stderr and reads stdin.
func Stdio() error {
	fmt.Print("probe")          // want "domain-is-pure: fmt.Print*"
	fmt.Printf("%s\n", "probe") // want "domain-is-pure: fmt.Print*"
	fmt.Println("probe")        // want "domain-is-pure: fmt.Print*"
	var s string
	if _, err := fmt.Scan(&s); err != nil { // want "domain-is-pure: fmt.Scan*"
		return err
	}
	if _, err := fmt.Scanf("%s", &s); err != nil { // want "domain-is-pure: fmt.Scan*"
		return err
	}
	if _, err := fmt.Scanln(&s); err != nil { // want "domain-is-pure: fmt.Scan*"
		return err
	}
	print("probe")   // want "domain-is-pure: print and println"
	println("probe") // want "domain-is-pure: print and println"
	return nil
}

// Clock reads and waits on the clock.
func Clock(d time.Duration) {
	now := time.Now()          // want "domain-is-pure: the domain reads no clock"
	_ = time.Since(now)        // want "domain-is-pure: the domain reads no clock"
	_ = time.Until(now)        // want "domain-is-pure: the domain reads no clock"
	time.Sleep(d)              // want "domain-is-pure: the domain reads no clock"
	_ = time.After(d)          // want "domain-is-pure: the domain reads no clock"
	_ = time.Tick(d)           // want "domain-is-pure: the domain reads no clock"
	_ = time.NewTimer(d)       // want "domain-is-pure: the domain reads no clock"
	_ = time.NewTicker(d)      // want "domain-is-pure: the domain reads no clock"
	_ = time.AfterFunc(d, nil) // want "domain-is-pure: the domain reads no clock"
	f := time.Now              // want "domain-is-pure: the domain reads no clock"
	_ = f
}

// Timers stops, resets and reads timers and tickers handed in. Their
// methods and their field C are refused, and so is naming either type.
func Timers(
	tr *time.Timer, // want "domain-is-pure: a time.Timer or time.Ticker"
	tk *time.Ticker, // want "domain-is-pure: a time.Timer or time.Ticker"
	d time.Duration,
) {
	_ = tr.Stop()   // want "domain-is-pure: a time.Timer or time.Ticker"
	_ = tr.Reset(d) // want "domain-is-pure: a time.Timer or time.Ticker"
	tk.Stop()       // want "domain-is-pure: a time.Timer or time.Ticker"
	tk.Reset(d)     // want "domain-is-pure: a time.Timer or time.Ticker"
	stop := tr.Stop // want "domain-is-pure: a time.Timer or time.Ticker"
	_ = stop
	<-tr.C // want "domain-is-pure: a time.Timer or time.Ticker"
	<-tk.C // want "domain-is-pure: a time.Timer or time.Ticker"
}

// Zones reads zoneinfo files, TZ and /etc/localtime.
func Zones(t time.Time, data []byte) error {
	loc, err := time.LoadLocation("Europe/Paris") // want "domain-is-pure: time.LoadLocation"
	if err != nil {
		return err
	}
	_ = t.In(time.Local)                  // want "domain-is-pure: time.Local reads"
	_ = t.Local()                         // want "domain-is-pure: time.Local reads"
	_ = t.Local                           // want "domain-is-pure: time.Local reads"
	_ = time.Time.Local                   // want "domain-is-pure: time.Local reads"
	_ = time.Unix(0, 0)                   // want "domain-is-pure: time.Unix*"
	_ = time.UnixMilli(0)                 // want "domain-is-pure: time.Unix*"
	_ = time.UnixMicro(0)                 // want "domain-is-pure: time.Unix*"
	_, _ = time.Parse("", "")             // want "domain-is-pure: this matches a zone offset"
	_ = t.UnmarshalJSON(data)             // want "domain-is-pure: this matches a zone offset"
	_ = t.UnmarshalText(data)             // want "domain-is-pure: this matches a zone offset"
	_ = t.UnmarshalBinary(data)           // want "domain-is-pure: this matches a zone offset"
	_ = t.GobDecode(data)                 // want "domain-is-pure: this matches a zone offset"
	return fmt.Errorf("probe in %s", loc) // fmt.Errorf passes, loc is no call
}

// Reader and Writer stand in for io.Reader and io.Writer, which the domain
// cannot import.
type (
	Reader interface{ Read(p []byte) (int, error) }
	Writer interface{ Write(p []byte) (int, error) }
)

// Allowed is the pure formatting, parsing and time.Time use the domain needs.
func Allowed(t time.Time, w Writer, r Reader) (time.Time, error) {
	var n int
	if _, err := fmt.Sscan("1", &n); err != nil {
		return time.Time{}, fmt.Errorf("parse: %w", err)
	}
	if _, err := fmt.Sscanf("1", "%d", &n); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Sscanln("1", &n); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fscan(r, &n); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fscanf(r, "%d", &n); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fscanln(r, &n); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fprint(w, n); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fprintf(w, "%d\n", n); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fprintln(w, n); err != nil {
		return time.Time{}, err
	}
	_ = fmt.Sprint(n) + fmt.Sprintln(n)
	at, err := time.ParseInLocation(time.RFC3339, fmt.Sprintf("%d", n), time.UTC)
	if err != nil {
		return time.Time{}, err
	}
	_ = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.FixedZone("CET", 3600))
	_ = t.Unix() + t.UnixMilli() + t.UnixMicro()
	_ = t.UTC().Format(time.RFC3339)
	_, _ = t.MarshalJSON()
	return at.Add(time.Duration(n) * time.Hour), nil
}
