package architecture_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/test/architecture/internal/check"
)

// pinnedDepguardRules is a copy of every depguard rule of .golangci.yml: the
// list-mode of each, its files list, its allow list and the pkg of each deny
// entry, rule by rule, in document order. Issue #80 is why it is here: a merge
// conflict resolution dropped the whole reporters-stay-offline rule and the
// exact jsondoc$ allow entry of adapters-are-built-in-cmd, no test noticed, and
// the dependency rule was quietly looser than the architecture it enforces.
//
// Change .golangci.yml and this copy in the same commit, on purpose, and let a
// reviewer read the edit as a change to the architecture rules: a rule, an
// entry or a list-mode that differs fails TestDepguardRulesArePinned, and
// TestDepguardPinCatchesMutations proves the pin would notice.
//
// The deny entries are pinned by their pkg only: a reworded desc is not a
// weakening of the rule. app-tests-use-fakes-not-adapters shares its deny list
// with app-uses-ports-not-adapters through a YAML anchor, so the entries listed
// under it are the ones the alias stands for.
var pinnedDepguardRules = []check.DepguardRule{
	{
		Name:     "domain-is-pure",
		ListMode: "strict",
		Files: []string{
			"**/internal/domain/**",
			"!$test",
		},
		Allow: []string{
			"errors$",
			"fmt$",
			"net/netip$",
			"net/url$",
			"slices$",
			"strconv$",
			"strings$",
			"time$",
			"unicode$",
			"unicode/utf8$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/assessment$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/mapping$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/text$",
		},
		Deny: []string{
			"os",
			"net",
			"syscall",
			"unsafe",
			"plugin",
			"runtime/cgo",
		},
	},
	{
		Name:     "app-uses-ports-not-adapters",
		ListMode: "strict",
		Files: []string{
			"**/internal/app/**",
			"!$test",
		},
		Allow: []string{
			"context$",
			"errors$",
			"fmt$",
			"io$",
			"strings$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/assessment$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/mapping$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/text$",
		},
		Deny: []string{
			"github.com/timothymamo/eu-sov-cloud/internal/adapter",
			"github.com/timothymamo/eu-sov-cloud/internal/cli",
			"github.com/aws/aws-sdk-go-v2",
			"github.com/Azure/azure-sdk-for-go",
			"cloud.google.com/go",
			"google.golang.org/api",
			"net/http",
			"net/rpc",
			"net/smtp",
			"net/mail",
			"net",
			"os/exec",
			"os/user",
			"os/signal",
			"os",
		},
	},
	{
		Name:     "app-tests-use-fakes-not-adapters",
		ListMode: "lax",
		Files: []string{
			"**/internal/app/**/*_test.go",
			"**/internal/app/*_test.go",
		},
		Allow: nil,
		Deny: []string{
			"github.com/timothymamo/eu-sov-cloud/internal/adapter",
			"github.com/timothymamo/eu-sov-cloud/internal/cli",
			"github.com/aws/aws-sdk-go-v2",
			"github.com/Azure/azure-sdk-for-go",
			"cloud.google.com/go",
			"google.golang.org/api",
			"net/http",
			"net/rpc",
			"net/smtp",
			"net/mail",
			"net",
			"os/exec",
			"os/user",
			"os/signal",
			"os",
		},
	},
	{
		Name:     "cli-gets-adapters-from-cmd",
		ListMode: "strict",
		Files: []string{
			"**/internal/cli/**",
			"!$test",
		},
		Allow: []string{
			"$gostd",
			"github.com/timothymamo/eu-sov-cloud/internal/app/analysis$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/assessment$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/mapping$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/text$",
			"github.com/timothymamo/eu-sov-cloud/internal/version$",
		},
		Deny: []string{
			"github.com/timothymamo/eu-sov-cloud/internal/adapter",
			"net$",
			"net/http$",
			"net/http/cgi$",
			"net/http/cookiejar$",
			"net/http/fcgi$",
			"net/http/httptest$",
			"net/http/httptrace$",
			"net/http/httputil$",
			"net/http/pprof$",
			"net/mail$",
			"net/rpc$",
			"net/rpc/jsonrpc$",
			"net/smtp$",
			"net/textproto$",
			"crypto/tls$",
			"os/exec$",
			"os/user$",
			"syscall$",
			"plugin$",
		},
	},
	{
		Name:     "adapters-are-built-in-cmd",
		ListMode: "lax",
		Files: []string{
			"$all",
			"!$test",
			"!**/cmd/eusov/**",
		},
		Allow: []string{
			"github.com/timothymamo/eu-sov-cloud/internal/adapter/internal/jsondoc$",
			"github.com/timothymamo/eu-sov-cloud/internal/adapter/target/snapshot$",
		},
		Deny: []string{
			"github.com/timothymamo/eu-sov-cloud/internal/adapter/internal/",
			"github.com/timothymamo/eu-sov-cloud/internal/adapter/",
		},
	},
	{
		Name:     "cli-and-cmd-imported-only-by-cmd",
		ListMode: "lax",
		Files: []string{
			"$all",
			"!$test",
			"!**/cmd/eusov/**",
			"!**/internal/cli/**",
		},
		Allow: nil,
		Deny: []string{
			"github.com/timothymamo/eu-sov-cloud/internal/cli$",
			"github.com/timothymamo/eu-sov-cloud/internal/cli/",
			"github.com/timothymamo/eu-sov-cloud/cmd$",
			"github.com/timothymamo/eu-sov-cloud/cmd/",
		},
	},
	{
		Name:     "snapshot-reader-only-in-targets",
		ListMode: "lax",
		Files: []string{
			"$all",
			"!$test",
			"!**/internal/adapter/target/**",
		},
		Allow: nil,
		Deny: []string{
			"github.com/timothymamo/eu-sov-cloud/internal/adapter/target/snapshot",
		},
	},
	{
		Name:     "target-snapshot-reader-parses-bytes",
		ListMode: "strict",
		Files: []string{
			"**/internal/adapter/target/snapshot/**",
			"!$test",
		},
		Allow: []string{
			"errors$",
			"fmt$",
			"strconv$",
			"strings$",
			"time$",
			"github.com/timothymamo/eu-sov-cloud/internal/adapter/internal/jsondoc$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/text$",
		},
		Deny: nil,
	},
	{
		Name:     "target-adapters-read-snapshots-through-the-reader",
		ListMode: "lax",
		Files: []string{
			"**/internal/adapter/target/**",
			"!$test",
		},
		Allow: nil,
		Deny: []string{
			"encoding/json",
		},
	},
	{
		Name:     "adapter-helpers-do-no-io",
		ListMode: "strict",
		Files: []string{
			"**/internal/adapter/internal/**",
			"!$test",
		},
		Allow: []string{
			"bytes$",
			"encoding/json$",
			"errors$",
			"fmt$",
			"io$",
			"slices$",
			"strconv$",
			"strings$",
			"unicode$",
			"unicode/utf16$",
			"unicode/utf8$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/text$",
		},
		Deny: []string{
			"io/fs",
			"io/ioutil",
			"net",
			"os",
			"plugin",
			"runtime/cgo",
			"syscall",
			"unsafe",
		},
	},
	{
		Name:     "jsondoc-parses-bytes",
		ListMode: "strict",
		Files: []string{
			"**/internal/adapter/internal/jsondoc/**",
			"!$test",
		},
		Allow: []string{
			"bytes$",
			"encoding/json$",
			"errors$",
			"fmt$",
			"io$",
			"slices$",
			"strconv$",
			"strings$",
			"unicode$",
			"unicode/utf16$",
			"unicode/utf8$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/text$",
		},
		Deny: nil,
	},
	{
		Name:     "inventory-file-reader-reads-one-file",
		ListMode: "strict",
		Files: []string{
			"**/internal/adapter/source/inventoryfile/**",
			"!$test",
		},
		Allow: []string{
			"context$",
			"errors$",
			"fmt$",
			"io$",
			"io/fs$",
			"os$",
			"github.com/timothymamo/eu-sov-cloud/internal/adapter/internal/jsondoc$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/text$",
		},
		Deny: []string{
			"encoding/json",
		},
	},
	{
		Name:     "cloud-adapters-reach-the-cloud-through-the-sdk",
		ListMode: "strict",
		Files: []string{
			"**/internal/adapter/source/**",
			"!**/internal/adapter/source/inventoryfile/**",
			"!**/internal/adapter/source/sourcetest/**",
		},
		Allow: []string{
			"bytes$",
			"context$",
			"errors$",
			"fmt$",
			"iter$",
			"log/slog$",
			"maps$",
			"net/netip$",
			"net/url$",
			"slices$",
			"sort$",
			"strconv$",
			"strings$",
			"sync$",
			"time$",
			"unicode$",
			"unicode/utf8$",
			"testing$",
			"github.com/aws/aws-sdk-go-v2/",
			"github.com/aws/smithy-go$",
			"github.com/Azure/azure-sdk-for-go/sdk/",
			"cloud.google.com/go/",
			"google.golang.org/api/googleapi$",
			"google.golang.org/api/iterator$",
			"google.golang.org/api/option$",
			"google.golang.org/grpc/codes$",
			"google.golang.org/grpc/status$",
			"github.com/google/go-cmp/cmp",
			"github.com/timothymamo/eu-sov-cloud/internal/adapter/source/aws$",
			"github.com/timothymamo/eu-sov-cloud/internal/adapter/source/azure$",
			"github.com/timothymamo/eu-sov-cloud/internal/adapter/source/gcp$",
			"github.com/timothymamo/eu-sov-cloud/internal/adapter/source/sourcetest$",
			"github.com/timothymamo/eu-sov-cloud/internal/app/analysis$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/text$",
			"github.com/timothymamo/eu-sov-cloud/internal/testutil$",
		},
		Deny: []string{
			"reflect",
			"text/template",
			"html/template",
			"unsafe",
			"os",
			"plugin",
			"syscall",
			"golang.org/x/sys",
			"runtime/cgo",
			"net",
			"database/sql",
		},
	},
	{
		Name:     "cloud-sdks-only-in-provider-adapters",
		ListMode: "lax",
		Files: []string{
			"$all",
			"!**/internal/adapter/source/aws/**",
			"!**/internal/adapter/source/azure/**",
			"!**/internal/adapter/source/gcp/**",
			"!**/internal/adapter/target/ovhcloud/**",
			"!**/internal/adapter/target/scaleway/**",
			"!**/internal/adapter/target/upcloud/**",
		},
		Allow: nil,
		Deny: []string{
			"github.com/aws/",
			"github.com/Azure/",
			"cloud.google.com/go",
			"google.golang.org/api",
			"google.golang.org/grpc",
			"github.com/googleapis/",
			"golang.org/x/oauth2",
			"github.com/ovh/",
			"github.com/scaleway/",
			"github.com/UpCloudLtd/",
		},
	},
	{
		Name:     "embedded-snapshot-targets-stay-offline",
		ListMode: "strict",
		Files: []string{
			"**/internal/adapter/target/ovhcloud/**",
			"**/internal/adapter/target/scaleway/**",
			"!$test",
		},
		Allow: []string{
			"context$",
			"embed$",
			"fmt$",
			"github.com/timothymamo/eu-sov-cloud/internal/adapter/target/snapshot$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/text$",
		},
		Deny: []string{
			"io/fs",
			"io/ioutil",
			"net",
			"os",
			"plugin",
			"syscall",
			"unsafe",
		},
	},
	{
		Name:     "reporters-stay-offline",
		ListMode: "strict",
		Files: []string{
			"**/internal/adapter/report/**",
			"!$test",
		},
		Allow: []string{
			"context$",
			"fmt$",
			"io$",
			"strconv$",
			"strings$",
			"github.com/timothymamo/eu-sov-cloud/internal/app/analysis$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/assessment$",
			"github.com/timothymamo/eu-sov-cloud/internal/domain/mapping$",
		},
		Deny: []string{
			"io/fs",
			"io/ioutil",
			"net",
			"os/exec",
			"os",
			"plugin",
			"runtime/cgo",
			"syscall",
			"unsafe",
		},
	},
	{
		Name:     "testutil-only-in-tests",
		ListMode: "lax",
		Files: []string{
			"$all",
			"!$test",
			"!**/internal/testutil/**",
		},
		Allow: nil,
		Deny: []string{
			"github.com/timothymamo/eu-sov-cloud/internal/testutil",
		},
	},
	{
		Name:     "contract-suites-only-in-tests",
		ListMode: "lax",
		Files: []string{
			"$all",
			"!$test",
			"!**/internal/adapter/source/sourcetest/**",
			"!**/internal/adapter/target/targettest/**",
		},
		Allow: nil,
		Deny: []string{
			"github.com/timothymamo/eu-sov-cloud/internal/adapter/source/sourcetest",
			"github.com/timothymamo/eu-sov-cloud/internal/adapter/target/targettest",
		},
	},
	{
		Name:     "go-cmp-only-in-tests",
		ListMode: "lax",
		Files: []string{
			"$all",
			"!$test",
		},
		Allow: nil,
		Deny: []string{
			"github.com/google/go-cmp",
		},
	},
	{
		Name:     "e2e-is-black-box",
		ListMode: "strict",
		Files: []string{
			"**/test/e2e/**",
		},
		Allow: []string{
			"$gostd",
			"github.com/google/go-cmp",
			"github.com/timothymamo/eu-sov-cloud/internal/testutil$",
		},
		Deny: []string{
			"github.com/timothymamo/eu-sov-cloud/",
			"github.com/aws/aws-sdk-go-v2",
			"github.com/Azure/azure-sdk-for-go",
			"cloud.google.com/go",
			"google.golang.org/api",
		},
	},
}

