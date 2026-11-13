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

// exemptionSpoilers are the go command's flags that hand its environment to a
// program the next argument (or the value after "=") names: an exempt command
// with a literal argument that is, or starts with, one of them is exempt no
// more.
var exemptionSpoilers = []string{"-toolexec", "--toolexec", "-exec", "--exec", "-overlay", "--overlay"}

// CheckExecEnv returns a finding, in source order, for every place in the Go
// source src where a command of os/exec may start with a nil Env, which
// inherits the whole environment of the test process. It fails closed: a
// command it cannot follow, and a use of a command it cannot vouch for, is a
// finding too. filename names the file in the findings and in the parse
// error.
//
// A command is followed when exec.Command or exec.CommandContext builds it as
// the whole right-hand side of a statement inside a function that declares the
// name it is bound to (`ident :=` or `var ident =`), and it is keyed by that
// variable, so a shadowing name in an inner block is another command. A
// construction bound any other way (to a name that exists already, a call
// argument, a composite element, a struct field, a chained run) is a finding,
// and so is a followed name rebound to anything but a new construction.
//
// Until an Env is set, the only uses of a followed command accepted are
// `ident.<Field> = ...` and, for an exempt command, a Run, Start, Output or
// CombinedOutput call on it; every other use (an alias, an argument, a method
// value or expression, a capture in a function literal, a run call) is a
// finding. Env is set only by a value the check proves is not nil: a composite
// literal, a make call, an append whose first argument is such a value, or a
// call to a function of the same file whose every return is one of the first
// three. Any other value is a finding and leaves Env unset. Once the command
// has been aliased, stored, passed, captured, deferred or handed to a go
// statement, an Env assigned to it is a finding whatever the value, and Env
// keeps the state it had. The check follows the source, not the flow: an Env
// assigned anywhere before the use counts, in a branch that never runs or in a
// closure too.
//
// Also refused: every type expression naming exec.Cmd that is not the operand
// of a `*` (a value variable, field, array, alias, defined type, type argument,
// new(exec.Cmd)), except an exec.Cmd composite literal whose Env key holds a
// proven value; an embedded *exec.Cmd field; the address of any Env field; and
// a blank or dot import of os/exec.
//
// exempt is handed the command name, the first argument of exec.Command and the
// second of exec.CommandContext, and sub, the argument after it, when both are
// string literals; a command it admits may run with no Env. A missing or
// non-literal name or sub is never exempt, and the exemption is lost by a
// spread argument, a literal argument that is or starts with one of
// exemptionSpoilers, and an assignment to Path or Args.
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
	c.funcDecls = funcDecls(c.info, file)
	ast.Inspect(file, c.visit)
	return c.findings, nil
}

// resolve type-checks file on its own, for the objects its identifiers
// declare and use. Every import resolves to an empty package, so the check
// reports errors (exec.Command among them is undefined), which it ignores:
// what it needs is which variable each identifier names, and that the check
// records whatever the types it cannot see. An identifier declared in another
// file of the package resolves to no object, so a construction bound to it is
// no command the checker can follow and is refused, and a function declared
// there proves no Env value.
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

// funcDecls returns the functions file declares, with no receiver, by their
// object.
func funcDecls(info *types.Info, file *ast.File) map[types.Object]*ast.FuncDecl {
	decls := map[types.Object]*ast.FuncDecl{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Body == nil {
			continue
		}
		if obj := info.Defs[fn.Name]; obj != nil {
			decls[obj] = fn
		}
	}
	return decls
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
	// funcDecls are the functions the file declares, which an Env value may
	// call.
	funcDecls map[types.Object]*ast.FuncDecl
	// commands are the commands followed so far, by the variable that holds
	// each.
	commands map[types.Object]*command
	// funcs counts the function bodies the walk is inside, a function
	// literal's included; a construction outside every one is no command the
	// checker can follow.
	funcs int
	// lits are the function literals the walk is inside, innermost last.
	lits []*ast.FuncLit
	// deferred counts the go and defer statements the walk is inside, within
	// the innermost function: a run there may happen after the statements
	// below it.
	deferred int
	findings []Finding
}

