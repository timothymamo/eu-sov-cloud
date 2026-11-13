package execenv

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Finding is one place in a source file that breaks a rule of this package.
type Finding struct {
	// File is the file name the caller passed, unchanged.
	File string
	// Line is the 1-based line a reader fixes.
	Line int
	// Msg says what is wrong and how to fix it.
	Msg string
}

// execPath is the import path of the package whose commands CheckExecEnv
// follows.
const execPath = "os/exec"

// runMethods are the methods of *exec.Cmd that start the child.
var runMethods = map[string]bool{
	"Run":            true,
	"Start":          true,
	"Output":         true,
	"CombinedOutput": true,
}

// CheckExecEnv returns a finding, in source order, for every place in the Go
// source src where a command of os/exec may start with a nil Env, which
// inherits the whole environment of the test process. It fails closed: a
// command it cannot follow, and a use of a command it cannot vouch for, is a
// finding too. filename names the file in the findings and in the parse
// error.
//
// A command is followed when exec.Command or exec.CommandContext builds it as
// the whole right-hand side of `ident :=`, `ident =` or `var ident =` inside a
// function, and it is keyed by the variable the identifier declares, so a
// shadowing name in an inner block is another command. Until an Env that is
// not nil (nor []string(nil)) is assigned to it, the only uses accepted are
// `ident.<Field> = ...` and, for an exempt command, a Run, Start, Output or
// CombinedOutput call on it; every other use (an alias, an argument, a method
// value or expression, a capture in a function literal, a run call) is a
// finding. Once Env is set, every use is accepted. The check follows the
// source, not the flow: an Env assigned anywhere before the use counts, in a
// branch that never runs too.
//
// Also refused: a construction bound any other way (a call argument, a
// composite element, a struct field, a chained call), unless it is an exempt
// command whose run method is called on it in the same expression; an
// exec.Cmd composite literal with no Env key, or with a nil one; new(exec.Cmd);
// and a blank or dot import of os/exec.
//
// exempt is handed the command name, the first argument of exec.Command and the
// second of exec.CommandContext, and sub, the argument after it, when both are
// string literals; a command it admits may run with no Env. A missing or
// non-literal name or sub is never exempt, and assigning Path or Args to an
// exempt command takes the exemption away.
func CheckExecEnv(filename string, src []byte, exempt func(name, sub string) bool) ([]Finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}

	c := &execChecker{
		fset:     fset,
		filename: filename,
		exempt:   exempt,
		commands: map[types.Object]*command{},
	}
	if !c.imports(file) {
		return c.findings, nil
	}
	c.info = resolve(fset, file)
	ast.Inspect(file, c.visit)
	return c.findings, nil
}

// resolve type-checks file on its own, for the objects its identifiers
// declare and use. Every import resolves to an empty package, so the check
// reports errors (exec.Command among them is undefined), which it ignores:
// what it needs is which variable each identifier names, and that the check
// records whatever the types it cannot see.
func resolve(fset *token.FileSet, file *ast.File) *types.Info {
	info := &types.Info{
		Defs: map[*ast.Ident]types.Object{},
		Uses: map[*ast.Ident]types.Object{},
	}
	conf := types.Config{
		Importer: emptyImporter{},
		Error:    func(error) {},
	}
	// The errors are expected, see above.
	_, _ = conf.Check(file.Name.Name, fset, []*ast.File{file}, info)
	return info
}

// emptyImporter imports every path as a complete package with no members,
// named after the path's last element, which is what os/exec needs.
type emptyImporter struct{}

func (emptyImporter) Import(importPath string) (*types.Package, error) {
	pkg := types.NewPackage(importPath, path.Base(importPath))
	pkg.MarkComplete()
	return pkg, nil
}

// execChecker holds what CheckExecEnv needs while it walks one file.
type execChecker struct {
	fset     *token.FileSet
	filename string
	info     *types.Info
	exempt   func(name, sub string) bool
	// commands are the commands followed so far, by the variable that holds
	// each.
	commands map[types.Object]*command
	// funcs counts the function bodies the walk is inside, a function
	// literal's included; a construction outside every one is no command the
	// checker can follow.
	funcs int
	// lits are the function literals the walk is inside, innermost last.
	lits     []*ast.FuncLit
	findings []Finding
}

