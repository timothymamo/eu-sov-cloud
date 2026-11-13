// Package execenv holds the exec environment rules of issue #195, with the
// standard library only: the rules that keep the e2e tier's _test.go files, and
// every Go file under test/, from reading the whole environment of the test
// process, where the agents' keys and tokens live (CLAUDE.md rule 3).
//
// CheckExecEnv parses one Go source file and refuses every way an *exec.Cmd
// may start with a nil Env, which exec.Cmd reads as "inherit the whole
// environment of this process"; whatever the child prints from it can reach a
// t.Log call or a golden file. It fails closed: it follows a command only when
// exec.Command or exec.CommandContext builds it as the whole right-hand side
// of a statement inside a function that declares the name it binds
// (`ident :=` or `var ident =`), keyed by the variable that identifier
// declares (go/types, with every import an empty package), so a shadowing name
// cannot mask it. A construction bound any other way (to a name that exists
// already or is declared in another file, a call argument, a composite
// element, a struct field, a chained run) is a finding, and so is a followed
// name rebound to anything but a new construction.
//
// Until its Env is set, the command may only have fields assigned
// (`ident.<Field> = ...`); a Run, Start, Output or CombinedOutput call, an
// alias, an argument, a method value or expression and a capture in a function
// literal are each a finding. Env is set only by a value the check proves is
// not nil: a composite literal, a make call, an append of such a value, or a
// call to a function of the same file whose every return is one of those. Any
// other value (nil, []string(nil), a variable, *new([]string), a multi-value
// call) is a finding and leaves Env unset. Once the command has been aliased,
// stored, passed, captured, deferred or handed to a go statement, any Env
// assigned to it is a finding, and the address of any Env field is a finding
// wherever it appears.
//
// Every type expression naming exec.Cmd that is not the operand of a `*` is
// refused (a value variable, field, array, embedded field, alias, defined
// type, type argument, new(exec.Cmd)), as is an embedded *exec.Cmd: a command
// value no exec.Command built has a nil Env. The one exception is an exec.Cmd
// composite literal whose Env key holds a proven value. A *exec.Cmd field,
// parameter, result or var is accepted, since a nil pointer runs nothing. A
// blank or dot import of os/exec, which hides the commands the check follows,
// is refused too. A finding names the line a reader fixes, and says nothing
// about how far the child's output travels, which no source check can see.
//
// The caller's exempt function is handed the command name and the argument
// after it, both only when they are string literals; a command it admits may
// run with no Env, once bound to a variable (a run chained to the construction
// is always refused). test/lint admits `go build` and `go list`, which need
// the environment and print none of it. The exemption is lost by a spread
// argument, by a literal argument that is or starts with -toolexec,
// --toolexec, -exec, --exec, -overlay or --overlay, and by assigning Path or
// Args.
//
// Four gaps are accepted, and review holds them:
//   - Flow-insensitivity: the check follows the source, not the flow, so an
//     Env assigned anywhere before a use counts, in a branch that never runs
//     (`if false { cmd.Env = ... }`) or in a closure too, as forbidigo's
//     e2e-no-env group accepts its embedded-type gap.
//   - Paths built at run time are beyond CheckEnvironLiterals, a rule of
//     literals (below).
//   - A command built in another package is out of the file's sight.
//   - A non-literal argument to an exempt toolchain command that holds one of
//     the flags above keeps the exemption: only literals are read.
//
// CheckEnvironLiterals refuses the string literals of a file that name the
// environment of a process, which reads it without starting a child, under one
// of two tiers. TierE2E, for test/e2e, which holds no fixture literals,
// refuses a literal with a proc or dev/fd path segment (EnvironmentPathIn).
// TierTest, for the rest of test/, where a probe's source and an expected
// message are literals of their own, refuses a literal whose value, after
// path.Clean, is a /proc/.../environ path at any depth and with any number of
// leading slashes or none. A literal is read as the compiler reads it, a raw
// one too, and one that does not unquote is skipped; comments are no literals.
// A path built at run time (filepath.Join, concatenation, fmt.Sprintf,
// os.DirFS("/proc").Open(...)) is beyond a rule of literals: review holds it,
// and in test/e2e forbidigo's e2e-no-env.
//
// EnvironmentPathIn is the segment matcher both test/lint's composition-root
// rule and the e2e tier share. Its test, environmentpath_test.go, reads the
// paths it pins from a JSON file under testdata, so it holds no path literal
// and TestTestTreeNamesNoProcEnviron scans it like every other Go file.
//
// test/lint runs both rules over the repository, in TestE2ETestsSetExecEnv,
// which walks test/e2e recursively, and TestTestTreeNamesNoProcEnviron; their
// probes are the .go.txt files of this package's testdata, which no build
// compiles and the second scan never meets. It lives here rather than in
// test/lint because test/lint holds doc.go and _test.go files only, under the
// strict test-docs-are-pure depguard rule, as the shellcheck rule of issue
// #177 does.
package execenv
