package check

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

// plainMode is the mode git records for a regular file that is not
// executable. Symlinks (120000) and gitlinks (160000) are no scripts.
const plainMode = "100644"

// NonExecutableScripts returns, sorted, every file in the git index under dir,
// a slash-separated directory relative to root, whose staged content starts
// with #! and whose mode is 100644, not 100755. Such a script fails with Permission
// denied when it is run directly, as the Makefile runs the scripts under dev/
// (issue #125).
//
// It reads both the mode and the content from the index (git ls-files -s and
// git cat-file --batch), never from the working tree: the index is what a commit
// records, and a checkout's file system may not keep modes. An untracked file
// has no mode yet, so it is not reported. It fails closed: when git cannot run
// or cannot list root, it returns an error.
func NonExecutableScripts(ctx context.Context, root, dir string) ([]string, error) {
	out, err := runGit(exec.CommandContext(ctx, "git", "ls-files", "-z", "--stage"), root)
	if err != nil {
		return nil, fmt.Errorf("list the index of %s: %w", root, err)
	}

	prefix := strings.TrimSuffix(dir, "/") + "/"

	var paths, objects []string

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

		if !strings.HasPrefix(path, prefix) || fields[0] != plainMode {
			continue
		}

		paths = append(paths, path)
		objects = append(objects, fields[1])
	}

	if len(paths) == 0 {
		return nil, nil
	}

	contents, err := blobContents(ctx, root, objects)
	if err != nil {
		return nil, fmt.Errorf("read the index under %s/%s: %w", root, dir, err)
	}

	var scripts []string

	for i, path := range paths {
		if bytes.HasPrefix(contents[i], []byte("#!")) {
			scripts = append(scripts, path)
		}
	}

	slices.Sort(scripts)

	return slices.Compact(scripts), nil
}

// blobContents returns the content of each object, in order, read in one git
// cat-file --batch, whose output is "<object> blob <size>\n<content>\n" per
// object.
func blobContents(ctx context.Context, root string, objects []string) ([][]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "cat-file", "--batch")
	cmd.Stdin = strings.NewReader(strings.Join(objects, "\n") + "\n")

	out, err := runGit(cmd, root)
	if err != nil {
		return nil, err
	}

	contents := make([][]byte, 0, len(objects))

	for _, object := range objects {
		header, rest, ok := bytes.Cut(out, []byte("\n"))
		fields := strings.Fields(string(header))

		if !ok || len(fields) != 3 || fields[0] != object || fields[1] != "blob" {
			return nil, fmt.Errorf("git cat-file --batch: want a blob header for %s, got %q", object, header)
		}

		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 || size+1 > len(rest) {
			return nil, fmt.Errorf("git cat-file --batch: bad size in %q", header)
		}

		contents = append(contents, rest[:size])
		out = rest[size+1:]
	}

	return contents, nil
}

// runGit runs the git command cmd in root, with no GIT_ variable from the
// environment, as gitFiles does, and returns what it prints.
func runGit(cmd *exec.Cmd, root string) ([]byte, error) {
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
		return nil, fmt.Errorf("%s: %w: %s", strings.Join(cmd.Args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}
