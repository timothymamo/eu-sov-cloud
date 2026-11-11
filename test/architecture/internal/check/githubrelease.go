package check

import (
	"errors"
	"fmt"

	"github.com/timothymamo/eu-sov-cloud/test/internal/yamlread"
)

// GitHubRelease checks how a .goreleaser.yaml document publishes the GitHub
// release: its top-level release section must hold exactly one key,
// prerelease, set to auto (issue #92). goreleaser then marks a tag with a
// pre-release suffix (v0.1.0-rc.2) as a GitHub pre-release, which never
// becomes the latest release, and a plain vX.Y.Z as a normal release, as the
// image gets no X.Y and no latest tag for a pre-release (ADR 0015).
//
// It fails closed: a missing release section is refused, because goreleaser's
// default publishes every tag as a normal release, and so is any other key in
// it: make_latest, for one, would override what prerelease: auto decides.
func GitHubRelease(goreleaser []byte) error {
	lines, err := yamlread.SplitLines(goreleaser)
	if err != nil {
		return fmt.Errorf("read .goreleaser.yaml: %w", err)
	}

	entries, err := yamlread.Mapping(lines)
	if err != nil {
		return fmt.Errorf("read .goreleaser.yaml: %w", err)
	}

	for _, e := range entries {
		if e.Key == "release" {
			if err := checkRelease(e); err != nil {
				return fmt.Errorf("read .goreleaser.yaml: %w", err)
			}

			return nil
		}
	}

	return errors.New("read .goreleaser.yaml: no release section, want release: prerelease: auto: goreleaser's default publishes a pre-release tag as a normal release, the latest one")
}

// checkRelease refuses a release section that holds anything but
// prerelease: auto.
func checkRelease(release yamlread.Entry) error {
	block, err := yamlread.BlockValue(release, "release")
	if err != nil {
		return err
	}

	entries, err := yamlread.Mapping(block)
	if err != nil {
		return fmt.Errorf("release: %w", err)
	}

	if len(entries) != 1 || entries[0].Key != "prerelease" {
		return fmt.Errorf("line %d: release holds %d keys, want exactly prerelease: any other key may change which release GitHub shows as the latest", release.Num, len(entries))
	}

	value, err := yamlread.Single(entries[0])
	if err != nil {
		return fmt.Errorf("release: %w", err)
	}

	if value != "auto" {
		return fmt.Errorf("line %d: release.prerelease is %q, want auto, so a pre-release tag is a GitHub pre-release", entries[0].Num, value)
	}

	return nil
}