// command is what the checker knows of a variable that holds an *exec.Cmd
// built in the file.
type command struct {
	// exempt is whether the command's name and subcommand are literals exempt
	// admits, and neither Path nor Args has been assigned since.
	exempt bool
	// envSet is whether an Env that is not nil was assigned since the command
	// was built.
	envSet bool
	// lit is the innermost function literal the command was built in, nil for
	// a function declaration's own body. A use inside another literal captures
	// it.
	lit *ast.FuncLit
}

// imports reports whether file imports os/exec under a name the checker can
// follow, and refuses a blank or a dot import of it.
func (c *execChecker) imports(file *ast.File) bool {
	named := false
	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != execPath {
			continue
		}
		switch {
		case imp.Name != nil && imp.Name.Name == "_":
			c.report(imp.Pos(), "a blank import of os/exec hides the commands this check follows; import os/exec by its name")
		case imp.Name != nil && imp.Name.Name == ".":
			c.report(imp.Pos(), "a dot import of os/exec hides the commands this check follows; import os/exec by its name")
		default:
			named = true
		}
	}
	return named
}

// visit is the ast.Inspect function of the walk. It handles the nodes the rule
// is about and walks their parts itself, in source order, so that what it
// already knows of a command holds at every use.
func (c *execChecker) visit(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.FuncDecl:
		if n.Body != nil {
			c.funcs++
			ast.Inspect(n.Body, c.visit)
			c.funcs--
		}
		return false
	case *ast.FuncLit:
		c.funcs++
		c.lits = append(c.lits, n)
		ast.Inspect(n.Body, c.visit)
		c.lits = c.lits[:len(c.lits)-1]
		c.funcs--
		return false
	case *ast.AssignStmt:
		c.assign(n.Lhs, n.Rhs)
		return false
	case *ast.ValueSpec:
		lhs := make([]ast.Expr, len(n.Names))
		for i, name := range n.Names {
			lhs[i] = name
		}
		c.assign(lhs, n.Values)
		return false
	case *ast.CallExpr:
		return c.call(n)
	case *ast.CompositeLit:
		c.composite(n)
		return true
	case *ast.Ident:
		c.use(n)
		return true
	}
	return true
}

// binding is a construction an assignment binds to a name.
type binding struct {
	exempt bool
}

// assign walks an assignment or a var declaration: the right-hand side first,
// as Go evaluates it, then the targets, which bind the commands it builds,
// forget the names it rebinds to something else, and record the fields it sets.
func (c *execChecker) assign(lhs, rhs []ast.Expr) {
	paired := len(lhs) == len(rhs)
	bound := make([]*binding, len(lhs))
	for i, value := range rhs {
		if paired && c.funcs > 0 && isName(lhs[i]) {
			if call, ok := ast.Unparen(value).(*ast.CallExpr); ok {
				if exempt, ok := c.construction(call); ok {
					bound[i] = &binding{exempt: exempt}
					c.walkArgs(call)
					continue
				}
			}
		}
		ast.Inspect(value, c.visit)
	}

	for i, target := range lhs {
		switch target := ast.Unparen(target).(type) {
		case *ast.Ident:
			obj := c.object(target)
			if obj == nil {
				continue
			}
			if bound[i] != nil {
				c.commands[obj] = &command{exempt: bound[i].exempt, lit: c.lit()}
				continue
			}
			// The name now holds something else: a command built
			// elsewhere in the file was checked where it was built.
			delete(c.commands, obj)
		case *ast.SelectorExpr:
			id, cmd := c.command(target.X)
			if cmd == nil {
				ast.Inspect(target, c.visit)
				continue
			}
			if c.captured(cmd) && !cmd.envSet {
				c.report(id.Pos(), fmt.Sprintf(
					"%s is captured by a function literal before its Env is set, so the check cannot tell what runs it; assign %s.Env before the literal",
					id.Name, id.Name))
			}
			switch target.Sel.Name {
			case "Env":
				cmd.envSet = !paired || !isNil(c.info, rhs[i])
			case "Path", "Args":
				cmd.exempt = false
			}
		default:
			ast.Inspect(target, c.visit)
		}
	}
}

