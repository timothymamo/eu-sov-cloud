package check

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// executableMode is the mode git records for an executable regular file.
const executableMode = "100755"

// NonExecutableScripts returns, sorted, every file in the git index under dir,
// a slash-separated directory relative to root, whose staged content starts
// with #! and whose mode is not 100755. Such a script fails with Permission
// denied when it is run directly, as the Makefile runs the scripts under dev/
// (issue #125).
//
// It reads both the mode and the content from the index (git ls-files -s and
// git cat-file), never from the working tree: the index is what a commit
// records, and a checkout's file system may not keep modes. An untracked file
// has no mode yet, so it is not reported. It fails closed: when git cannot run
// or cannot list root, it returns an error.
func NonExecutableScripts(ctx context.Context, root, dir string) ([]string, error) {
	out, err := runGit(ctx, root, "ls-files", "-z", "--stage", "--", dir+"/")
	if err != nil {
		return nil, fmt.Errorf("list the index under %s/%s: %w", root, dir, err)
	}

	var scripts []string

	for entry := range strings.SplitSeq(string(out), "\x00") {
		if entry == "" {
			continue
		}

		// An entry is "<mode> <object> <stage>\t<path>".
		meta, path, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)

		if !ok || len(fields) != 3 {
			return nil, fmt.Errorf("read the index entry %q: want <mode> <object> <stage>\\t<path>", entry)
		}

		mode, object := fields[0], fields[1]
		if mode == executableMode {
			continue
		}

		content, err := runGit(ctx, root, "cat-file", "blob", object)
		if err != nil {
			return nil, fmt.Errorf("read %s from the index: %w", path, err)
		}

		if bytes.HasPrefix(content, []byte("#!")) {
			scripts = append(scripts, path)
		}
	}

	slices.Sort(scripts)

	return slices.Compact(scripts), nil
}

// runGit runs git with args in root, with no GIT_ variable from the
// environment, as gitFiles does, and returns what it prints.
func runGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root

	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}
