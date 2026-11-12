// Package analysis_test stands for a test of the application layer, which
// app-tests-use-fakes-not-adapters holds to the shared deny list: the tests use
// fakes, not adapters, and do no I/O (CLAUDE.md: default tests never touch the
// network). The rule is lax, so its deny list alone decides what is refused.
//
// Every line that ends in a want comment names the list this rule uses. The
// first group is refused by an entry that names it with a reason; the second
// does I/O without importing os or net, which the reviewer and the auditor
// found, and its want names only the list, so the entry that refuses it may
// pick its reason. The control imports below are pure and must not be
// reported. path/filepath stays allowed on purpose (Join and Base are pure) and
// is not pinned either way.
package analysis_test

import (
	"archive/zip" // want "import 'archive/zip' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"context"
	"crypto/tls"                 // want "import 'crypto/tls' is not allowed from list 'app-tests-use-fakes-not-adapters': the application layer opens no TLS connections; an adapter behind a port does"
	"debug/buildinfo"            // want "import 'debug/buildinfo' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"debug/elf"                  // want "import 'debug/elf' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"go/build"                   // want "import 'go/build' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"go/importer"                // want "import 'go/importer' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	htmltemplate "html/template" // want "import 'html/template' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"io/ioutil"                  // want "import 'io/ioutil' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"log/syslog"                 // want "import 'log/syslog' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"net"                        // want "import 'net' is not allowed from list 'app-tests-use-fakes-not-adapters': the application layer has no network access; an adapter behind a port does"
	"net/http"                   // want "import 'net/http' is not allowed from list 'app-tests-use-fakes-not-adapters': the application layer makes no HTTP calls; an adapter behind a port does"
	"net/netip"
	"net/textproto" // want "import 'net/textproto' is not allowed from list 'app-tests-use-fakes-not-adapters': the application layer speaks no network protocol; an adapter behind a port does"
	"net/url"
	"os"      // want "import 'os' is not allowed from list 'app-tests-use-fakes-not-adapters': the application layer has no OS access (files, environment); the driving adapter passes what it needs through the ports"
	"os/exec" // want "import 'os/exec' is not allowed from list 'app-tests-use-fakes-not-adapters': the application layer starts no processes"
	"plugin"  // want "import 'plugin' is not allowed from list 'app-tests-use-fakes-not-adapters': the application layer loads no code at run time; the composition root wires everything it needs"
	"runtime"
	"runtime/debug" // want "import 'runtime/debug' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"strings"
	"syscall" // want "import 'syscall' is not allowed from list 'app-tests-use-fakes-not-adapters': the application layer makes no system calls (files, sockets, processes); an adapter behind a port does"
	"testing"
	"text/template" // want "import 'text/template' is not allowed from list 'app-tests-use-fakes-not-adapters'"
)

// The refused packages are imported to be judged: one symbol of each keeps the
// import used, which the type checker requires.
var (
	_ = zip.NewReader
	_ = tls.Dial
	_ = buildinfo.ReadFile
	_ = elf.Open
	_ = build.Default
	_ = importer.ForCompiler
	_ = htmltemplate.New
	_ = ioutil.ReadFile
	_ = syslog.Dial
	_ = net.Dial
	_ = http.Get
	_ = textproto.Dial
	_ = os.Open
	_ = exec.Command
	_ = plugin.Open
	_ = debug.ReadBuildInfo
	_ = syscall.Open
	_ = template.New
)

// TestProbe uses the packages the rule must leave alone: net/url and net/netip
// parse values, and strings, context and runtime are pure. The test itself
// touches no network and no file, as the rule requires.
func TestProbe(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("https://example.com/a%20b")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	if got, want := strings.ToUpper(u.Host), "EXAMPLE.COM"; got != want {
		t.Errorf("strings.ToUpper(%q) = %q, want %q", u.Host, got, want)
	}
	if ctx := context.Background(); ctx.Err() != nil {
		t.Errorf("context.Background().Err() = %v, want nil", ctx.Err())
	}
	t.Logf("netip %s, runtime %s", netip.MustParseAddr("192.0.2.1"), runtime.Version())
}