// call handles a call expression and reports whether the walk should descend
// into it.
func (c *execChecker) call(call *ast.CallExpr) bool {
	if c.isNewCmd(call) {
		c.report(call.Pos(), "new(exec.Cmd) builds a command with a nil Env, which inherits the whole environment of the test process; build it with exec.Command and assign its Env")
		return true
	}

	if _, ok := c.construction(call); ok {
		c.report(call.Pos(), "the command is not bound as the whole right-hand side of a variable inside a function, so the check cannot follow its Env; bind it with `cmd := exec.Command(...)` and assign cmd.Env before it runs")
		c.walkArgs(call)
		return false
	}

	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || !runMethods[sel.Sel.Name] {
		return true
	}

	// A command built and run in one expression.
	if inner, ok := ast.Unparen(sel.X).(*ast.CallExpr); ok {
		if exempt, ok := c.construction(inner); ok {
			if !exempt {
				c.report(sel.Sel.Pos(), fmt.Sprintf(
					"%s is called on a command built in the same expression, whose Env is nil and inherits the whole environment of the test process; bind the command to a variable and assign its Env first",
					sel.Sel.Name))
			}
			c.walkArgs(inner)
			for _, arg := range call.Args {
				ast.Inspect(arg, c.visit)
			}
			return false
		}
	}

	id, cmd := c.command(sel.X)
	if cmd == nil {
		return true
	}
	switch {
	case cmd.envSet:
	case c.captured(cmd):
		c.report(sel.Sel.Pos(), fmt.Sprintf(
			"%s.%s runs, inside a function literal, a command whose Env is not set; assign %s.Env before the literal",
			id.Name, sel.Sel.Name, id.Name))
	case !cmd.exempt:
		c.report(sel.Sel.Pos(), fmt.Sprintf(
			"%s.%s runs a command whose Env is nil, which inherits the whole environment of the test process; assign %s.Env before the call",
			id.Name, sel.Sel.Name, id.Name))
	}
	for _, arg := range call.Args {
		ast.Inspect(arg, c.visit)
	}
	return false
}

// composite refuses an exec.Cmd composite literal that sets no Env, or a nil
// one.
func (c *execChecker) composite(lit *ast.CompositeLit) {
	if lit.Type == nil || !c.isCmdType(lit.Type) {
		return
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Env" && !isNil(c.info, kv.Value) {
			return
		}
	}
	c.report(lit.Pos(), "an exec.Cmd composite literal with no Env, or a nil one, inherits the whole environment of the test process; give it an Env key")
}

// use refuses an identifier that names a followed command whose Env is not set
// yet, where no other case of the walk has accepted it.
func (c *execChecker) use(id *ast.Ident) {
	cmd := c.commands[c.object(id)]
	if cmd == nil || cmd.envSet {
		return
	}
	c.report(id.Pos(), fmt.Sprintf(
		"%s is used before its Env is set (copied, passed on, captured or taken as a method), so the check cannot follow what runs it; assign %s.Env first",
		id.Name, id.Name))
}

// walkArgs walks the arguments of a construction, which the walk reads as
// any other expression.
func (c *execChecker) walkArgs(call *ast.CallExpr) {
	for _, arg := range call.Args {
		ast.Inspect(arg, c.visit)
	}
}

// construction reports whether call is exec.Command or exec.CommandContext,
// and if so whether the command it builds is exempt.
func (c *execChecker) construction(call *ast.CallExpr) (exempt, ok bool) {
	sel, isSel := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !isSel || !c.isExec(sel.X) {
		return false, false
	}
	var nameArg int
	switch sel.Sel.Name {
	case "Command":
		nameArg = 0
	case "CommandContext":
		nameArg = 1
	default:
		return false, false
	}
	if len(call.Args) <= nameArg+1 {
		return false, true
	}
	name, nameOK := stringLit(call.Args[nameArg])
	sub, subOK := stringLit(call.Args[nameArg+1])
	if !nameOK || !subOK {
		return false, true
	}
	return c.exempt(name, sub), true
}

