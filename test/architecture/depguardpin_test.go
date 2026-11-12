package architecture_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
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
			"github.com/timothymamo/shift2eu/internal/domain/assessment$",
			"github.com/timothymamo/shift2eu/internal/domain/catalog$",
			"github.com/timothymamo/shift2eu/internal/domain/inventory$",
			"github.com/timothymamo/shift2eu/internal/domain/mapping$",
			"github.com/timothymamo/shift2eu/internal/domain/taxonomy$",
			"github.com/timothymamo/shift2eu/internal/domain/text$",
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
			"github.com/timothymamo/shift2eu/internal/domain/assessment$",
			"github.com/timothymamo/shift2eu/internal/domain/catalog$",
			"github.com/timothymamo/shift2eu/internal/domain/inventory$",
			"github.com/timothymamo/shift2eu/internal/domain/mapping$",
			"github.com/timothymamo/shift2eu/internal/domain/taxonomy$",
			"github.com/timothymamo/shift2eu/internal/domain/text$",
		},
		Deny: []string{
			"github.com/timothymamo/shift2eu/internal/adapter",
			"github.com/timothymamo/shift2eu/internal/cli",
			"github.com/aws/aws-sdk-go-v2",
			"github.com/Azure/azure-sdk-for-go",
			"cloud.google.com/go",
			"google.golang.org/api",
			"net/http",
			"net/rpc",
			"net/smtp",
			"net/mail",
			"net/textproto",
			"crypto/tls",
			"net$",
			"os/exec",
			"os/user",
			"os/signal",
			"os$",
			"syscall",
			"plugin",
			"io/ioutil",
			"log/syslog",
			"text/template",
			"html/template",
			"archive/zip",
			"debug/",
			"go/build$",
			"go/importer",
			"runtime/debug",
			"go/parser",
			"mime/multipart",
			"runtime/coverage",
			"unsafe",
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
			"github.com/timothymamo/shift2eu/internal/adapter",
			"github.com/timothymamo/shift2eu/internal/cli",
			"github.com/aws/aws-sdk-go-v2",
			"github.com/Azure/azure-sdk-for-go",
			"cloud.google.com/go",
			"google.golang.org/api",
			"net/http",
			"net/rpc",
			"net/smtp",
			"net/mail",
			"net/textproto",
			"crypto/tls",
			"net$",
			"os/exec",
			"os/user",
			"os/signal",
			"os$",
			"syscall",
			"plugin",
			"io/ioutil",
			"log/syslog",
			"text/template",
			"html/template",
			"archive/zip",
			"debug/",
			"go/build$",
			"go/importer",
			"runtime/debug",
			"go/parser",
			"mime/multipart",
			"runtime/coverage",
			"unsafe",
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
			"github.com/timothymamo/shift2eu/internal/app/analysis$",
			"github.com/timothymamo/shift2eu/internal/domain/assessment$",
			"github.com/timothymamo/shift2eu/internal/domain/catalog$",
			"github.com/timothymamo/shift2eu/internal/domain/inventory$",
			"github.com/timothymamo/shift2eu/internal/domain/mapping$",
			"github.com/timothymamo/shift2eu/internal/domain/taxonomy$",
			"github.com/timothymamo/shift2eu/internal/domain/text$",
			"github.com/timothymamo/shift2eu/internal/version$",
		},
		Deny: []string{
			"github.com/timothymamo/shift2eu/internal/adapter",
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
		// The composition root (issue #101): main wires every adapter into the
		// application, so it may import every package of the module, named by
		// the one prefix entry, and the standard packages it imports today,
		// each exact. A third-party import is refused by the allow list, and
		// the cloud SDKs by the deny list too, as in app-uses-ports-not-adapters.
		// A sibling such as cmd/shift2eu-server gets a rule of its own, written
		// the same way, with its own files: entry.
		Name:     "composition-root",
		ListMode: "strict",
		Files: []string{
			"**/cmd/shift2eu/**",
			"!$test",
		},
		Allow: []string{
			"context$",
			"os$",
			"os/signal$",
			"syscall$",
			"github.com/timothymamo/shift2eu/",
		},
		Deny: []string{
			"github.com/aws/aws-sdk-go-v2",
			"github.com/Azure/azure-sdk-for-go",
			"cloud.google.com/go",
			"google.golang.org/api",
		},
	},
	{
		// The server's composition root (PRD #103, milestone 3), the sibling of
		// composition-root and written the same way: its files: list names
		// cmd/shift2eu-server and nothing beside it, its allow list is the module
		// path as the one prefix entry plus the standard packages the server
		// imports today, each exact, and its deny list names the four cloud
		// SDKs. A third-party package the allow list does not name is refused by
		// the allow list, so the server cannot gain a dependency unseen.
		//
		// test/architecture reads the rule the way it reads composition-root:
		// TestCompositionRootRulesAreChecked pins its files, allow and prefix
		// entries and its skip of the shared layer listing,
		// TestCompositionRootRulesAreHeldToTheirRuleForEveryReleaseTarget holds
		// cmd/shift2eu-server to it on every release target, and
		// TestCompositionRootRulesHoldTheirProbesForEveryReleaseTarget fails a
		// fixture server root that imports a third-party library or a cloud SDK.
		Name:     "server-composition-root",
		ListMode: "strict",
		Files: []string{
			"**/cmd/shift2eu-server/**",
			"!$test",
		},
		Allow: []string{
			"context$",
			"errors$",
			"flag$",
			"fmt$",
			"io$",
			"log$",
			"math$",
			"net$",
			"net/http$",
			"os$",
			"os/signal$",
			"runtime$",
			"runtime/debug$",
			"slices$",
			"strconv$",
			"strings$",
			"sync$",
			"syscall$",
			"time$",
			"github.com/timothymamo/shift2eu/",
		},
		Deny: []string{
			"github.com/aws/aws-sdk-go-v2",
			"github.com/Azure/azure-sdk-for-go",
			"cloud.google.com/go",
			"google.golang.org/api",
		},
	},
	{
		Name:     "adapters-are-built-in-cmd",
		ListMode: "lax",
		Files: []string{
			"$all",
			"!$test",
			"!**/cmd/shift2eu/**",
			"!**/cmd/shift2eu-server/**",
		},
		Allow: []string{
			"github.com/timothymamo/shift2eu/internal/adapter/internal/jsondoc$",
			"github.com/timothymamo/shift2eu/internal/adapter/target/snapshot$",
		},
		Deny: []string{
			"github.com/timothymamo/shift2eu/internal/adapter/internal/",
			"github.com/timothymamo/shift2eu/internal/adapter/report/",
			"github.com/timothymamo/shift2eu/internal/adapter/report$",
			"github.com/timothymamo/shift2eu/internal/adapter/request/",
			"github.com/timothymamo/shift2eu/internal/adapter/request$",
			"github.com/timothymamo/shift2eu/internal/adapter/source/",
			"github.com/timothymamo/shift2eu/internal/adapter/source$",
			"github.com/timothymamo/shift2eu/internal/adapter/target/",
			"github.com/timothymamo/shift2eu/internal/adapter/target$",
		},
	},
	{
		Name:     "cli-and-cmd-imported-only-by-cmd",
		ListMode: "lax",
		Files: []string{
			"$all",
			"!$test",
			"!**/cmd/shift2eu/**",
			"!**/internal/cli/**",
		},
		Allow: nil,
		Deny: []string{
			"github.com/timothymamo/shift2eu/internal/cli$",
			"github.com/timothymamo/shift2eu/internal/cli/",
			"github.com/timothymamo/shift2eu/cmd$",
			"github.com/timothymamo/shift2eu/cmd/",
		},
	},
	{
		Name:     "httpapi-gets-adapters-from-cmd",
		ListMode: "strict",
		Files: []string{
			"**/internal/httpapi/**",
			"!$test",
		},
		Allow: []string{
			"bytes$",
			"context$",
			"crypto/sha256$",
			"crypto/subtle$",
			"encoding/json$",
			"errors$",
			"fmt$",
			"io$",
			"maps$",
			"mime$",
			"net/http$",
			"net/url$",
			"runtime$",
			"slices$",
			"sort$",
			"strconv$",
			"strings$",
			"sync$",
			"time$",
			"unicode/utf8$",
			"github.com/timothymamo/shift2eu/internal/app/analysis$",
			"github.com/timothymamo/shift2eu/internal/domain/assessment$",
			"github.com/timothymamo/shift2eu/internal/domain/catalog$",
			"github.com/timothymamo/shift2eu/internal/domain/inventory$",
			"github.com/timothymamo/shift2eu/internal/domain/mapping$",
			"github.com/timothymamo/shift2eu/internal/domain/taxonomy$",
			"github.com/timothymamo/shift2eu/internal/domain/text$",
			"github.com/timothymamo/shift2eu/internal/version$",
		},
		Deny: []string{
			"github.com/timothymamo/shift2eu/internal/adapter",
			"github.com/timothymamo/shift2eu/internal/cli",
			"github.com/timothymamo/shift2eu/cmd",
			"os",
			"syscall",
			"plugin",
			"unsafe",
		},
	},
	{
		Name:     "httpapi-imported-only-by-the-server",
		ListMode: "lax",
		Files: []string{
			"$all",
			"!**/test/apiparity/**",
			"!**/cmd/shift2eu-server/**",
			"!**/internal/httpapi/**",
		},
		Allow: nil,
		Deny: []string{
			"github.com/timothymamo/shift2eu/internal/httpapi$",
			"github.com/timothymamo/shift2eu/internal/httpapi/",
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
			"github.com/timothymamo/shift2eu/internal/adapter/target/snapshot",
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
			"github.com/timothymamo/shift2eu/internal/adapter/internal/jsondoc$",
			"github.com/timothymamo/shift2eu/internal/domain/catalog$",
			"github.com/timothymamo/shift2eu/internal/domain/taxonomy$",
			"github.com/timothymamo/shift2eu/internal/domain/text$",
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
			"github.com/timothymamo/shift2eu/internal/domain/text$",
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
			"github.com/timothymamo/shift2eu/internal/domain/text$",
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
			"github.com/timothymamo/shift2eu/internal/adapter/internal/jsondoc$",
			"github.com/timothymamo/shift2eu/internal/domain/inventory$",
			"github.com/timothymamo/shift2eu/internal/domain/taxonomy$",
			"github.com/timothymamo/shift2eu/internal/domain/text$",
		},
		Deny: []string{
			"encoding/json",
		},
	},
	{
		Name:     "request-decoders-parse-bytes",
		ListMode: "strict",
		Files: []string{
			"**/internal/adapter/request/**",
			"!$test",
		},
		Allow: []string{
			"errors$",
			"fmt$",
			"github.com/timothymamo/shift2eu/internal/adapter/internal/jsondoc$",
		},
		Deny: []string{
			"encoding/json",
			"net",
			"os",
			"github.com/timothymamo/shift2eu/internal/httpapi",
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
			"github.com/google/go-cmp/cmp$",
			"github.com/google/go-cmp/cmp/",
			"github.com/timothymamo/shift2eu/internal/adapter/source/aws$",
			"github.com/timothymamo/shift2eu/internal/adapter/source/azure$",
			"github.com/timothymamo/shift2eu/internal/adapter/source/gcp$",
			"github.com/timothymamo/shift2eu/internal/adapter/source/sourcetest$",
			"github.com/timothymamo/shift2eu/internal/app/analysis$",
			"github.com/timothymamo/shift2eu/internal/domain/inventory$",
			"github.com/timothymamo/shift2eu/internal/domain/taxonomy$",
			"github.com/timothymamo/shift2eu/internal/domain/text$",
			"github.com/timothymamo/shift2eu/internal/testutil$",
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
			"**/internal/adapter/target/upcloud/**",
			"!$test",
		},
		Allow: []string{
			"context$",
			"embed$",
			"fmt$",
			"github.com/timothymamo/shift2eu/internal/adapter/target/snapshot$",
			"github.com/timothymamo/shift2eu/internal/domain/catalog$",
			"github.com/timothymamo/shift2eu/internal/domain/text$",
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
			"encoding/json$",
			"github.com/timothymamo/shift2eu/internal/app/analysis$",
			"github.com/timothymamo/shift2eu/internal/domain/assessment$",
			"github.com/timothymamo/shift2eu/internal/domain/mapping$",
		},
		Deny: []string{
			"io/fs",
			"io/ioutil",
			"net",
			"os/exec",
			"os/signal",
			"os/user",
			"os$",
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
			"github.com/timothymamo/shift2eu/internal/testutil",
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
			"github.com/timothymamo/shift2eu/internal/adapter/source/sourcetest",
			"github.com/timothymamo/shift2eu/internal/adapter/target/targettest",
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
			"github.com/timothymamo/shift2eu/internal/testutil$",
		},
		Deny: []string{
			"github.com/timothymamo/shift2eu/",
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
// drops the $ of each exact allow entry, adds an allow entry, adds a files
// exclusion and rewrites, deletes or adds the list-mode. A deletion and an
// addition weaken a rule alike, and the entries added here are the ones a
// careless edit adds. Every mutated document must make PinnedRules name the
// rule it is about, except the one mutation that breaks the document itself,
// which the reader must refuse rather than read as a weaker rule. A mutation
// that leaves the document unchanged fails, so no case here passes on nothing.
func TestDepguardPinCatchesMutations(t *testing.T) {
	t.Parallel()

	if len(pinnedDepguardRules) == 0 {
		t.Fatal("pinnedDepguardRules is empty, so this test would mutate nothing")
	}

	mutations := depguardMutations(t, string(readRepositoryFile(t, ".golangci.yml")))

	t.Logf("%d mutations of .golangci.yml", len(mutations))

	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			t.Parallel()

			requireMutationReported(t, mutation)
		})
	}
}

