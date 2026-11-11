package check

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// LineDirectives returns the position of every line directive in the Go
// source src: each comment that starts with "//line " or "/*line ", the two
// forms the compiler reads, wherever it sits on its line. Text inside a string
// literal is not a comment and is not reported.
//
// Positions are written file:line:column, filename as given, at the physical
// place in src: go/scanner applies the directives it reads, so the adjusted
// position would be the one a directive names. Any scan error, an unterminated
// comment for one, is returned.
func LineDirectives(filename string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	file := fset.AddFile(filename, -1, len(src))

	var errs scanner.ErrorList

	var s scanner.Scanner
	s.Init(file, src, errs.Add, scanner.ScanComments)

	var found []string

	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}

		if tok == token.COMMENT && (strings.HasPrefix(lit, "//line ") || strings.HasPrefix(lit, "/*line ")) {
			found = append(found, fset.PositionFor(pos, false).String())
		}
	}

	if len(errs) != 0 {
		errs.Sort()

		return nil, fmt.Errorf("scan %s: %w", filename, errs.Err())
	}

	return found, nil
}

// FindLineDirectives returns the position of every line directive in the .go
// files under root that git tracks or would add, and every symlink among those
// files. The files are those `git ls-files --cached --others
// --exclude-standard` lists in root: tracked files, ignored or not, and
// untracked files that are not ignored, so what can be committed is scanned
// and a local bin/, dist/ or editor directory is not. A listed file gone from
// the working tree is skipped. A symlink is reported as "<path>: symlink, not
// scanned" rather than followed: lint reads what a symlink points to under its
// real path, if at all, so the repository must hold none; any other listed
// entry that is not a regular file (a nested repository) is reported as
// "<path>: not a regular file, not scanned". Paths are slash-separated and
// relative to root, in path order and then by position in the file.
//
// It fails closed: when git cannot run or cannot list root, it returns an
// error. git runs without the GIT_ variables of the environment, so a hook
// that runs the tests cannot point it at another repository or index.
func FindLineDirectives(ctx context.Context, root string) ([]string, error) {
	names, err := gitFiles(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("find line directives under %s: %w", root, err)
	}

	// Every file is opened through an os.Root, so a listed path cannot reach
	// outside root, not even through a symlinked directory.
	dir, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("find line directives under %s: %w", root, err)
	}
	defer dir.Close()

	var found []string

	for _, name := range names {
		path := filepath.FromSlash(name)

		info, err := dir.Lstat(path)

		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil:
			return nil, fmt.Errorf("find line directives under %s: %w", root, err)
		case info.Mode()&fs.ModeSymlink != 0:
			found = append(found, name+": symlink, not scanned")

			continue
		case !info.Mode().IsRegular():
			found = append(found, name+": not a regular file, not scanned")

			continue
		case !strings.HasSuffix(name, ".go"):
			continue
		}

		src, err := dir.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("find line directives under %s: read %s: %w", root, name, err)
		}

		directives, err := LineDirectives(name, src)
		if err != nil {
			return nil, fmt.Errorf("find line directives under %s: %w", root, err)
		}

		found = append(found, directives...)
	}

	return found, nil
}

// gitFiles returns the paths git lists in root, tracked and untracked but not
// ignored, slash-separated, sorted and each once.
func gitFiles(ctx context.Context, root string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
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
		return nil, fmt.Errorf("git ls-files: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	var names []string

	for name := range strings.SplitSeq(stdout.String(), "\x00") {
		if name != "" {
			names = append(names, strings.TrimSuffix(name, "/"))
		}
	}

	slices.Sort(names)

	return slices.Compact(names), nil
}
