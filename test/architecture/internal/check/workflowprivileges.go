package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/timothymamo/eu-sov-cloud/test/internal/yamlread"
)

// releaseWorkflowName is the one workflow that may publish: ReleaseWorkflow
// holds it to a closed schema.
const releaseWorkflowName = ".github/workflows/release.yml"

// publishingScopes are the permissions through which a workflow pushes to a
// registry, mints an OIDC identity to sign with, or stores attestations.
var publishingScopes = []string{"packages", "id-token", "attestations"}

// tagEvents are the events, other than a push, that GitHub runs a workflow for
// when a tag is created or released: a created tag or branch, a published
// release, and a run of another workflow, which a tag may have started.
var tagEvents = []string{"create", "release", "workflow_run"}

// WorkflowPrivileges returns a message for everything in the workflow
// document src, at the slash-separated path name, that publishes a release
// when name is not .github/workflows/release.yml: a packages: write,
// id-token: write or attestations: write permission, or write-all, at the
// workflow's level or a job's; an on: push: tags: trigger, or one of the other
// events GitHub runs for a tag (create, release, workflow_run), in any form of
// on:; and a workflow with no top-level permissions:, which takes the
// repository's default, whatever a job's own permissions: say. The release
// workflow is the one place reviewed for those; ReleaseWorkflow reads it.
//
// It reads only the top-level on: and permissions:, and the permissions: of
// every job, with the strict line reader, and refuses what it cannot read
// there (a flow mapping of permissions other than {}, a key it does not
// understand) rather than guess at it. Any other permission, a read scope or
// security-events: write among them, and any other trigger are accepted:
// workflow_dispatch too, although it can run on a tag ref, because the
// publishing scopes are refused anyway.
func WorkflowPrivileges(name string, src []byte) []string {
	if name == releaseWorkflowName {
		return nil
	}

	lines, err := yamlread.SplitLines(src)
	if err != nil {
		return []string{fmt.Sprintf("%s: read the workflow: %v", name, err)}
	}

	if len(lines) == 0 {
		return []string{fmt.Sprintf("%s: the workflow is empty, so this check cannot tell what it may do", name)}
	}

	top, err := yamlread.Mapping(lines)
	if err != nil {
		return []string{fmt.Sprintf("%s: read the workflow: %v", name, err)}
	}

	var messages []string

	declared := false

	for _, e := range top {
		switch e.Key {
		case "on":
			messages = append(messages, tagTriggers(name, e)...)
		case "permissions":
			declared = true

			messages = append(messages, publishingPermissions(name, e, "permissions")...)
		case "jobs":
			messages = append(messages, jobPermissions(name, e)...)
		}
	}

	if !declared {
		messages = append(messages, fmt.Sprintf("%s: the workflow has no top-level permissions:, so it runs with the repository's default token permissions, which may grant packages: write; declare them at the top level (permissions: {} or a block mapping)", name))
	}

	return messages
}