// depguardMutation is one weakening of the depguard rules: the name of its case,
// the rule it is about, the mutated lines of .golangci.yml and whether the
// reader is expected to refuse the mutation rather than read it as a different
// rule.
type depguardMutation struct {
	name  string
	rule  string
	lines []string
	// wantParseError is set on the one mutation that breaks the document
	// itself: deleting the rule that holds the deny list its alias needs
	// leaves the alias with no anchor, and the reader refuses the document.
	wantParseError bool
}

// depguardMutations builds every weakening of .golangci.yml the pin must catch,
// before any case runs: each mutation must change the document, and at least one
// must exist, so no case can pass on nothing. It fails when the entry walk does
// not find as many entries as the pin holds, so the mutations below cannot
// quietly cover a list's first entry only.
func depguardMutations(t *testing.T, document string) []depguardMutation {
	t.Helper()

	lines := strings.Split(document, "\n")

	var mutations []depguardMutation

	// counts says how many mutations each kind of weakening made, for the
	// log line at the end.
	counts := map[string]int{}

	add := func(m depguardMutation, kind string) {
		t.Helper()

		if strings.Join(m.lines, "\n") == document {
			t.Fatalf("%s: the mutation left .golangci.yml unchanged, so its case would pass on nothing", m.name)
		}

		counts[kind]++
		mutations = append(mutations, m)
	}

	for _, rule := range pinnedDepguardRules {
		span := findDepguardRule(t, lines, rule.Name)

		// The entries are read from the document, not from the pin: a rule
		// that shares its deny list through an alias has no entry lines of
		// its own, and the rule it shares them with is mutated on its own.
		sections := make(map[string][]depguardEntry, 3)

		for _, section := range []string{"files", "allow", "deny"} {
			sections[section] = depguardEntries(t, lines, span, section)

			want := len(rule.Files)

			switch section {
			case "allow":
				want = len(rule.Allow)
			case "deny":
				want = len(rule.Deny)
			}

			if depguardSectionAliased(t, lines, span, section) {
				want = 0
			}

			if got := len(sections[section]); got != want {
				t.Fatalf("%s: the document holds %d %s entries, the pin holds %d: the entry walk lost entries, so the mutations below would miss them", rule.Name, got, section, want)
			}
		}

		add(depguardMutation{
			name:  rule.Name + ": rule deleted",
			rule:  rule.Name,
			lines: deleteDepguardLines(lines, span.key, span.end),
			// The alias rule needs the anchor this rule holds: the reader
			// refuses the document, which is the check failing closed.
			wantParseError: rule.Name == "app-uses-ports-not-adapters",
		}, "rule deleted")

		for _, section := range []string{"files", "allow", "deny"} {
			for i, entry := range sections[section] {
				add(depguardMutation{
					name:  fmt.Sprintf("%s: %s entry %d deleted: %s", rule.Name, section, i, entry.text),
					rule:  rule.Name,
					lines: deleteDepguardLines(lines, entry.from, entry.to),
				}, section+" entry deleted")
			}
		}

		for i, entry := range sections["allow"] {
			if !entry.exact() {
				continue
			}

			mutated := slices.Clone(lines)
			mutated[entry.from] = dropDollar(lines[entry.from])

			add(depguardMutation{
				name:  fmt.Sprintf("%s: allow entry %d without its $: %s", rule.Name, i, entry.text),
				rule:  rule.Name,
				lines: mutated,
			}, "allow entry dropped its $")
		}

		// An addition weakens a rule as surely as a deletion: an allow
		// entry, a files exclusion or a laxer list-mode is what a careless
		// edit adds.
		if at, ok := depguardInsertPoint(t, lines, span, "allow"); ok {
			add(depguardMutation{
				name:  rule.Name + ": allow entry added: os",
				rule:  rule.Name,
				lines: insertDepguardLine(lines, at, 12, "- os"),
			}, "allow entry added")
		}

		if at, ok := depguardInsertPoint(t, lines, span, "files"); ok {
			add(depguardMutation{
				name:  rule.Name + `: files entry added: "!**/x/**"`,
				rule:  rule.Name,
				lines: insertDepguardLine(lines, at, 12, `- "!**/x/**"`),
			}, "files entry added")
		}

		if at, mode, ok := depguardListMode(t, lines, span); ok {
			for _, other := range []string{"original", "strict", "lax"} {
				if other == mode {
					continue
				}

				mutated := slices.Clone(lines)
				mutated[at] = "          list-mode: " + other

				add(depguardMutation{
					name:  fmt.Sprintf("%s: list-mode rewritten from %s to %s", rule.Name, mode, other),
					rule:  rule.Name,
					lines: mutated,
				}, "list-mode rewritten")
			}

			add(depguardMutation{
				name:  rule.Name + ": list-mode line deleted",
				rule:  rule.Name,
				lines: deleteDepguardLines(lines, at, at+1),
			}, "list-mode deleted")
		} else {
			add(depguardMutation{
				name:  rule.Name + ": list-mode strict added",
				rule:  rule.Name,
				lines: insertDepguardLine(lines, span.key+1, 10, "list-mode: strict"),
			}, "list-mode added")
		}
	}

	if len(mutations) == 0 {
		t.Fatal("no mutation was made, so this test would prove nothing")
	}

	kinds := make([]string, 0, len(counts))
	for kind := range counts {
		kinds = append(kinds, kind)
	}

	slices.Sort(kinds)

	breakdown := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		breakdown = append(breakdown, fmt.Sprintf("%d %s", counts[kind], kind))
	}

	t.Logf("%d mutations of .golangci.yml: %s", len(mutations), strings.Join(breakdown, ", "))

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

