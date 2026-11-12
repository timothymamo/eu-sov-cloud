// A helper reads no environment variable and starts no process (issue #144,
// CLAUDE.md rule 3: the agents' API keys and tokens are in the environment and
// are never read, copied or printed). testutil-imports allows os, which the
// golden files are read and written with, and an import rule sees no call: so
// the forbidigo group testutil-no-env refuses the calls, and every line below
// ends in a want comment of that group. os.ReadFile in golden.go stays allowed,
// which is why the group names these calls and not the package.
package testutil

import "os"

// environment holds one call for each way a helper could reach the environment
// or hand a child a process. Nothing here runs: the probe lints the file.
func environment() {
	_ = os.Getenv("SHIFT2EU_TOKEN")               // want "testutil-no-env: "
	_, _ = os.LookupEnv("SHIFT2EU_TOKEN")         // want "testutil-no-env: "
	_ = os.ExpandEnv("$SHIFT2EU_TOKEN")           // want "testutil-no-env: "
	_ = os.Environ()                              // want "testutil-no-env: "
	_ = os.Setenv("SHIFT2EU_TOKEN", "example")    // want "testutil-no-env: "
	_ = os.Unsetenv("SHIFT2EU_TOKEN")             // want "testutil-no-env: "
	os.Clearenv()                                 // want "testutil-no-env: "
	_, _ = os.StartProcess("/bin/true", nil, nil) // want "testutil-no-env: "
}
