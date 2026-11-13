// Package execenv holds the exec environment rules of issue #195, with the
// standard library only: the rules that keep the e2e tier's _test.go files, and
// every Go file under test/, from reading the whole environment of the test
// process, where the agents' keys and tokens live (CLAUDE.md rule 3).
//
// CheckExecEnv parses one Go source file and refuses every *exec.Cmd built by
// exec.Command or exec.CommandContext on which Run, Start, Output or
// CombinedOutput is called before its Env has been assigned in the same
// function: exec.Cmd reads a nil Env as "inherit the whole environment of this
// process", and whatever the child prints from it can reach a t.Log call or a
// golden file. The finding names the line of the call that runs the command,
// which is the line a reader fixes; it says nothing about how far the child's
// output travels, which no source check can see. A command name the caller's
// exempt function admits is let through: the go toolchain's calls inherit the
// environment because the go command needs it, and print none of it.
//
// CheckEnvironLiterals refuses every string literal of the file whose value
// names a /proc/<something>/environ path, which reads the same environment
// without starting a child: the file's comments are not literals and pass, and
// so does every other /proc path.
//
// test/lint runs both rules over the repository, in TestE2ETestsSetExecEnv and
// TestTestTreeNamesNoProcEnviron; their probes are the .go.txt files of this
// package's testdata, which no build compiles and the second scan never meets.
// It lived in test/lint until issue #177 moved the shellcheck rule out of it:
// test/lint holds doc.go and _test.go files only, under the strict
// test-docs-are-pure depguard rule.
package execenv
