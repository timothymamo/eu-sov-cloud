package examples_test

import (
	"net/netip"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/timothymamo/shift2eu/internal/domain/taxonomy"
)

const (
	// dummyAccountID is the AWS account ID every sample uses (CLAUDE.md
	// rule 2): 123456789012 is a documented example value, never a real one.
	dummyAccountID = "123456789012"

	// documentationPrefix is the RFC 5737 documentation range every sample
	// address is in. Names use example.com (RFC 2606) or the reserved .example
	// TLD.
	documentationPrefix = "192.0.2.0/24"
)

// The sanitisation patterns. They are deliberately simple: a sample is dummy
// data, and each pattern catches the shape of a value that could identify a
// real account.
var (
	// accountIDPattern finds every run of 12 digits, the length of an AWS
	// account ID. Only a run that stands alone is an account ID (standsAlone):
	// a run inside a longer number, a letter sequence or a hyphenated identifier
	// such as an Azure subscription GUID's last group is something else.
	accountIDPattern = regexp.MustCompile(`[0-9]{12}`)

	// ipv4Pattern finds every dotted-quad candidate for an IPv4 address: four
	// runs of 1 to 3 digits. Each is parsed, so "1.2.3.400" is reported as
	// malformed rather than taken for an address.
	ipv4Pattern = regexp.MustCompile(`\b[0-9]{1,3}(?:\.[0-9]{1,3}){3}\b`)

	// hostnamePattern finds every lowercase dot-separated name, the shape of a
	// DNS name: one or more labels, each starting and ending with a letter or
	// digit, separated by dots. Uppercase dot-separated names are cloud
	// resource type namespaces such as Microsoft.Compute, not domain names, so
	// they are left alone; so is the taxonomy vocabulary (compute.vm,
	// database.postgresql), which is dotted lowercase too and is not data. An
	// IPv4 address matches too and is checked by checkIPv4.
	hostnamePattern = regexp.MustCompile(`\b[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+\b`)

	// documentationPrefixNet is documentationPrefix, parsed once.
	documentationPrefixNet = netip.MustParsePrefix(documentationPrefix)
)

// TestSamplesAreSanitised pins CLAUDE.md rule 2 for the samples: no sample may
// carry real identifiers, so every 12-digit account ID is 123456789012, every
// IPv4 address is in 192.0.2.0/24, and every domain name ends in example.com
// or .example. A person copies these files into their own demo, so they read
// as documentation values only.
func TestSamplesAreSanitised(t *testing.T) {
	t.Parallel()

	for _, file := range sampleFiles(t) {
		t.Run(file.name, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(file.path)
			if err != nil {
				t.Fatalf("read %s: %v", file.path, err)
			}

			text := string(data)

			checkAccountIDs(t, file.name, text)
			checkIPv4(t, file.name, text)
			checkHostnames(t, file.name, text)
		})
	}
}

// checkAccountIDs fails on every 12-digit account ID, or GCP project number,
// that is not the dummy one.
func checkAccountIDs(t *testing.T, name, text string) {
	t.Helper()

	for _, match := range accountIDPattern.FindAllStringIndex(text, -1) {
		if !standsAlone(text, match[0], match[1]) {
			continue
		}

		if got := text[match[0]:match[1]]; got != dummyAccountID {
			t.Errorf("%s: 12-digit number %s is not the dummy %s (an AWS account ID or GCP project number, CLAUDE.md rule 2)",
				name, got, dummyAccountID)
		}
	}
}

// standsAlone reports whether text[start:end] is a whole identifier rather than
// a part of a longer one: neither neighbour may be a digit, a letter or a
// hyphen.
func standsAlone(text string, start, end int) bool {
	return !identifierByte(text, start-1) && !identifierByte(text, end)
}

// identifierByte reports whether the byte at i, when there is one, is part of
// an identifier: a digit, an ASCII letter or a hyphen.
func identifierByte(text string, i int) bool {
	if i < 0 || i >= len(text) {
		return false
	}

	c := text[i]

	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '-'
}

// checkIPv4 fails on every IPv4 address outside the documentation range, and
// on a dotted quad that is not an address at all.
func checkIPv4(t *testing.T, name, text string) {
	t.Helper()

	for _, candidate := range ipv4Pattern.FindAllString(text, -1) {
		addr, err := netip.ParseAddr(candidate)
		if err != nil || !addr.Is4() {
			t.Errorf("%s: %q looks like an IPv4 address but is not one; samples use addresses in %s",
				name, candidate, documentationPrefix)

			continue
		}

		if !documentationPrefixNet.Contains(addr) {
			t.Errorf("%s: IPv4 address %s is outside the documentation range %s (CLAUDE.md rule 2)",
				name, candidate, documentationPrefix)
		}
	}
}

// checkHostnames fails on every domain name that does not end in example.com
// or .example.
func checkHostnames(t *testing.T, name, text string) {
	t.Helper()

	for _, candidate := range hostnamePattern.FindAllString(text, -1) {
		if _, err := netip.ParseAddr(candidate); err == nil {
			continue
		}

		// The taxonomy vocabulary is dotted lowercase as well, and an inventory
		// carries it in every resource: a value the taxonomy declares is not a
		// domain name.
		if _, err := taxonomy.ParseCategory(candidate); err == nil {
			continue
		}

		if _, err := taxonomy.ParseCapability(candidate); err == nil {
			continue
		}

		if candidate == "example.com" ||
			strings.HasSuffix(candidate, ".example.com") ||
			strings.HasSuffix(candidate, ".example") {
			continue
		}

		t.Errorf("%s: %q looks like a domain name but does not end in example.com or .example (CLAUDE.md rule 2)",
			name, candidate)
	}
}
