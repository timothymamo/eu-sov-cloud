package lint_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// writeVerbs start the name of a method that can write to a cloud. It is a
// denylist, and always misses a verb, so it applies where an allowlist cannot:
// to an interface outside the cloud source adapters, and to the last entry of
// forbidigo's cloud-read-only group in .golangci.yml, which refuses the same
// verbs on the types of a source adapter. TestWriteVerbsMatchForbidigo keeps
// the two lists the same.
var writeVerbs = []string{
	"Abandon", "Abort", "Accept", "Acquire", "Activate", "Add", "Advertise", "Allocate", "Apply", "Archive",
	"Assign", "Associate", "Assume", "Attach", "Authorize", "Backtrack", "Batch", "Begin", "Break", "Bulk",
	"Bundle", "Cancel", "Change", "Clear", "Commit", "Complete", "Confirm", "Continue", "Copy", "Create",
	"Deactivate", "Deallocate", "Decrypt", "Delete", "Deploy", "Deprecate", "Deprovision", "Deregister",
	"Detach", "Disable", "Disassociate", "Enable", "Execute", "Expand", "Export", "Failover", "Flush",
	"Generalize", "Generate", "Grant", "Import", "Initiate", "Insert", "Invoke", "Leave", "Lock", "Merge",
	"Modify", "Monitor", "Move", "Patch", "Perform", "Power", "Promote", "Provision", "Publish", "Purchase",
	"Purge", "Put", "Reapply", "Reboot", "Receive", "Recreate", "Redeploy", "Regenerate", "Register", "Reimage",
	"Reject", "Release", "Remove", "Renew", "Replace", "Request", "Reset", "Resize", "Restart", "Restore",
	"Resume", "Revoke", "Rollback", "Rotate", "Run", "Scale", "Send", "Set", "Sign", "Signal", "Simulate",
	"Split", "Start", "Stop", "Subscribe", "Suspend", "Swap", "Switch", "Tag", "Terminate", "Truncate",
	"Unarchive", "Unassign", "Undelete", "Unlock", "Unmonitor", "Unsubscribe", "Untag", "Update", "Upgrade",
	"Upload", "Upsert", "Withdraw", "Write",
}

// verbAlternation is writeVerbs as a regular expression alternation.
var verbAlternation = "(?:" + strings.Join(writeVerbs, "|") + ")"

// writeName matches a name that starts with a write verb, as a whole word:
// TerminateInstances and Delete, not Settings or Address.
var writeName = regexp.MustCompile(`^` + verbAlternation + `(?:[A-Z0-9]\w*)?$`)

// cloudSDKs are the import paths of the cloud SDKs and of the auth and
// transport packages they are built on. Each covers that path and every path
// under it: github.com/aws covers aws-sdk-go-v2 and smithy-go.
var cloudSDKs = []string{
	"github.com/aws",
	"github.com/Azure",
	"cloud.google.com/go",
	"google.golang.org/api",
	"google.golang.org/grpc",
	"golang.org/x/oauth2",
}

func isCloudSDK(importPath string) bool {
	for _, sdk := range cloudSDKs {
		if importPath == sdk || strings.HasPrefix(importPath, sdk+"/") {
			return true
		}
	}
	return false
}

// readName matches the name of an interface method a cloud source adapter may
// declare: an exported read verb, as a whole word, or an unexported name, which
// no SDK client method has.
var readName = regexp.MustCompile(`^(?:(?:Describe|Get|List|NewGet|NewList|AggregatedList)(?:[A-Z0-9]\w*)?|Close|HasMorePages|NextPage|More|[^A-Z].*)$`)

// The messages of interfaceFindings, which a probe's want comment names.
const (
	msgSourceInterface = astPrefix + "an interface of a cloud source adapter declares only read verbs"
	msgWriteInterface  = astPrefix + "an interface method is named for a write"
)

// inCloudSourceAdapter reports whether the file at rel, relative to the module
// root and slash-separated, is in a cloud source adapter package, or a sibling
// of one: a package under internal/adapter/source other than inventoryfile and
// sourcetest.
func inCloudSourceAdapter(rel string) bool {
	rest, ok := strings.CutPrefix(rel, "internal/adapter/source/")
	if !ok {
		return false
	}
	dir, _, ok := strings.Cut(rest, "/")
	return ok && dir != "inventoryfile" && dir != "sourcetest"
}

// finding is a rule test/lint checks on the AST, broken at pos.
type finding struct {
	pos, msg, detail string
}

