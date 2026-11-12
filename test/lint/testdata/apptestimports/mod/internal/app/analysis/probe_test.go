// Package analysis_test stands for a test of the application layer, whose
// imports app-tests-use-fakes-not-adapters holds to a strict allow list (issue
// #144): the tests use fakes, not adapters, and do no I/O (CLAUDE.md: default
// tests never touch the network). The rule names what the application layer's
// tests may import and refuses everything else, so a package it does not name
// fails lint whether or not anyone thought to deny it. allow_test.go holds every
// import the rule allows; this file holds the ones it must refuse.
//
// Every line here that ends in a want comment holds a package the rule must
// refuse, and no issue may land on a line without one. The first group is
// refused by an entry that names it with a reason. The second does I/O without
// importing os or net, which the reviewer and the auditor found, or is a pure
// package the application tests need no more than they need expvar (runtime,
// go/build/constraint), which only an allow list can refuse; its want names only
// the list, so the entry that refuses it may pick its reason. unsafe is in the
// second group: //go:linkname reaches the refused packages' functions without an
// import, so a test may not import it.
//
// runtime and go/build/constraint were controls before issue #144, and the last
// one pinned that the go/build entry stayed exact (go/build$), since a bare
// go/build prefix entry would also refuse it. A strict allow list needs neither:
// it refuses both, the subtree included, because it names neither. The pure
// packages the tests may use are the ones allow_test.go imports.
//
// path/filepath is not here on purpose: it is pure enough that the rule may
// name it, and the probe holds only what the rule must refuse.
package analysis_test

import (
	"archive/zip"                // want "import 'archive/zip' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"crypto/tls"                 // want "import 'crypto/tls' is not allowed from list 'app-tests-use-fakes-not-adapters': the application layer opens no TLS connections; an adapter behind a port does"
	"debug/buildinfo"            // want "import 'debug/buildinfo' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"debug/elf"                  // want "import 'debug/elf' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"encoding/xml"               // want "import 'encoding/xml' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"expvar"                     // want "import 'expvar' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"go/build"                   // want "import 'go/build' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"go/build/constraint"        // want "import 'go/build/constraint' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"go/importer"                // want "import 'go/importer' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"go/parser"                  // want "import 'go/parser' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	htmltemplate "html/template" // want "import 'html/template' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"io/ioutil"                  // want "import 'io/ioutil' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"log/syslog"                 // want "import 'log/syslog' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"mime/multipart"             // want "import 'mime/multipart' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"net"                        // want "import 'net' is not allowed from list 'app-tests-use-fakes-not-adapters': the application layer has no network access; an adapter behind a port does"
	"net/http"                   // want "import 'net/http' is not allowed from list 'app-tests-use-fakes-not-adapters': the application layer makes no HTTP calls; an adapter behind a port does"
	"net/textproto"              // want "import 'net/textproto' is not allowed from list 'app-tests-use-fakes-not-adapters': the application layer speaks no network protocol; an adapter behind a port does"
	"os"                         // want "import 'os' is not allowed from list 'app-tests-use-fakes-not-adapters': the application layer has no OS access (files, environment); the driving adapter passes what it needs through the ports"
	"os/exec"                    // want "import 'os/exec' is not allowed from list 'app-tests-use-fakes-not-adapters': the application layer starts no processes"
	"plugin"                     // want "import 'plugin' is not allowed from list 'app-tests-use-fakes-not-adapters': the application layer loads no code at run time; the composition root wires everything it needs"
	"runtime"                    // want "import 'runtime' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"runtime/coverage"           // want "import 'runtime/coverage' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"runtime/debug"              // want "import 'runtime/debug' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"syscall"                    // want "import 'syscall' is not allowed from list 'app-tests-use-fakes-not-adapters': the application layer makes no system calls (files, sockets, processes); an adapter behind a port does"
	"text/template"              // want "import 'text/template' is not allowed from list 'app-tests-use-fakes-not-adapters'"
	"unsafe"                     // want "import 'unsafe' is not allowed from list 'app-tests-use-fakes-not-adapters'"

	"github.com/timothymamo/shift2eu/internal/adapter/report/markdown" // want "import 'github.com/timothymamo/shift2eu/internal/adapter/report/markdown' is not allowed from list 'app-tests-use-fakes-not-adapters'"
)

// The refused packages are imported to be judged: one symbol of each keeps the
// import used, which the type checker requires.
var (
	_ = zip.NewReader
	_ = tls.Dial
	_ = buildinfo.ReadFile
	_ = elf.Open
	_ = xml.Marshal
	_ = expvar.NewInt
	_ = build.Default
	_ = constraint.Parse
	_ = importer.ForCompiler
	_ = parser.ParseFile
	_ = htmltemplate.New
	_ = ioutil.ReadFile
	_ = syslog.Dial
	_ = multipart.NewReader
	_ = net.Dial
	_ = http.Get
	_ = textproto.Dial
	_ = os.Open
	_ = exec.Command
	_ = plugin.Open
	_ = runtime.GOMAXPROCS
	_ = coverage.WriteMetaDir
	_ = debug.ReadBuildInfo
	_ = syscall.Open
	_ = template.New
	_ = unsafe.Sizeof
	_ = markdown.Render
)