// The helpers below read the mutated .golangci.yml by indentation and assume
// its 2-space layout: the depguard rules at indent 8, their keys at 10 and the
// entries of a list at 12. They fail, naming the rule and the section, when the
// document no longer has that shape, rather than mutate nothing.

// depguardSectionKey returns the line of the rule's section key (files, allow
// or deny) and whether the rule has it.
func depguardSectionKey(t *testing.T, lines []string, span depguardRuleSpan, section string) (int, bool) {
	t.Helper()

	for i := span.key + 1; i < span.end; i++ {
		indent, text := indentOf(lines[i])
		if indent == 10 && (text == section+":" || strings.HasPrefix(text, section+": ")) {
			return i, true
		}
	}

	return 0, false
}

// depguardSectionAliased reports whether the rule's section key stands for a
// list written under another rule (deny: *app-deny): the section has no entry
// lines of its own, and the rule whose list it is holds them.
func depguardSectionAliased(t *testing.T, lines []string, span depguardRuleSpan, section string) bool {
	t.Helper()

	key, ok := depguardSectionKey(t, lines, span, section)
	if !ok {
		return false
	}

	_, text := indentOf(lines[key])
	value := strings.TrimSpace(strings.TrimPrefix(text, section+":"))

	return strings.HasPrefix(value, "*")
}