// command is what the checker knows of a variable that holds an *exec.Cmd
// built in the file.
type command struct {
	// exempt is whether the command's name and subcommand are literals exempt
	// admits, no argument spoils that, and neither Path nor Args has been
	// assigned since.
	exempt bool
	// envSet is whether an Env the check proves is not nil was assigned since
	// the command was built.
	envSet bool
	// escaped is whether the command has been aliased, stored, passed,
	// captured, deferred or handed to a go statement: an Env assigned after
	// that may not be the one a run sees.
	escaped bool
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
		if n.Recv != nil {
			ast.Inspect(n.Recv, c.visit)
		}
		ast.Inspect(n.Type, c.visit)
		if n.Body != nil {
			c.funcs++
			ast.Inspect(n.Body, c.visit)
			c.funcs--
		}
		return false
	case *ast.FuncLit:
		ast.Inspect(n.Type, c.visit)
		deferred := c.deferred
		c.deferred = 0
		c.funcs++
		c.lits = append(c.lits, n)
		ast.Inspect(n.Body, c.visit)
		c.lits = c.lits[:len(c.lits)-1]
		c.funcs--
		c.deferred = deferred
		return false
	case *ast.GoStmt:
		c.walkDeferred(n.Call)
		return false
	case *ast.DeferStmt:
		c.walkDeferred(n.Call)
		return false
	case *ast.AssignStmt:
		c.assign(n.Lhs, n.Rhs)
		return false
	case *ast.ValueSpec:
		if n.Type != nil {
			ast.Inspect(n.Type, c.visit)
		}
		lhs := make([]ast.Expr, len(n.Names))
		for i, name := range n.Names {
			lhs[i] = name
		}
		c.assign(lhs, n.Values)
		return false
	case *ast.CallExpr:
		return c.call(n)
	case *ast.CompositeLit:
		return c.composite(n)
	case *ast.StarExpr:
		// A pointer to a command is nil until a command the check follows, or
		// refuses, is assigned to it.
		return !c.isCmdType(n.X)
	case *ast.SelectorExpr:
		if c.isCmdType(n) {
			c.report(n.Pos(), "exec.Cmd as a value type holds a command no exec.Command built, whose nil Env inherits the whole environment of the test process; hold a *exec.Cmd that exec.Command returns")
			return false
		}
		return true
	case *ast.StructType:
		for _, field := range n.Fields.List {
			if star, ok := ast.Unparen(field.Type).(*ast.StarExpr); ok && len(field.Names) == 0 && c.isCmdType(star.X) {
				c.report(field.Type.Pos(), "an embedded *exec.Cmd hands its run methods to a struct the check cannot follow; hold the command in a named field")
			}
		}
		return true
	case *ast.UnaryExpr:
		if sel, ok := ast.Unparen(n.X).(*ast.SelectorExpr); ok && n.Op == token.AND && sel.Sel.Name == "Env" {
			c.report(n.Pos(), "the address of an Env field lets code the check cannot follow change the Env; assign the field directly")
		}
		return true
	case *ast.Ident:
		c.use(n)
		return true
	}
	return true
}

// walkDeferred walks the call of a go or defer statement, which may run after
// the statements below it.
func (c *execChecker) walkDeferred(call *ast.CallExpr) {
	c.deferred++
	ast.Inspect(call, c.visit)
	c.deferred--
}

