package check

import (
	"errors"
	"fmt"
	"strings"

	"github.com/timothymamo/shift2eu/test/internal/openapibundle"
	"github.com/timothymamo/shift2eu/test/internal/yamlread"
)

// The one extra file the release publishes beside the archives (issue #139):
// the bundled OpenAPI document, under a versionless name, so that
// releases/latest/download/<name> is a stable URL. Both release: and
// checksum: hold it, so checksums.txt covers it.
//
// BundleGlob is the file the bundler writes, openapibundle.Path, as
// .goreleaser.yaml names a repository-relative file: one source for the
// bundle's path, so the release cannot publish a file the bundler never
// writes.
const (
	BundleGlob = "./" + openapibundle.Path
	bundleName = "shift2eu-server_openapi.json"
)

// bundleWant is how a message names the extra_files entry it wants.
const bundleWant = "exactly one extra_files entry, glob: " + BundleGlob + " and name_template: " + bundleName

// GitHubRelease checks how a .goreleaser.yaml document publishes the GitHub
// release: its top-level release section must hold exactly draft: true,
// prerelease: auto and replace_existing_draft: true, in that order, and then
// extra_files, holding exactly the bundled OpenAPI document (issue #139).
//
//   - draft: true (issue #156): goreleaser uploads every asset to a draft and
//     leaves it a draft, and release.yml's publish job publishes it last,
//     after the archives are attested. A published release whose repository
//     has release immutability on can then never gain or lose an asset (ADR
//     0015). goreleaser's default publishes the release itself, as soon as
//     its own uploads are done.
//   - prerelease: auto (issue #92): goreleaser marks a tag with a pre-release
//     suffix (v0.1.0-rc.2) as a GitHub pre-release, which never becomes the
//     latest release, and a plain vX.Y.Z as a normal release, as the image
//     gets no X.Y and no latest tag for a pre-release (ADR 0015).
//   - replace_existing_draft: true: a re-run of a failed goreleaser job
//     deletes the draft the failed run left for the tag and starts a new one,
//     rather than leave two drafts for one tag. GitHub's release by tag
//     (GetReleaseByTag) skips drafts, so without it goreleaser creates a
//     second one, and the publish job may publish the incomplete first.
//   - extra_files (issue #139): goreleaser publishes api/openapi.bundled.json
//     as the asset shift2eu-server_openapi.json.
//
// It fails closed: a missing release section is refused, because goreleaser's
// default publishes every tag as a normal release, and so is any other key or
// value in it: make_latest, for one, would override what prerelease: auto
// decides, and draft: false would publish assets that could no longer change
// before they are attested. An extra_files that holds anything but the one
// bundle entry is refused too.
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

	return errors.New("read .goreleaser.yaml: no release section, want release: draft: true, prerelease: auto and replace_existing_draft: true: goreleaser's default publishes a pre-release tag as a normal release, the latest one, and publishes the assets before they are attested")
}

// releaseKeys are the keys of the release section before extra_files, in
// order, each with the one plain value it must have.
var releaseKeys = []struct{ key, value string }{
	{"draft", "true"},
	{"prerelease", "auto"},
	{"replace_existing_draft", "true"},
}

// checkRelease refuses a release section that holds anything but draft: true,
// prerelease: auto and replace_existing_draft: true, in that order, and then
// the bundle's extra file.
func checkRelease(release yamlread.Entry) error {
	block, err := yamlread.BlockValue(release, "release")
	if err != nil {
		return err
	}

	entries, err := yamlread.Mapping(block)
	if err != nil {
		return fmt.Errorf("release: %w", err)
	}

	if len(entries) != len(releaseKeys)+1 {
		return fmt.Errorf("line %d: release holds %d keys, want exactly draft: true, prerelease: auto, replace_existing_draft: true and extra_files: a missing replace_existing_draft leaves a re-run two drafts for one tag, and any other key may change which release GitHub shows as the latest or publish it before its archives are attested", release.Num, len(entries))
	}

	for i, want := range releaseKeys {
		e := entries[i]
		if e.Key != want.key {
			return fmt.Errorf("line %d: release holds %s where it wants %s, in the order draft, prerelease, replace_existing_draft, extra_files", e.Num, e.Key, want.key)
		}

		value, err := yamlread.Single(e)
		if err != nil {
			return fmt.Errorf("release: %w", err)
		}

		// draft and replace_existing_draft are booleans: a quoted "true" is a
		// string goreleaser refuses, so only the plain word is taken.
		if want.key != "prerelease" && strings.ContainsAny(e.Value[:min(1, len(e.Value))], `"'`) {
			value = e.Value
		}

		if value != want.value {
			return fmt.Errorf("line %d: release.%s is %q, want %s", e.Num, want.key, value, want.value)
		}
	}

	if e := entries[len(releaseKeys)]; e.Key != "extra_files" {
		return fmt.Errorf("line %d: release holds %s where it wants extra_files, in the order draft, prerelease, replace_existing_draft, extra_files", e.Num, e.Key)
	}

	return bundleExtraFile(entries, "release")
}

// bundleExtraFile refuses a section, release or checksum at path, whose
// extra_files is anything but exactly one entry holding exactly glob:
// ./api/openapi.bundled.json and name_template: shift2eu-server_openapi.json
// (issue #139).
func bundleExtraFile(entries []yamlread.Entry, path string) error {
	extra, ok := child(entries, "extra_files")
	if !ok {
		return fmt.Errorf("%s holds no extra_files, want %s", path, bundleWant)
	}

	path += ": extra_files"

	block, err := yamlread.BlockValue(extra, path)
	if err != nil {
		return fmt.Errorf("%w: want %s", err, bundleWant)
	}

	items, err := yamlread.Sequence(block)
	if err != nil {
		return fmt.Errorf("%s: %w: want %s", path, err, bundleWant)
	}

	if len(items) != 1 {
		return fmt.Errorf("line %d: %s holds %d entries, want %s", extra.Num, path, len(items), bundleWant)
	}

	fields, err := yamlread.Mapping(items[0].Lines)
	if err != nil {
		return fmt.Errorf("%s: %w: want %s", path, err, bundleWant)
	}

	want := map[string]string{"glob": BundleGlob, "name_template": bundleName}

	if len(fields) != len(want) {
		return fmt.Errorf("line %d: %s holds an entry of %d keys, want %s", items[0].Num, path, len(fields), bundleWant)
	}

	for _, f := range fields {
		wantValue, known := want[f.Key]
		if !known {
			return fmt.Errorf("line %d: %s holds the key %s, want %s", f.Num, path, f.Key, bundleWant)
		}

		got, err := yamlread.Single(f)
		if err != nil {
			return fmt.Errorf("%s: %w: want %s", path, err, bundleWant)
		}

		if got != wantValue {
			return fmt.Errorf("line %d: %s: %s is %q, want %s", f.Num, path, f.Key, got, bundleWant)
		}
	}

	return nil
}