// depguardEntries returns the entry lines of the rule's section (files, allow
// or deny), in file order. A section the rule does not have, or one whose list
// it shares through an alias, has no entry line of its own.
func depguardEntries(t *testing.T, lines []string, span depguardRuleSpan, section string) []depguardEntry {
	t.Helper()

	key, ok := depguardSectionKey(t, lines, span, section)
	if !ok {
		return nil
	}

	var entries []depguardEntry

	for i := key + 1; i < span.end; {
		indent, text := indentOf(lines[i])
		if !isDepguardContent(text) {
			i++

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

		// Resume after the entry's last line: the desc of a deny entry is
		// on the line after its pkg, and reading it as the next entry would
		// end the walk at the first entry of the list.
		i = to
	}

	return entries
}

// depguardInsertPoint returns the line a new entry of the rule's section
// belongs before: after the section's last entry, or right after the key when
// the list has none. It reports false for a section the rule does not have and
// for one it shares through an alias, which has no entries to add to.
func depguardInsertPoint(t *testing.T, lines []string, span depguardRuleSpan, section string) (int, bool) {
	t.Helper()

	key, ok := depguardSectionKey(t, lines, span, section)
	if !ok || depguardSectionAliased(t, lines, span, section) {
		return 0, false
	}

	if entries := depguardEntries(t, lines, span, section); len(entries) > 0 {
		return entries[len(entries)-1].to, true
	}

	return key + 1, true
}

// depguardListMode returns the line of the rule's list-mode key and the mode it
// writes, and whether the rule writes one.
func depguardListMode(t *testing.T, lines []string, span depguardRuleSpan) (int, string, bool) {
	t.Helper()

	for i := span.key + 1; i < span.end; i++ {
		indent, text := indentOf(lines[i])
		if indent == 10 && strings.HasPrefix(text, "list-mode: ") {
			return i, strings.TrimSpace(strings.TrimPrefix(text, "list-mode:")), true
		}
	}

	return 0, "", false
}

// insertDepguardLine returns the lines with text inserted before line at,
// indented by indent spaces.
func insertDepguardLine(lines []string, at, indent int, text string) []string {
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:at]...)
	out = append(out, strings.Repeat(" ", indent)+text)
	out = append(out, lines[at:]...)

	return out
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

// requireMutationReported fails unless the mutated document makes PinnedRules
// report a difference that starts with the rule's name, or, for the mutation
// marked as breaking the document, is refused by the reader.
// depguardMutations has already refused a mutation that changes nothing.
func requireMutationReported(t *testing.T, mutation depguardMutation) {
	t.Helper()

	rules, err := check.DepguardRules([]byte(strings.Join(mutation.lines, "\n")))
	if err != nil {
		if !mutation.wantParseError {
			t.Fatalf("the mutated document is refused with %v, and this is not a mutation the reader may refuse: a parse error here hides the weakening rather than reporting it", err)
		}

		return
	}

	if mutation.wantParseError {
		t.Fatalf("the mutated document reads as %v, and this mutation removes what the rule below it needs: the reader must refuse it, not read a weaker rule", rules)
	}

	if !slices.ContainsFunc(check.PinnedRules(rules, pinnedDepguardRules), func(message string) bool {
		return strings.HasPrefix(message, mutation.rule+": ")
	}) {
		t.Errorf("the mutated document reads as the pinned rules: DepguardRules() = %v and PinnedRules() reported nothing about %s, so the pin would miss this mutation", rules, mutation.rule)
	}
}