// TestDepguardRulesArePinned pins every depguard rule of .golangci.yml against
// pinnedDepguardRules. A rule removed, weakened or added without this copy
// following it fails with the rule named, so no merge can drop a piece of the
// dependency rule unseen (issue #80).
func TestDepguardRulesArePinned(t *testing.T) {
	t.Parallel()

	golangci := readRepositoryFile(t, ".golangci.yml")

	rules, err := check.DepguardRules(golangci)
	if err != nil {
		t.Fatalf("DepguardRules(.golangci.yml): %v", err)
	}

	if len(rules) != len(pinnedDepguardRules) {
		t.Errorf(".golangci.yml holds %d depguard rules, the pin holds %d", len(rules), len(pinnedDepguardRules))
	}

	for _, message := range check.PinnedRules(rules, pinnedDepguardRules) {
		t.Errorf("%s\n"+
			"the depguard rules are pinned: if .golangci.yml changed on purpose, update pinnedDepguardRules in this file in the same commit, and a reviewer reads that edit as a change to the architecture rules", message)
	}
}

// TestDepguardPinCatchesMutations proves the pin of TestDepguardRulesArePinned
// is not vacuous: it takes the real .golangci.yml text in memory and, for every
// rule, deletes the rule, deletes each entry of its files, allow and deny lists,
// and drops the $ of each exact allow entry. Every mutated document must either
// fail to parse or make PinnedRules name the rule it is about, so a weakened
// rule cannot read as the pinned one. A mutation that leaves the document
// unchanged fails, so no case here passes on nothing.
func TestDepguardPinCatchesMutations(t *testing.T) {
	t.Parallel()

	if len(pinnedDepguardRules) == 0 {
		t.Fatal("pinnedDepguardRules is empty, so this test would mutate nothing")
	}

	mutations := depguardMutations(t, string(readRepositoryFile(t, ".golangci.yml")))

	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			t.Parallel()

			requireMutationReported(t, mutation.lines, mutation.rule)
		})
	}
}

