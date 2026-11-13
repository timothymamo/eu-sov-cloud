// The inventory-file reader's stand-in for TestInventoryFileNoEnvProbe (issue
// #152, the auditor's finding M-B). depguard's
// inventory-file-reader-reads-one-file allows os, which the reader opens the file
// a user names with; os also reads the environment, where credentials live
// (CLAUDE.md rule 3), and locates a home, config, cache or temp directory from an
// unnamed variable. The forbidigo group inventoryfile-no-env refuses those calls,
// and cmd/shift2eu, which constructs the reader, hands it no way to make one.
//
// Every line below that ends in a want comment is a call the group must refuse;
// os.Open and the calls the reader's tests make keep no want comment, so a group
// that stops firing, or one that reaches os.Open, fails the probe. Nothing here
// runs: the probe lints the file.
package inventoryfile

import "os"

// reads makes every call of os that reaches the environment or a directory an
// environment variable names.
func reads() {
	_ = os.Getenv("AWS_PROFILE")                  // want "inventoryfile-no-env: "
	_, _ = os.LookupEnv("AWS_PROFILE")            // want "inventoryfile-no-env: "
	_ = os.ExpandEnv("$AWS_PROFILE")              // want "inventoryfile-no-env: "
	_ = os.Environ()                              // want "inventoryfile-no-env: "
	_, _ = os.StartProcess("/bin/true", nil, nil) // want "inventoryfile-no-env: "
	_, _ = os.UserHomeDir()                       // want "inventoryfile-no-env: "
	_, _ = os.UserConfigDir()                     // want "inventoryfile-no-env: "
	_, _ = os.UserCacheDir()                      // want "inventoryfile-no-env: "
	_ = os.TempDir()                              // want "inventoryfile-no-env: "
}

// opens reads the one file the reader is given, and the calls its own tests make
// a temporary file and directory with: the group keeps them allowed.
func opens(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	_ = f.Close()

	_, _ = os.ReadFile(path)
	_ = os.WriteFile(path, nil, 0o600)
	_ = os.MkdirAll(path, 0o700)
	_, _ = os.ReadDir(path)
	_, _ = os.MkdirTemp("", "shift2eu-inventory-")
	_, _ = os.CreateTemp("", "shift2eu-inventory-")
}