// assign walks an assignment or a var declaration: the right-hand side first,
// as Go evaluates it, then the targets, which bind the commands it builds,
// refuse the followed names it rebinds to something else, and record the
// fields it sets.
func (c *execChecker) assign(lhs, rhs []ast.Expr) {
	paired := len(lhs) == len(rhs)
	bound := make([]*binding, len(lhs))
	constructed := make([]bool, len(lhs))
	for i, value := range rhs {
		call, isCall := ast.Unparen(value).(*ast.CallExpr)
		if isCall && paired {
			if exempt, ok := c.construction(call); ok {
				constructed[i] = true
				if c.funcs > 0 && c.declares(lhs[i]) {
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
			c.bind(target, bound[i], constructed[i])
		case *ast.SelectorExpr:
			var value ast.Expr
			if paired {
				value = rhs[i]
			}
			c.setField(target, value)
		default:
			ast.Inspect(target, c.visit)
		}
	}
}

// binding is a construction an assignment binds to the name it declares.
type binding struct {
	exempt bool
}

// bind handles a name an assignment targets: it starts following the command
// a declaration binds to it, and refuses a followed name rebound to anything
// but a construction, which is refused where it is built.
func (c *execChecker) bind(target *ast.Ident, bound *binding, constructed bool) {
	obj := c.object(target)
	if obj == nil {
		return
	}
	if bound != nil {
		c.commands[obj] = &command{exempt: bound.exempt, lit: c.lit()}
		return
	}
	if _, followed := c.commands[obj]; !followed {
		return
	}
	delete(c.commands, obj)
	if !constructed {
		c.report(target.Pos(), fmt.Sprintf(
			"%s held a command the check follows and is rebound to a value it cannot follow; declare a new variable for the other command",
			target.Name))
	}
}

// setField handles an assignment to a field: on a followed command it refuses
// an Env the check cannot prove, or one assigned after the command escaped,
// and takes the exemption away on Path and Args. value is nil for an
// assignment whose right-hand side is no single expression per target.
func (c *execChecker) setField(target *ast.SelectorExpr, value ast.Expr) {
	id, cmd := c.command(target.X)
	if cmd == nil {
		ast.Inspect(target, c.visit)
		return
	}
	field := target.Sel.Name
	if field == "Env" && cmd.escaped {
		c.report(target.Pos(), fmt.Sprintf(
			"%s.Env is assigned after %s was aliased, stored, passed, captured or deferred, so a run may not see it; assign %s.Env before the command leaves the function's hands",
			id.Name, id.Name, id.Name))
		return
	}

	reported := false
	if c.captured(cmd) {
		cmd.escaped = true
		if !cmd.envSet {
			c.report(id.Pos(), fmt.Sprintf(
				"%s is captured by a function literal before its Env is set, so the check cannot tell what runs it; assign %s.Env before the literal",
				id.Name, id.Name))
			reported = true
		}
	}

	switch field {
	case "Env":
		if value != nil && c.accepted(value, true) {
			cmd.envSet = true
			return
		}
		cmd.envSet = false
		if !reported {
			c.report(target.Pos(), fmt.Sprintf(
				"%s.Env is assigned a value the check cannot prove is not nil, and a nil Env inherits the whole environment of the test process; assign a composite literal, a make, an append of one, or a call to a helper of this file that returns one",
				id.Name))
		}
	case "Path", "Args":
		cmd.exempt = false
	}
}

// call handles a call expression and reports whether the walk should descend
// into it.
func (c *execChecker) call(call *ast.CallExpr) bool {
	if c.isNewCmd(call) {
		c.report(call.Pos(), "new(exec.Cmd) builds a command with a nil Env, which inherits the whole environment of the test process; build it with exec.Command and assign its Env")
		return false
	}

	if _, ok := c.construction(call); ok {
		c.report(call.Pos(), "the command is not bound as the whole right-hand side of a new variable inside a function, so the check cannot follow its Env; bind it with `cmd := exec.Command(...)` and assign cmd.Env before it runs")
		c.walkArgs(call)
		return false
	}

	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || !runMethods[sel.Sel.Name] {
		return true
	}

	// A command built and run in one expression has no name to hold an Env,
	// and no exemption reaches it.
	if inner, ok := ast.Unparen(sel.X).(*ast.CallExpr); ok {
		if _, ok := c.construction(inner); ok {
			c.report(sel.Sel.Pos(), fmt.Sprintf(
				"%s is called on a command built in the same expression, whose Env is nil and inherits the whole environment of the test process; bind the command to a variable and assign its Env first",
				sel.Sel.Name))
			c.walkArgs(inner)
			c.walkArgs(call)
			return false
		}
	}

	id, cmd := c.command(sel.X)
	if cmd == nil {
		return true
	}
	captured := c.captured(cmd)
	if captured || c.deferred > 0 {
		cmd.escaped = true
	}
	switch {
	case cmd.envSet:
	case captured:
		c.report(sel.Sel.Pos(), fmt.Sprintf(
			"%s.%s runs, inside a function literal, a command whose Env is not set; assign %s.Env before the literal",
			id.Name, sel.Sel.Name, id.Name))
	case !cmd.exempt:
		c.report(sel.Sel.Pos(), fmt.Sprintf(
			"%s.%s runs a command whose Env is nil, which inherits the whole environment of the test process; assign %s.Env before the call",
			id.Name, sel.Sel.Name, id.Name))
	}
	c.walkArgs(call)
	return false
}

// composite handles a composite literal and reports whether the walk should
// descend into it. An exec.Cmd literal, and one whose type the source elides
// in a literal of exec.Cmd elements, is refused unless its Env key holds a
// value the check proves is not nil; its type is no value type to refuse.
func (c *execChecker) composite(lit *ast.CompositeLit) bool {
	if lit.Type != nil && c.isCmdType(lit.Type) {
		c.checkCmdLiteral(lit)
		c.walkElts(lit)
		return false
	}
	if c.holdsCmd(lit.Type) {
		for _, elt := range lit.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				elt = kv.Value
			}
			if inner, ok := ast.Unparen(elt).(*ast.CompositeLit); ok && inner.Type == nil {
				c.checkCmdLiteral(inner)
			}
		}
	}
	return true
}