// depguardMutation is one weakening of the depguard rules: the name of its case,
// the rule it is about and the mutated lines of .golangci.yml.
type depguardMutation struct {
	name  string
	rule  string
	lines []string
}

// depguardMutations builds every weakening of .golangci.yml the pin must catch,
// before any case runs: each mutation must change the document, and at least one
// must exist, so no case can pass on nothing.
func depguardMutations(t *testing.T, document string) []depguardMutation {
	t.Helper()

	lines := strings.Split(document, "\n")

	var mutations []depguardMutation

	add := func(rule, name string, mutated []string) {
		t.Helper()

		if strings.Join(mutated, "\n") == document {
			t.Fatalf("%s: the mutation left .golangci.yml unchanged, so its case would pass on nothing", name)
		}

		mutations = append(mutations, depguardMutation{name: name, rule: rule, lines: mutated})
	}

	for _, rule := range pinnedDepguardRules {
		span := findDepguardRule(t, lines, rule.Name)

		add(rule.Name, rule.Name+": rule deleted",
			deleteDepguardLines(lines, span.key, span.end))

		// The entries are read from the document, not from the pin: a rule
		// that shares its deny list through an alias has no entry lines of
		// its own, and the rule it shares them with is mutated on its own.
		for _, section := range []string{"files", "allow", "deny"} {
			for i, entry := range depguardEntries(t, lines, span, section) {
				add(rule.Name, fmt.Sprintf("%s: %s entry %d deleted: %s", rule.Name, section, i, entry.text),
					deleteDepguardLines(lines, entry.from, entry.to))
			}
		}

		for i, entry := range depguardEntries(t, lines, span, "allow") {
			if !entry.exact() {
				continue
			}

			mutated := slices.Clone(lines)
			mutated[entry.from] = dropDollar(lines[entry.from])

			add(rule.Name, fmt.Sprintf("%s: allow entry %d without its $: %s", rule.Name, i, entry.text), mutated)
		}
	}

	if len(mutations) == 0 {
		t.Fatal("no mutation was made, so this test would prove nothing")
	}

	return mutations
}

