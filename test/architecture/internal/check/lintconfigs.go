package check

import (
	"context"
	"fmt"
	"slices"
)

// strayLintConfigs are the golangci-lint documents at the repository root that
// golangci-lint reads in place of .golangci.yml: it looks its config up
// through viper, which tries .golangci.json, .golangci.toml and .golangci.yaml
// before .golangci.yml and uses the first it finds.
var strayLintConfigs = []string{".golangci.json", ".golangci.toml", ".golangci.yaml"}

// StrayLintConfigs returns a message for every golangci-lint document at root
// other than .golangci.yml that git could commit: .golangci.json,
// .golangci.toml or .golangci.yaml. Any of them silently replaces
// .golangci.yml, the document HidingSettings and the depguard and forbidigo
// checks read, so lint would run with whatever it sets, forbidigo off and exit
// code 0 included (issue #81). Like StrayReleaseFiles it decides by what `git
// ls-files --cached --others --exclude-standard` lists (gitFiles): a tracked
// file, even one gone from the working tree or added with --force over
// .gitignore, and an untracked file git does not ignore. A developer's ignored
// copy never reaches CI, so it is not refused. Symlinks count too. Only the
// root counts: golangci-lint starts its search in the directory it runs in,
// which make lint and CI set to the root.
//
// It fails closed: when git cannot run or cannot list root, it returns an
// error.
func StrayLintConfigs(ctx context.Context, root string) ([]string, error) {
	names, err := gitFiles(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("find stray lint configs under %s: %w", root, err)
	}

	var messages []string

	for _, stray := range strayLintConfigs {
		if slices.Contains(names, stray) {
			messages = append(messages, stray+" is one git could commit: golangci-lint reads it before .golangci.yml, so lint would run with its settings, which no check reads: remove it")
		}
	}

	return messages, nil
}