// isExec reports whether x names the os/exec package.
func (c *execChecker) isExec(x ast.Expr) bool {
	id, ok := ast.Unparen(x).(*ast.Ident)
	if !ok {
		return false
	}
	pkg, ok := c.info.Uses[id].(*types.PkgName)
	return ok && pkg.Imported().Path() == execPath
}

// isCmdType reports whether t is exec.Cmd.
func (c *execChecker) isCmdType(t ast.Expr) bool {
	sel, ok := ast.Unparen(t).(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Cmd" && c.isExec(sel.X)
}

// isNewCmd reports whether call is new(exec.Cmd).
func (c *execChecker) isNewCmd(call *ast.CallExpr) bool {
	id, ok := ast.Unparen(call.Fun).(*ast.Ident)
	if !ok || id.Name != "new" || len(call.Args) != 1 {
		return false
	}
	if _, builtin := c.info.Uses[id].(*types.Builtin); !builtin {
		return false
	}
	return c.isCmdType(call.Args[0])
}

// object returns the variable id declares or uses, or nil.
func (c *execChecker) object(id *ast.Ident) types.Object {
	if obj := c.info.Defs[id]; obj != nil {
		return obj
	}
	return c.info.Uses[id]
}

// command returns the identifier x is, through parentheses, and the command
// it holds, or a nil command when x names no followed command.
func (c *execChecker) command(x ast.Expr) (*ast.Ident, *command) {
	id, ok := ast.Unparen(x).(*ast.Ident)
	if !ok {
		return nil, nil
	}
	obj := c.object(id)
	if obj == nil {
		return id, nil
	}
	return id, c.commands[obj]
}

// lit returns the innermost function literal the walk is in, or nil.
func (c *execChecker) lit() *ast.FuncLit {
	if len(c.lits) == 0 {
		return nil
	}
	return c.lits[len(c.lits)-1]
}

// captured reports whether a use of cmd at the walk's position is inside a
// function literal the command was not built in.
func (c *execChecker) captured(cmd *command) bool {
	return c.lit() != cmd.lit
}

func (c *execChecker) report(pos token.Pos, msg string) {
	c.findings = append(c.findings, Finding{
		File: c.filename,
		Line: c.fset.Position(pos).Line,
		Msg:  msg,
	})
}

// isName reports whether x is an identifier other than the blank one.
func isName(x ast.Expr) bool {
	id, ok := ast.Unparen(x).(*ast.Ident)
	return ok && id.Name != "_"
}

// isNil reports whether x is nil, or a conversion of nil such as
// []string(nil): an Env with that value inherits the environment.
func isNil(info *types.Info, x ast.Expr) bool {
	switch x := ast.Unparen(x).(type) {
	case *ast.Ident:
		_, ok := info.Uses[x].(*types.Nil)
		return ok
	case *ast.CallExpr:
		if _, ok := ast.Unparen(x.Fun).(*ast.ArrayType); ok && len(x.Args) == 1 {
			return isNil(info, x.Args[0])
		}
	}
	return false
}

// stringLit returns the value of x when it is a string literal.
func stringLit(x ast.Expr) (string, bool) {
	lit, ok := ast.Unparen(x).(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

// Tier is the part of test/ a file is read in, which decides the rule
// CheckEnvironLiterals holds its literals to.
type Tier int

const (
	// TierTest is every Go file under test/ outside test/e2e, where a probe's
	// source and an expected message are literals of their own: a literal is
	// refused only when its whole value, cleaned, names a
	// /proc/<something>/environ file.
	TierTest Tier = iota
	// TierE2E is the e2e tier, which holds no fixture literals: a literal is
	// refused when it names any path into /proc or /dev/fd
	// (EnvironmentPathIn).
	TierE2E
)

// String names the tier in a test failure.
func (t Tier) String() string {
	switch t {
	case TierTest:
		return "test"
	case TierE2E:
		return "e2e"
	}
	return "Tier(" + strconv.Itoa(int(t)) + ")"
}

// environPath matches a cleaned path that names the environ file of a
// process, at any depth below /proc (/proc/self/task/1/environ too), with any
// number of leading slashes or none (os.DirFS("/") opens it without one).
var environPath = regexp.MustCompile(`^/*proc/(?:[^/]+/)+environ$`)

// environmentPathLiterals are the path segments that hold every environment
// variable of a process: its own /proc entry, where the environ file sits, the
// /proc entries of every other process, and /dev/fd, the descriptor directory,
// whose numbers include the one environ is. Reading one dumps the environment,
// credentials included (CLAUDE.md rule 3), and no forbidigo group can say so: the
// call is os.ReadFile, or io.ReadAll over an os.Open, and its argument is a
// string.
//
// Each is written without its leading slash and matched as a whole path segment,
// at the start of a literal value or after a "/" and ending the value or before a
// "/": the directory itself is as much a way in as a file below it, which
// os.DirFS("/proc").Open("self/environ"), os.Open("/dev/fd") and
// os.OpenRoot("/proc") all do, while a value that only ends in such a segment
// (myproc/self/environ, /procfs) is no such path. The segment form is also why
// os.DirFS("/") with fs.ReadFile("proc/self/environ"), a path relative to the
// root, names the same file as "/proc/self/environ".
var environmentPathLiterals = []string{"proc", "dev/fd"}

// environmentPathSegment matches one of environmentPathLiterals as a whole path
// segment of a value. It is built from that list, so an entry added there is
// matched without a second edit, and its entries hold no regular-expression
// metacharacter.
var environmentPathSegment = regexp.MustCompile(`(?:^|/)(` + strings.Join(environmentPathLiterals, "|") + `)(?:/|$)`)

// EnvironmentPathIn returns the path segment that value names among those that
// hold the environment of a process ("proc" and "dev/fd"), written without its
// leading slash, and whether it names one. A segment matches whole, at the
// start of the value or after a "/", so a value that only ends in one
// (myproc/self/environ) or only starts with one (/procfs) is no match. It is
// the rule of the composition roots' literals in test/lint and of the e2e tier
// in CheckEnvironLiterals.
func EnvironmentPathIn(value string) (segment string, found bool) {
	match := environmentPathSegment.FindStringSubmatch(value)
	if match == nil {
		return "", false
	}
	return match[1], true
}

// CheckEnvironLiterals returns a finding, in source order, for every string
// literal in the Go source src that names the environment of a process under
// the rule of tier: for TierTest, a literal whose cleaned value is a
// /proc/.../environ path; for TierE2E, a literal with a proc or dev/fd path
// segment (EnvironmentPathIn). A literal is read as the compiler reads it, so a
// raw and a quoted string are alike, and one that does not unquote is skipped.
// Comments are no literals and pass. A path built at run time is beyond a rule
// of literals. filename names the file in the findings and in the parse error.
func CheckEnvironLiterals(filename string, src []byte, tier Tier) ([]Finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}

	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		if msg, refused := environMessage(value, tier); refused {
			findings = append(findings, Finding{
				File: filename,
				Line: fset.Position(lit.Pos()).Line,
				Msg:  msg,
			})
		}
		return true
	})
	return findings, nil
}

// environMessage returns the finding's message for a literal of value in tier,
// and whether the tier refuses it.
func environMessage(value string, tier Tier) (string, bool) {
	if tier == TierE2E {
		segment, found := EnvironmentPathIn(value)
		if !found {
			return "", false
		}
		return fmt.Sprintf("the literal names a path into /%s, which holds the environment of a process; an e2e test names no such path, so read a named variable instead", segment), true
	}
	if !environPath.MatchString(path.Clean(value)) {
		return "", false
	}
	return fmt.Sprintf("the literal names %s, which holds the whole environment of a process; read a named variable instead", value), true
}