// depguardRuleSpan is where one rule sits in the .golangci.yml lines: its key
// line and the last line of its body.
type depguardRuleSpan struct {
	key int
	// end is one past the rule's last line.
	end int
}

// depguardEntry is one entry line of a rule's files, allow or deny list, from
// its dashes of the line it starts on to the last line that belongs to it: the
// desc of a deny entry is on the line after its pkg.
type depguardEntry struct {
	text string
	from int
	to   int
}

// indentOf returns the indentation of a line and its text, trailing white space
// removed.
func indentOf(line string) (int, string) {
	text := strings.TrimLeft(line, " ")
	spaces := len(line) - len(text)
	trimmed := strings.TrimRight(text, " \t")

	return spaces, trimmed
}

// isDepguardContent reports whether a line holds something but a comment.
func isDepguardContent(text string) bool {
	return text != "" && !strings.HasPrefix(text, "#")
}

// findDepguardRule returns where the rule named name sits in the lines of
// .golangci.yml. A rule is a key at the indentation of the depguard rules, and
// its body is every line below it indented further; the test fails when the
// rule cannot be found, rather than mutate nothing.
func findDepguardRule(t *testing.T, lines []string, name string) depguardRuleSpan {
	t.Helper()

	for i, line := range lines {
		indent, text := indentOf(line)
		if indent != 8 || text != name+":" {
			continue
		}

		end := len(lines)

		for j := i + 1; j < len(lines); j++ {
			jIndent, jText := indentOf(lines[j])
			if isDepguardContent(jText) && jIndent <= indent {
				end = j

				break
			}
		}

		return depguardRuleSpan{key: i, end: end}
	}

	t.Fatalf("no depguard rule named %s in .golangci.yml, so this mutation test would pass on nothing", name)

	return depguardRuleSpan{}
}

