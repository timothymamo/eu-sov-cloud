// Package shadow declares a function named println, which shadows the
// builtin in this package. The entry for the builtins has pkg ^$, a name
// with no package, so neither the declaration nor the call is refused.
package shadow

func println(s string) string {
	return s + "\n"
}

// Line returns s as a line.
func Line(s string) string {
	return println(s)
}