// interfaceFindings returns every interface method declared in files, tests
// included, that could carry a write: a named interface, an interface literal
// and a type-parameter constraint alike. An SDK client satisfies such an
// interface without the operation type being named when type inference fills
// it in, and forbidigo cannot see a call through an interface literal or a type
// parameter at all. So in a cloud source adapter, where an SDK client is at
// hand, an exported method must be named for a read (an allowlist); anywhere
// else, it must not be named for a write (writeVerbs, a denylist), because a
// source adapter can import only the project packages its depguard rule lists.
func interfaceFindings(t *testing.T, files []goFile) []finding {
	t.Helper()

	var found []finding
	for _, f := range files {
		source := inCloudSourceAdapter(f.rel)
		fset, file := parseGoFile(t, f)
		ast.Inspect(file, func(n ast.Node) bool {
			iface, ok := n.(*ast.InterfaceType)
			if !ok {
				return true
			}
			for _, method := range iface.Methods.List {
				for _, name := range method.Names {
					switch {
					case source && !readName.MatchString(name.Name):
						found = append(found, finding{position(fset, f, name), msgSourceInterface, fmt.Sprintf(
							"%s is not a read verb (Describe, Get, List, NewGet, NewList, AggregatedList, Close, "+
								"HasMorePages, NextPage, More). An SDK client satisfies the interface, and a write "+
								"through it, a type parameter or an interface literal gets past forbidigo's "+
								"cloud-read-only rule; call the SDK client directly, or unexport the method",
							name.Name)})
					case !source && writeName.MatchString(name.Name):
						found = append(found, finding{position(fset, f, name), msgWriteInterface, fmt.Sprintf(
							"%s is named for a write. An SDK client satisfies the interface, and a write through "+
								"it gets past forbidigo's cloud-read-only rule; name the method with another verb",
							name.Name)})
					}
				}
			}
			return true
		})
	}
	return found
}

// TestNoInterfaceDeclaresACloudWrite fails on every interface method of the
// module that interfaceFindings returns.
func TestNoInterfaceDeclaresACloudWrite(t *testing.T) {
	t.Parallel()

	for _, f := range interfaceFindings(t, moduleGoFiles(t, moduleRoot(t))) {
		t.Errorf("%s: %s: %s", f.pos, f.msg, f.detail)
	}
}

// TestInterfaceFindingsProbe runs interfaceFindings on the probe module in
// testdata/cloudreadonly/mod, where every line with a want comment that names a
// test/lint message must be found, and no other line: the generic constraints
// of the review (RequestSpotInstances and LeaveOrganization with their types
// inferred), an interface literal and an adapter interface that spells a write
// out are refused, and read and unexported methods are not.
func TestInterfaceFindingsProbe(t *testing.T) {
	t.Parallel()

	mod := filepath.Join(moduleRoot(t), "test", "lint", "testdata", "cloudreadonly", "mod")
	want := wantMessages(t, mod, func(prefix string) bool { return strings.HasPrefix(prefix, astPrefix) })
	if len(want) == 0 {
		t.Fatal("the probe holds no test/lint want comment")
	}
	got := map[string][]string{}
	for _, f := range interfaceFindings(t, moduleGoFiles(t, mod)) {
		line := f.pos[:strings.LastIndex(f.pos, ":")] // file:line, without the column
		got[line] = append(got[line], f.msg)
	}
	for _, line := range slices.Sorted(maps.Keys(want)) {
		if !slices.Equal(want[line], got[line]) {
			t.Errorf("%s: want %q, got %q", line, want[line], got[line])
		}
	}
	for _, line := range slices.Sorted(maps.Keys(got)) {
		if _, ok := want[line]; !ok {
			t.Errorf("%s: %q, which the probe does not expect", line, got[line])
		}
	}
}

// TestNoTypeHidesACloudSDKType fails on every struct in the module, tests
// included, that embeds a type from a cloud SDK, on every type alias of one,
// and on every dot import of a cloud SDK package. forbidigo matches a method
// promoted from an embedded client against the embedding type, which has no
// name when the struct is a literal (struct{ *ec2.Client }), so a write
// through it is not refused; an alias or a dot import would hide the embedded
// type from this test.
func TestNoTypeHidesACloudSDKType(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	names := packageNames(t, root)
	for _, f := range moduleGoFiles(t, root) {
		fset, file := parseGoFile(t, f)
		imports := map[string]string{} // the name a file uses for an import, to its path
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("%s: import %s: %v", f.rel, spec.Path.Value, err)
			}
			name, ok := names[path]
			if !ok {
				name = guessPackageName(path)
			}
			if spec.Name != nil {
				name = spec.Name.Name
			}
			if name == "." && isCloudSDK(path) {
				t.Errorf("%s: dot import of %s hides which names come from a cloud SDK; import it by name",
					position(fset, f, spec), path)
			}
			imports[name] = path
		}
		fromSDK := func(expr ast.Expr) (string, bool) {
			sel, ok := typeName(expr).(*ast.SelectorExpr)
			if !ok {
				return "", false
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || !isCloudSDK(imports[pkg.Name]) {
				return "", false
			}
			return pkg.Name + "." + sel.Sel.Name, true
		}

		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.StructType:
				for _, field := range n.Fields.List {
					if len(field.Names) > 0 {
						continue
					}
					if name, ok := fromSDK(field.Type); ok {
						t.Errorf("%s: a struct embeds %s, and a write through a method it promotes gets past "+
							"forbidigo's cloud-read-only rule; hold the SDK type in a named field",
							position(fset, f, field), name)
					}
				}
			case *ast.TypeSpec:
				if !n.Assign.IsValid() {
					return true
				}
				if name, ok := fromSDK(n.Type); ok {
					t.Errorf("%s: type %s is an alias of %s, which would let a struct embed the SDK type "+
						"unseen; name the SDK type itself", position(fset, f, n), n.Name.Name, name)
				}
			}
			return true
		})
	}
}