// checkCmdLiteral refuses an exec.Cmd composite literal with no Env key, or
// one the check cannot prove is not nil.
func (c *execChecker) checkCmdLiteral(lit *ast.CompositeLit) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Env" && c.accepted(kv.Value, true) {
			return
		}
	}
	c.report(lit.Pos(), "an exec.Cmd composite literal with no Env, or one the check cannot prove is not nil, inherits the whole environment of the test process; give it an Env key holding a composite literal, a make or an append of one")
}

// walkElts walks the elements of a composite literal, not its type.
func (c *execChecker) walkElts(lit *ast.CompositeLit) {
	for _, elt := range lit.Elts {
		ast.Inspect(elt, c.visit)
	}
}

// holdsCmd reports whether t is an array, slice or map type whose elements are
// exec.Cmd or *exec.Cmd, so an element literal with no type of its own builds
// a command.
func (c *execChecker) holdsCmd(t ast.Expr) bool {
	var elt ast.Expr
	switch t := ast.Unparen(t).(type) {
	case *ast.ArrayType:
		elt = t.Elt
	case *ast.MapType:
		elt = t.Value
	default:
		return false
	}
	if star, ok := ast.Unparen(elt).(*ast.StarExpr); ok {
		elt = star.X
	}
	return c.isCmdType(elt)
}

// accepted reports whether the Env value x is one the check proves is not nil:
// a composite literal, a make call, an append whose first argument is
// accepted, or, when helpers is set, a call to a function of the file whose
// every return expression is accepted without helpers.
func (c *execChecker) accepted(x ast.Expr, helpers bool) bool {
	switch x := ast.Unparen(x).(type) {
	case *ast.CompositeLit:
		return true
	case *ast.CallExpr:
		id, ok := ast.Unparen(x.Fun).(*ast.Ident)
		if !ok {
			return false
		}
		switch obj := c.info.Uses[id].(type) {
		case *types.Builtin:
			switch obj.Name() {
			case "make":
				return true
			case "append":
				return len(x.Args) > 0 && c.accepted(x.Args[0], helpers)
			}
		case *types.Func:
			decl := c.funcDecls[obj]
			return helpers && decl != nil && c.returnsAccepted(decl)
		}
	}
	return false
}

// returnsAccepted reports whether decl returns, at every return statement of
// its own body, one expression the check accepts without following a further
// call into the file, and has at least one such statement.
func (c *execChecker) returnsAccepted(decl *ast.FuncDecl) bool {
	returns, ok := 0, true
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			returns++
			if len(n.Results) != 1 || !c.accepted(n.Results[0], false) {
				ok = false
			}
		}
		return ok
	})
	return ok && returns > 0
}

// use refuses an identifier that names a followed command whose Env is not set
// yet, where no other case of the walk has accepted it, and records that the
// command has gone where the check cannot follow it.
func (c *execChecker) use(id *ast.Ident) {
	cmd := c.commands[c.object(id)]
	if cmd == nil {
		return
	}
	cmd.escaped = true
	if cmd.envSet {
		return
	}
	c.report(id.Pos(), fmt.Sprintf(
		"%s is used before its Env is set (copied, passed on, captured or taken as a method), so the check cannot follow what runs it; assign %s.Env first",
		id.Name, id.Name))
}

// walkArgs walks the arguments of a call, which the walk reads as any other
// expression.
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
	if !nameOK || !subOK || !c.exempt(name, sub) {
		return false, true
	}
	return !spoilsExemption(call), true
}

// spoilsExemption reports whether call has a spread argument or a literal
// argument that names one of exemptionSpoilers.
func spoilsExemption(call *ast.CallExpr) bool {
	if call.Ellipsis.IsValid() {
		return true
	}
	for _, arg := range call.Args {
		value, ok := stringLit(arg)
		if !ok {
			continue
		}
		for _, flag := range exemptionSpoilers {
			if strings.HasPrefix(value, flag) {
				return true
			}
		}
	}
	return false
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

// declares reports whether x is a name, other than the blank one, that the
// statement it is in declares: a name `:=` reuses, or one that exists
// already, is no new variable.
func (c *execChecker) declares(x ast.Expr) bool {
	id, ok := ast.Unparen(x).(*ast.Ident)
	return ok && id.Name != "_" && c.info.Defs[id] != nil
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
