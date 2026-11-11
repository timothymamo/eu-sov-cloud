package check

import (
	"fmt"
	"slices"
	"strings"
)

// releaseWorkflowName is the one workflow that may publish: ReleaseWorkflow
// holds it to a closed schema.
const releaseWorkflowName = ".github/workflows/release.yml"

// publishingScopes are the permissions through which a workflow pushes to a
// registry, mints an OIDC identity to sign with, or stores attestations.
var publishingScopes = []string{"packages", "id-token", "attestations"}

// WorkflowPrivileges returns a message for everything in the workflow
// document src, at the slash-separated path name, that publishes a release
// when name is not .github/workflows/release.yml: a packages: write,
// id-token: write or attestations: write permission, or write-all, at the
// workflow's level or a job's, and an on: push: tags: trigger. The release
// workflow is the one place reviewed for those; ReleaseWorkflow reads it.
//
// It reads only the top-level on: and permissions:, and the permissions: of
// every job, with the strict line reader, and refuses what it cannot read
// there (a flow mapping of permissions other than {}, a key it does not
// understand) rather than guess at it. Any other permission, a read scope or
// security-events: write among them, and any other trigger are accepted.
func WorkflowPrivileges(name string, src []byte) []string {
	if name == releaseWorkflowName {
		return nil
	}

	lines, err := splitLines(src)
	if err != nil {
		return []string{fmt.Sprintf("%s: read the workflow: %v", name, err)}
	}

	if len(lines) == 0 {
		return []string{fmt.Sprintf("%s: the workflow is empty, so this check cannot tell what it may do", name)}
	}

	top, err := mapping(lines)
	if err != nil {
		return []string{fmt.Sprintf("%s: read the workflow: %v", name, err)}
	}

	var messages []string

	for _, e := range top {
		switch e.key {
		case "on":
			messages = append(messages, tagTriggers(name, e)...)
		case "permissions":
			messages = append(messages, publishingPermissions(name, e, "permissions")...)
		case "jobs":
			messages = append(messages, jobPermissions(name, e)...)
		}
	}

	return messages
}

// tagTriggers returns a message when e, the workflow's on:, holds a push: of
// tags (or tags-ignore, which runs on every other tag).
func tagTriggers(name string, e entry) []string {
	if e.value != "" {
		// on: push or on: [push, pull_request]: a push of branches and tags
		// alike.
		events, err := list(e)
		if err != nil {
			var one string

			one, err = single(e)
			events = []string{one}
		}

		if err != nil {
			return []string{fmt.Sprintf("%s: read on: %v", name, err)}
		}

		if slices.Contains(events, "push") {
			return []string{fmt.Sprintf("%s: line %d: on: %s runs on a push of any tag; only %s may run on a tag", name, e.num, e.value, releaseWorkflowName)}
		}

		return nil
	}

	triggers, err := mapping(e.block)
	if err != nil {
		return []string{fmt.Sprintf("%s: read on: %v", name, err)}
	}

	for _, trigger := range triggers {
		if trigger.key != "push" {
			continue
		}

		if trigger.value != "" {
			return []string{fmt.Sprintf("%s: line %d: on: push is %q, which this check does not read and which may run on a tag; only %s may run on a tag", name, trigger.num, trigger.value, releaseWorkflowName)}
		}

		push, err := mapping(trigger.block)
		if err != nil {
			return []string{fmt.Sprintf("%s: read on: push: %v", name, err)}
		}

		if len(push) == 0 {
			return []string{fmt.Sprintf("%s: line %d: on: push runs on a push of any tag; only %s may run on a tag", name, trigger.num, releaseWorkflowName)}
		}

		for _, p := range push {
			if p.key == "tags" || p.key == "tags-ignore" {
				return []string{fmt.Sprintf("%s: line %d: on: push: %s runs the workflow on a tag; only %s may", name, p.num, p.key, releaseWorkflowName)}
			}
		}
	}

	return nil
}

// jobPermissions returns a message for every publishing permission of every
// job of e, the workflow's jobs:.
func jobPermissions(name string, e entry) []string {
	jobs, err := mapping(e.block)
	if err != nil {
		return []string{fmt.Sprintf("%s: read jobs: %v", name, err)}
	}

	var messages []string

	for _, job := range jobs {
		keys, err := mapping(job.block)
		if err != nil {
			messages = append(messages, fmt.Sprintf("%s: read jobs: %s: %v", name, job.key, err))

			continue
		}

		for _, k := range keys {
			if k.key == "permissions" {
				messages = append(messages, publishingPermissions(name, k, "jobs: "+job.key+": permissions")...)
			}
		}
	}

	return messages
}

// publishingPermissions returns a message for e, a permissions: at path,
// when it is write-all or grants a publishing scope write.
func publishingPermissions(name string, e entry, path string) []string {
	if e.value != "" {
		if len(e.block) != 0 {
			return []string{fmt.Sprintf("%s: line %d: %s has a value on its line and lines below it", name, e.num, path)}
		}

		switch e.value {
		case "{}", "read-all":
			return nil
		case "write-all":
			return []string{fmt.Sprintf("%s: line %d: %s is write-all, which grants %s write; only %s may", name, e.num, path, strings.Join(publishingScopes, ", "), releaseWorkflowName)}
		default:
			return []string{fmt.Sprintf("%s: line %d: %s is %q, which this check does not read; write a block mapping", name, e.num, path, e.value)}
		}
	}

	scopes, err := mapping(e.block)
	if err != nil {
		return []string{fmt.Sprintf("%s: read %s: %v", name, path, err)}
	}

	var messages []string

	for _, scope := range scopes {
		value, err := single(scope)
		if err != nil {
			messages = append(messages, fmt.Sprintf("%s: %s: %v", name, path, err))

			continue
		}

		for _, publishing := range publishingScopes {
			if scope.key == publishing && value != "read" && value != "none" {
				messages = append(messages, fmt.Sprintf("%s: line %d: %s: %s is %s; only %s may publish to a registry, mint an OIDC identity or store attestations", name, scope.num, path, scope.key, value, releaseWorkflowName))
			}
		}
	}

	return messages
}