// TestWriteVerbsMatchForbidigo fails when the forbidigo entry that refuses a
// method named for a write on a cloud source adapter's own types holds other
// verbs than writeVerbs.
func TestWriteVerbsMatchForbidigo(t *testing.T) {
	t.Parallel()

	const pkgLine = `pkg: '^github\.com/timothymamo/shift2eu/internal/adapter/source/'`
	want := `- pattern: '^\w+\.\w+\.` + verbAlternation + `(?:[A-Z0-9]\w*)?$'`

	config, err := os.ReadFile(filepath.Join(moduleRoot(t), ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(config), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != pkgLine {
			continue
		}
		if i == 0 || strings.TrimSpace(lines[i-1]) != want {
			t.Errorf(".golangci.yml:%d: the entry's pattern differs from writeVerbs in %s; change both.\n"+
				"got:  %s\nwant: %s", i, "test/lint/sourcewrites_test.go", strings.TrimSpace(lines[max(i-1, 0)]), want)
		}
		return
	}
	t.Errorf(".golangci.yml has no line %s, the pkg of the entry that refuses a method named for a write on "+
		"a cloud source adapter's own types", pkgLine)
}

func TestWriteName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{name: "TerminateInstances", want: true},
		{name: "Delete", want: true},
		{name: "PutObject", want: true},
		{name: "Run", want: true},
		{name: "Set2", want: true},
		{name: "BeginCreateOrUpdate", want: true},
		{name: "RequestSpotInstances", want: true},
		{name: "LeaveOrganization", want: true},
		{name: "Decrypt", want: true},
		{name: "PurchaseReservedInstancesOffering", want: true},
		{name: "DescribeInstances", want: false},
		{name: "GetObject", want: false},
		{name: "NewListPager", want: false},
		{name: "Settings", want: false},
		{name: "Address", want: false},
		{name: "Runtime", want: false},
		{name: "terminateInstances", want: false},
		{name: "Inventory", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := writeName.MatchString(tc.name); got != tc.want {
				t.Errorf("writeName.MatchString(%q) = %t, want %t", tc.name, got, tc.want)
			}
		})
	}
}

func TestReadName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{name: "DescribeInstances", want: true},
		{name: "GetObject", want: true},
		{name: "List", want: true},
		{name: "NewListPager", want: true},
		{name: "AggregatedList", want: true},
		{name: "Close", want: true},
		{name: "HasMorePages", want: true},
		{name: "NextPage", want: true},
		{name: "More", want: true},
		{name: "terminate", want: true},
		{name: "RequestSpotInstances", want: false},
		{name: "LeaveOrganization", want: false},
		{name: "Getaway", want: false},
		{name: "Listen", want: false},
		{name: "Describes", want: false},
		{name: "Closer", want: false},
		{name: "Inventory", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := readName.MatchString(tc.name); got != tc.want {
				t.Errorf("readName.MatchString(%q) = %t, want %t", tc.name, got, tc.want)
			}
		})
	}
}

// packageNames maps the import path of every package the module's code
// imports, tests included, to its package name, which can differ from the
// last element of its path (cloud.google.com/go/compute/apiv1 is compute).
func packageNames(t *testing.T, root string) map[string]string {
	t.Helper()

	names := map[string]string{}
	for _, p := range goList(t, root, "ImportPath,Name", "-deps", "-test", "./...") {
		if p.Name != "" {
			names[p.ImportPath] = p.Name
		}
	}
	return names
}

// majorVersion matches the last element of a major-version import path.
var majorVersion = regexp.MustCompile(`^v[0-9]+$`)

// guessPackageName returns the package name of an import go list could not
// load: the last element of its path that is not a major version.
func guessPackageName(importPath string) string {
	elems := strings.Split(importPath, "/")
	if n := len(elems); n > 1 && majorVersion.MatchString(elems[n-1]) {
		return elems[n-2]
	}
	return elems[len(elems)-1]
}

// typeName strips a pointer and type arguments off a type expression.
func typeName(expr ast.Expr) ast.Expr {
	for {
		switch e := expr.(type) {
		case *ast.StarExpr:
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.IndexListExpr:
			expr = e.X
		case *ast.ParenExpr:
			expr = e.X
		default:
			return expr
		}
	}
}

func parseGoFile(t *testing.T, f goFile) (*token.FileSet, *ast.File) {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, f.rel, f.src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", f.rel, err)
	}
	return fset, file
}

func position(fset *token.FileSet, f goFile, n ast.Node) string {
	pos := fset.Position(n.Pos())
	return fmt.Sprintf("%s:%d:%d", f.rel, pos.Line, pos.Column)
}
