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
// of `ident :=`, `ident =` or `var ident =` inside a function, keyed by the
// variable that identifier declares (go/types, with every import an empty
// package), so a shadowing name cannot mask it. Until an Env that is not nil,
// nor []string(nil), is assigned to it, the command may only have fields
// assigned (`ident.<Field> = ...`); a Run, Start, Output or CombinedOutput
// call, an alias, an argument, a method value or expression and a capture in a
// function literal are each a finding. Also refused: a construction bound any
// other way (a call argument, a composite element, a struct field, a chained
// call), an exec.Cmd composite literal or new(exec.Cmd) with no Env, and a
// blank or dot import of os/exec, which hides the commands the check follows.
// A finding names the line a reader fixes, and says nothing about how far the
// child's output travels, which no source check can see.
//
// The caller's exempt function is handed the command name and the argument
// after it, both only when they are string literals; a command it admits may
// run with no Env, directly or chained to its construction. test/lint admits
// `go build` and `go list`, which need the environment and print none of it.
// Assigning Path or Args to an exempt command takes the exemption away.
//
// Two gaps are accepted. The check follows the source, not the flow: an Env
// assigned anywhere before a use counts, in a branch that never runs too
// (`if false { cmd.Env = ... }`), as forbidigo's e2e-no-env group accepts its
// embedded-type gap; review holds the rest. And a command built in another
// package is out of the file's sight.
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
// rule and the e2e tier share. Its test, environmentpath_test.go, pins paths
// as bare values, so it is the one file TestTestTreeNamesNoProcEnviron leaves
// out by name (test/lint's environPathsPinnedBy).
//
// test/lint runs both rules over the repository, in TestE2ETestsSetExecEnv,
// which walks test/e2e recursively, and TestTestTreeNamesNoProcEnviron; their
// probes are the .go.txt files of this package's testdata, which no build
// compiles and the second scan never meets. It lives here rather than in
// test/lint because test/lint holds doc.go and _test.go files only, under the
// strict test-docs-are-pure depguard rule, as the shellcheck rule of issue
// #177 does.
package execenv