// tagTriggers returns a message when e, the workflow's on:, holds a push: of
// tags (or tags-ignore, which runs on every other tag), or one of tagEvents.
func tagTriggers(name string, e yamlread.Entry) []string {
	if e.Value != "" {
		// on: push or on: [push, pull_request]: a push of branches and tags
		// alike.
		events, err := yamlread.List(e)
		if err != nil {
			var one string

			one, err = yamlread.Single(e)
			events = []string{one}
		}

		if err != nil {
			return []string{fmt.Sprintf("%s: read on: %v", name, err)}
		}

		if slices.Contains(events, "push") {
			return []string{fmt.Sprintf("%s: line %d: on: %s runs on a push of any tag; only %s may run on a tag", name, e.Num, e.Value, releaseWorkflowName)}
		}

		for _, event := range events {
			if slices.Contains(tagEvents, event) {
				return []string{fmt.Sprintf("%s: line %d: on: %s runs on %s, which a tag may start; only %s may run on a tag", name, e.Num, e.Value, event, releaseWorkflowName)}
			}
		}

		return nil
	}

	triggers, err := yamlread.Mapping(e.Block)
	if err != nil {
		return []string{fmt.Sprintf("%s: read on: %v", name, err)}
	}

	for _, trigger := range triggers {
		if slices.Contains(tagEvents, trigger.Key) {
			return []string{fmt.Sprintf("%s: line %d: on: %s runs the workflow for a tag; only %s may run on a tag", name, trigger.Num, trigger.Key, releaseWorkflowName)}
		}

		if trigger.Key != "push" {
			continue
		}

		if trigger.Value != "" {
			return []string{fmt.Sprintf("%s: line %d: on: push is %q, which this check does not read and which may run on a tag; only %s may run on a tag", name, trigger.Num, trigger.Value, releaseWorkflowName)}
		}

		push, err := yamlread.Mapping(trigger.Block)
		if err != nil {
			return []string{fmt.Sprintf("%s: read on: push: %v", name, err)}
		}

		if len(push) == 0 {
			return []string{fmt.Sprintf("%s: line %d: on: push runs on a push of any tag; only %s may run on a tag", name, trigger.Num, releaseWorkflowName)}
		}

		for _, p := range push {
			if p.Key == "tags" || p.Key == "tags-ignore" {
				return []string{fmt.Sprintf("%s: line %d: on: push: %s runs the workflow on a tag; only %s may", name, p.Num, p.Key, releaseWorkflowName)}
			}
		}
	}

	return nil
}

// jobPermissions returns a message for every publishing permission of every
// job of e, the workflow's jobs:.
func jobPermissions(name string, e yamlread.Entry) []string {
	jobs, err := yamlread.Mapping(e.Block)
	if err != nil {
		return []string{fmt.Sprintf("%s: read jobs: %v", name, err)}
	}

	var messages []string

	for _, job := range jobs {
		keys, err := yamlread.Mapping(job.Block)
		if err != nil {
			messages = append(messages, fmt.Sprintf("%s: read jobs: %s: %v", name, job.Key, err))

			continue
		}

		for _, k := range keys {
			if k.Key == "permissions" {
				messages = append(messages, publishingPermissions(name, k, "jobs: "+job.Key+": permissions")...)
			}
		}
	}

	return messages
}

// publishingPermissions returns a message for e, a permissions: at path,
// when it is write-all or grants a publishing scope write.
func publishingPermissions(name string, e yamlread.Entry, path string) []string {
	if e.Value != "" {
		if len(e.Block) != 0 {
			return []string{fmt.Sprintf("%s: line %d: %s has a value on its line and lines below it", name, e.Num, path)}
		}

		switch e.Value {
		case "{}", "read-all":
			return nil
		case "write-all":
			return []string{fmt.Sprintf("%s: line %d: %s is write-all, which grants %s write; only %s may", name, e.Num, path, strings.Join(publishingScopes, ", "), releaseWorkflowName)}
		default:
			return []string{fmt.Sprintf("%s: line %d: %s is %q, which this check does not read; write a block mapping", name, e.Num, path, e.Value)}
		}
	}

	scopes, err := yamlread.Mapping(e.Block)
	if err != nil {
		return []string{fmt.Sprintf("%s: read %s: %v", name, path, err)}
	}

	var messages []string

	for _, scope := range scopes {
		value, err := yamlread.Single(scope)
		if err != nil {
			messages = append(messages, fmt.Sprintf("%s: %s: %v", name, path, err))

			continue
		}

		for _, publishing := range publishingScopes {
			if scope.Key == publishing && value != "read" && value != "none" {
				messages = append(messages, fmt.Sprintf("%s: line %d: %s: %s is %s; only %s may publish to a registry, mint an OIDC identity or store attestations", name, scope.Num, path, scope.Key, value, releaseWorkflowName))
			}
		}
	}

	return messages
}
