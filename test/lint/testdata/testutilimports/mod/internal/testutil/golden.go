// The helpers read and write golden files, which is what testutil does today:
// os for the file and path/filepath for its path. Neither line carries a want
// comment, so the probe fails if testutil-imports stops allowing them (issue
// #144): the rule is an allow list, and a helper that reads a golden file must
// stay able to.
package testutil

import (
	"os"
	"path/filepath"
)

// Golden stands for the helper that reads the golden file at dir/name.
func Golden(dir, name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(dir, name))
}