// depguardEntries returns the entry lines of the rule's section (files, allow
// or deny), in file order. A section the rule does not have, or one whose list
// it shares through an alias, has no entry line of its own.
func depguardEntries(t *testing.T, lines []string, span depguardRuleSpan, section string) []depguardEntry {
	t.Helper()

	start := -1

	for i := span.key + 1; i < span.end; i++ {
		indent, text := indentOf(lines[i])
		if indent == 10 && (text == section+":" || strings.HasPrefix(text, section+": ")) {
			start = i + 1

			break
		}
	}

	if start < 0 {
		return nil
	}

	var entries []depguardEntry

	for i := start; i < span.end; i++ {
		indent, text := indentOf(lines[i])
		if !isDepguardContent(text) {
			continue
		}

		if indent != 12 || !strings.HasPrefix(text, "- ") {
			break
		}

		to := i + 1

		for j := i + 1; j < span.end; j++ {
			jIndent, jText := indentOf(lines[j])
			if !isDepguardContent(jText) {
				continue
			}

			if jIndent <= indent {
				break
			}

			to = j + 1
		}

		entries = append(entries, depguardEntry{text: text[2:], from: i, to: to})
	}

	return entries
}

// exact reports whether the entry ends the way an exact depguard entry does.
func (e depguardEntry) exact() bool {
	return endsInDollar(e.text)
}

// endsInDollar reports whether a scalar entry ends in $, its quotes counted as
// the quotes they are.
func endsInDollar(text string) bool {
	return strings.HasSuffix(unquote(text), "$")
}

// unquote returns the text with the quotes a YAML scalar would have around it
// removed.
func unquote(text string) string {
	return strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(text), `"`), `"`)
}

// dropDollar returns the entry's line with the trailing $ of its value dropped,
// its quotes and indentation kept: the weakening the pin must notice.
func dropDollar(line string) string {
	text := strings.TrimRight(line, " ")
	quote := ""

	if strings.HasSuffix(text, `"`) {
		quote = `"`

		text = strings.TrimSuffix(text, `"`)
	}

	return strings.TrimSuffix(strings.TrimSuffix(text, "$"), `"`) + quote
}

// deleteDepguardLines returns the lines without [from, to).
func deleteDepguardLines(lines []string, from, to int) []string {
	out := make([]string, 0, len(lines)-(to-from))
	out = append(out, lines[:from]...)
	out = append(out, lines[to:]...)

	return out
}

// requireMutationReported fails unless the mutated document either fails to
// parse or makes PinnedRules report a difference that starts with the rule's
// name. depguardMutations has already refused a mutation that changes nothing.
func requireMutationReported(t *testing.T, mutated []string, rule string) {
	t.Helper()

	rules, err := check.DepguardRules([]byte(strings.Join(mutated, "\n")))
	if err != nil {
		// A mutated document the reader refuses is caught by the check: it
		// fails rather than reads the rule as the pinned one.
		return
	}

	if !slices.ContainsFunc(check.PinnedRules(rules, pinnedDepguardRules), func(message string) bool {
		return strings.HasPrefix(message, rule+": ")
	}) {
		t.Errorf("the mutated document reads as the pinned rules: DepguardRules() = %v and PinnedRules() reported nothing about %s, so the pin would miss this mutation", rules, rule)
	}
}
