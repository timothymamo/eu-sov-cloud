// A helper reads no environment variable, starts no process and locates no
// directory from one (issue #144, CLAUDE.md rule 3: the agents' API keys and
// tokens are in the environment and are never read, copied or printed).
// testutil-imports allows os, which the golden files are read and written with,
// and an import rule sees no call: so the forbidigo group testutil-no-env
// refuses the calls, and every line below ends in a want comment of that group.
// os.ReadFile in golden.go stays allowed, which is why the group names these
// calls and not the package.
package testutil

import "os"

// environment holds one call for each way a helper could reach the environment
// or hand a child a process. Nothing here runs: the probe lints the file.
//
// The writes — os.Setenv, os.Unsetenv, os.Clearenv and os.Chdir — are no call of
// this group: the group tests-no-process-state refuses them in every file of the
// module but the two harnesses under test/ (issue #153), so one group owns them
// and no line is reported twice.
func environment() {
	_ = os.Getenv("SHIFT2EU_TOKEN")               // want "testutil-no-env: "
	_, _ = os.LookupEnv("SHIFT2EU_TOKEN")         // want "testutil-no-env: "
	_ = os.ExpandEnv("$SHIFT2EU_TOKEN")           // want "testutil-no-env: "
	_ = os.Environ()                              // want "testutil-no-env: "
	_, _ = os.StartProcess("/bin/true", nil, nil) // want "testutil-no-env: "

	// The four lookups that locate a directory from the environment ($HOME,
	// $XDG_CONFIG_HOME, $XDG_CACHE_HOME, $TMPDIR): a helper that reads
	// ~/.config/dot-agent-deck/daemon.env or ~/.aws/credentials is one this
	// group must refuse, and it reads no named variable to build the path.
	_, _ = os.UserHomeDir()   // want "testutil-no-env: "
	_, _ = os.UserConfigDir() // want "testutil-no-env: "
	_, _ = os.UserCacheDir()  // want "testutil-no-env: "
	_ = os.TempDir()          // want "testutil-no-env: "

	// A method value is no call, and the group must refuse it too: taken
	// alone, it hands the read to whoever the helper passes it to.
	_ = os.Getenv // want "testutil-no-env: "
}
