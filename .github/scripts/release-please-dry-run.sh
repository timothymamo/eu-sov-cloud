#!/usr/bin/env bash
# The dry run of .github/workflows/release-please.yml (issue #142, ADR 0021), run by ci.yml: it reads the
# scripts of the workflow's guard and tag steps out of the workflow itself and runs them against a fake gh
# that answers from canned API responses and records every call, so no network is reached and nothing is
# written. Each scenario states what the step must do: exit 0 or fail, and which writes it makes (the tag
# object, the tag ref, the relabelling) or that it makes none.
#
# Usage: .github/scripts/release-please-dry-run.sh [workflow]
set -euo pipefail

workflow="${1:-.github/workflows/release-please.yml}"
work="$(mktemp -d)"
trap 'rm -rf "${work}"' EXIT

# step_script prints the run: block of the step with the given id: the lines below "run: |", their
# indentation removed, up to the first line indented no further than run: itself.
step_script() {
  awk -v id="$1" '
    $0 ~ "^ +id: " id "$" { found = 1; next }
    found && !inrun && match($0, /^ +run: \|$/) { inrun = 1; indent = RLENGTH - length("run: |"); next }
    found && !inrun && /^ +- / { found = 0 }
    inrun {
      if ($0 != "" && match($0, /^ */) && RLENGTH <= indent) exit
      print substr($0, indent + 3)
    }
  ' "${workflow}"
}

step_script guard >"${work}/guard.sh"
step_script tag >"${work}/tag.sh"
for step in guard tag; do
  grep -q 'set -euo pipefail' "${work}/${step}.sh" || { echo "could not read the ${step} step's script from ${workflow}" >&2; exit 1; }
done

# The fake gh: "gh api [--method M] [-H h] [--jq f] [--silent] [--paginate] [-f k=v ...] path". It
# logs "METHOD path k=v ..." to calls, and answers from the response stored for "METHOD path", or {} for
# a write without one; a read without one fails as an HTTP 404 would. A response is one page, so
# --paginate changes nothing.
mkdir "${work}/bin"
cat >"${work}/bin/gh" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
[ "$1" = api ] || { echo "fake gh: unexpected command $1" >&2; exit 2; }
shift
method=GET jq_filter="" silent=0 path="" fields=()
while [ "$#" -gt 0 ]; do
  case "$1" in
    --method) method="$2"; shift 2 ;;
    -H) shift 2 ;;
    --jq) jq_filter="$2"; shift 2 ;;
    --silent) silent=1; shift ;;
    --paginate) shift ;;
    -f) fields+=("$2"); shift 2 ;;
    -*) echo "fake gh: unexpected flag $1" >&2; exit 2 ;;
    *) path="$1"; shift ;;
  esac
done
echo "${method} ${path} ${fields[*]:-}" >>"${FAKE_GH}/calls"
key="$(printf '%s %s' "${method}" "${path}" | sha256sum | cut -c1-16)"
if [ -f "${FAKE_GH}/${key}" ]; then
  body="$(cat "${FAKE_GH}/${key}")"
elif [ "${method}" != GET ]; then
  body='{}'
else
  echo "fake gh: HTTP 404 for ${method} ${path}" >&2
  exit 1
fi
[ "${silent}" = 1 ] && exit 0
if [ -n "${jq_filter}" ]; then jq -r "${jq_filter}" <<<"${body}"; else printf '%s\n' "${body}"; fi
FAKE
chmod +x "${work}/bin/gh"

repo=example/shift2eu
slug=release-app
bot="${slug}[bot]"
branch=release-please--branches--main--components--shift2eu
merge=1111111111111111111111111111111111111111
object=2222222222222222222222222222222222222222
other=3333333333333333333333333333333333333333
failures=0

# A scenario's responses live in a directory of their own.
scenario() { FAKE_GH="${work}/scenario-$1"; mkdir "${FAKE_GH}"; : >"${FAKE_GH}/calls"; }
respond() { printf '%s\n' "$3" >"${FAKE_GH}/$(printf '%s %s' "$1" "$2" | sha256sum | cut -c1-16)"; }

# run_step runs a step's script as the workflow would and compares its exit with want (pass or fail).
run_step() {
  local name="$1" step="$2" want="$3" got=pass
  PATH="${work}/bin:${PATH}" FAKE_GH="${FAKE_GH}" GH_TOKEN=fake GH_REPO="${repo}" APP_SLUG="${slug}" \
    bash "${work}/${step}.sh" >"${FAKE_GH}/out" 2>&1 || got=fail
  if [ "${got}" != "${want}" ]; then
    echo "FAIL ${name}: the ${step} step's result is ${got}, want ${want}"; sed 's/^/    /' "${FAKE_GH}/out"
    failures=$((failures + 1))
  else
    echo "ok   ${name}"
  fi
}
# wrote checks that a call matching the pattern was made; wrote_nothing that no write was.
wrote() {
  grep -qE -- "$2" "${FAKE_GH}/calls" || { echo "FAIL $1: no call matches $2"; sed 's/^/    /' "${FAKE_GH}/calls"; failures=$((failures + 1)); }
}
wrote_nothing() {
  if grep -qvE '^GET ' "${FAKE_GH}/calls"; then
    echo "FAIL $1: the step wrote"; sed 's/^/    /' "${FAKE_GH}/calls"; failures=$((failures + 1))
  fi
}
# said checks that the step's output holds a line matching the pattern; never_said that none does.
said() {
  grep -qE -- "$2" "${FAKE_GH}/out" || { echo "FAIL $1: no output line matches $2"; sed 's/^/    /' "${FAKE_GH}/out"; failures=$((failures + 1)); }
}
never_said() {
  if grep -qE -- "$2" "${FAKE_GH}/out"; then
    echo "FAIL $1: an output line matches $2"; sed 's/^/    /' "${FAKE_GH}/out"; failures=$((failures + 1))
  fi
}

# The steps' one listing: the closed PRs into main from release-please's branch, as the pulls endpoint
# returns them.
pending="repos/${repo}/pulls?state=closed&base=main&head=${repo%%/*}:${branch}&per_page=100"

# Items of that listing: a merged release PR of the App labelled autorelease: pending, a PR closed
# without merging, a merged PR of someone else, both labelled autorelease: pending too, and an earlier
# release PR of the App, already tagged and labelled autorelease: tagged alone.
merged_pr() { echo "{\"number\": $1, \"user\": {\"login\": \"${2:-${bot}}\"}, \"merged_at\": \"2026-11-12T10:00:00Z\", \"labels\": [{\"name\": \"autorelease: pending\"}]}"; }
unmerged_pr() { echo "{\"number\": $1, \"user\": {\"login\": \"${bot}\"}, \"merged_at\": null, \"labels\": [{\"name\": \"autorelease: pending\"}]}"; }
tagged_pr="{\"number\": 7, \"user\": {\"login\": \"${bot}\"}, \"merged_at\": \"2026-10-01T10:00:00Z\", \"labels\": [{\"name\": \"autorelease: tagged\"}]}"

runs() { echo "{\"total_count\": $1}"; }
# guard_runs answers the guard's read of release.yml's runs: one run in the status given, none in the
# others ("" for none at all).
guard_runs() {
  local status n
  for status in requested waiting pending queued in_progress; do
    if [ "${status}" = "$1" ]; then n=1; else n=0; fi
    respond GET "repos/${repo}/actions/workflows/release.yml/runs?status=${status}&per_page=1" "$(runs "${n}")"
  done
}

scenario guard-idle; respond GET "${pending}" "[$(merged_pr 42)]"; guard_runs ""
run_step "guard: a merged release PR, no release in flight" guard pass; wrote_nothing "guard: idle"
for status in requested waiting pending queued in_progress; do
  scenario "guard-${status}"; respond GET "${pending}" "[$(merged_pr 42)]"; guard_runs "${status}"
  run_step "guard: a merged release PR, a release run ${status}" guard fail; wrote_nothing "guard: a release run ${status}"
  scenario "guard-nothing-${status}"; respond GET "${pending}" "[$(unmerged_pr 41), $(merged_pr 43 someone), ${tagged_pr}]"; guard_runs "${status}"
  run_step "guard: nothing to tag, a release run ${status}" guard pass; wrote_nothing "guard: nothing to tag, ${status}"
done

# merged_release answers for one merged release PR, #42, whose fields default to a well-formed release of
# version (the manifest's) with the given title; extra jq edits the pull request.
merged_release() {
  local version="$1" title="$2" edit="${3:-.}" ancestry="${4:-ahead}"
  respond GET "${pending}" "[$(merged_pr 42), ${tagged_pr}]"
  respond GET "repos/${repo}/pulls/42" "$(jq -c --arg version "${title}" "${edit}" <<<"{\"merged\": true, \"user\": {\"login\": \"${bot}\"}, \"base\": {\"ref\": \"main\"}, \"head\": {\"ref\": \"${branch}\", \"repo\": {\"full_name\": \"${repo}\"}}, \"merge_commit_sha\": \"${merge}\", \"title\": \"${title}\"}")"
  respond GET "repos/${repo}/pulls/42/files?per_page=100" '[{"filename": "CHANGELOG.md"}, {"filename": ".release-please-manifest.json"}]'
  respond GET "repos/${repo}/compare/${merge}...main" "{\"status\": \"${ancestry}\"}"
  respond GET "repos/${repo}/contents/.release-please-manifest.json?ref=${merge}" "$(jq -nc --arg v "${version}" '{".": $v}')"
  respond GET "repos/${repo}/git/matching-refs/tags/v${version}" '[]'
  respond POST "repos/${repo}/git/tags" "{\"sha\": \"${object}\"}"
}
# tagged_42 checks the writes of a release of #42 as v0.3.0: the tag object, the ref, then the label
# autorelease: tagged before autorelease: pending is removed.
tagged_42() {
  wrote "$1" "^POST repos/${repo}/git/tags tag=v0\.3\.0 message=v0\.3\.0 object=${merge} type=commit$"
  wrote "$1" "^POST repos/${repo}/git/refs ref=refs/tags/v0\.3\.0 sha=${object}$"
  local added removed
  added="$(grep -nE "^POST repos/${repo}/issues/42/labels " "${FAKE_GH}/calls" | cut -d: -f1 | head -n 1)"
  removed="$(grep -nE "^DELETE repos/${repo}/issues/42/labels/" "${FAKE_GH}/calls" | cut -d: -f1 | head -n 1)"
  if [ -z "${added}" ] || [ -z "${removed}" ] || [ "${added}" -gt "${removed}" ]; then
    echo "FAIL $1: want autorelease: tagged added, then autorelease: pending removed"; sed 's/^/    /' "${FAKE_GH}/calls"; failures=$((failures + 1))
  fi
  wrote "$1" "^POST repos/${repo}/issues/42/labels labels\[\]=autorelease: tagged$"
  wrote "$1" "^DELETE repos/${repo}/issues/42/labels/autorelease:%20pending"
  if grep -qE '/issues/(7|41|43|44)/' "${FAKE_GH}/calls"; then echo "FAIL $1: another PR was relabelled"; failures=$((failures + 1)); fi
}

scenario none; respond GET "${pending}" "[${tagged_pr}]"
run_step "tag: no merged release PR" tag pass; wrote_nothing "tag: no merged release PR"

scenario empty; respond GET "${pending}" '[]'
run_step "tag: no closed release PR at all" tag pass; wrote_nothing "tag: no closed release PR at all"

scenario release; merged_release 0.3.0 "chore(main): release 0.3.0"
run_step "tag: a merged release PR" tag pass; tagged_42 "tag: a merged release PR"

scenario unmerged-only; respond GET "${pending}" "[$(unmerged_pr 41), ${tagged_pr}]"
run_step "tag: only a PR closed without merging" tag pass; wrote_nothing "tag: only a PR closed without merging"
said "tag: only a PR closed without merging" '^::warning::#41 .*closed without merging'

scenario unmerged-and-merged; merged_release 0.3.0 "chore(main): release 0.3.0"
respond GET "${pending}" "[$(unmerged_pr 41), $(merged_pr 42)]"
run_step "tag: a PR closed without merging beside a merged one" tag pass; tagged_42 "tag: a PR closed without merging beside a merged one"
said "tag: a PR closed without merging beside a merged one" '^::warning::#41 '

scenario author-only; respond GET "${pending}" "[$(merged_pr 43 someone)]"
run_step "tag: only a PR of another author" tag pass; wrote_nothing "tag: only a PR of another author"
said "tag: only a PR of another author" "^::warning::#43 .*not opened by ${slug}\[bot\]"

scenario author-and-app; merged_release 0.3.0 "chore(main): release 0.3.0"
respond GET "${pending}" "[$(merged_pr 43 someone), $(merged_pr 42)]"
run_step "tag: a PR of another author beside the App's" tag pass; tagged_42 "tag: a PR of another author beside the App's"

scenario author-pull; merged_release 0.3.0 "chore(main): release 0.3.0" '.user.login = "someone"'
run_step "tag: a pull request whose author is not the App" tag fail; wrote_nothing "tag: a pull request whose author is not the App"

scenario two; merged_release 0.3.0 "chore(main): release 0.3.0"
respond GET "${pending}" "[$(merged_pr 42), $(merged_pr 44)]"
run_step "tag: two merged release PRs" tag fail; wrote_nothing "tag: two merged release PRs"

# A triage user re-applies autorelease: pending to #7, an earlier release PR of the App already tagged:
# two candidates, so the step fails closed rather than pick one.
scenario relabelled; merged_release 0.3.0 "chore(main): release 0.3.0"
respond GET "${pending}" "[$(merged_pr 42), $(jq -c '.labels += [{"name": "autorelease: pending"}]' <<<"${tagged_pr}")]"
run_step "tag: an old tagged release PR labelled autorelease: pending again" tag fail; wrote_nothing "tag: an old tagged release PR labelled again"

for bad in "0.3" "0.3.0-rc.1" "v0.3.0" "0.3.0 ; touch pwned" '0.3.0$(touch pwned)'; do
  scenario "version-$(printf '%s' "${bad}" | sha256sum | cut -c1-8)"; merged_release "${bad}" "chore(main): release ${bad}"
  run_step "tag: manifest version '${bad}'" tag fail; wrote_nothing "tag: manifest version '${bad}'"
done

# A version holding a newline and a workflow command is printed quoted, on one line.
scenario version-command; merged_release $'0.3.0\n::error::injected' "chore(main): release 0.3.0"
run_step "tag: a manifest version holding a workflow command" tag fail; wrote_nothing "tag: a manifest version holding a workflow command"
never_said "tag: a manifest version holding a workflow command" '^::error::injected'

scenario title; merged_release 0.3.0 'chore(main): release 0.4.0'
run_step "tag: a title naming another version" tag fail; wrote_nothing "tag: a title naming another version"

scenario unmerged; merged_release 0.3.0 "chore(main): release 0.3.0" '.merged = false'
run_step "tag: a listed merged PR that the pull request says is not merged" tag fail; wrote_nothing "tag: pull request not merged"

scenario base; merged_release 0.3.0 "chore(main): release 0.3.0" '.base.ref = "next"'
run_step "tag: a PR merged into another branch" tag fail; wrote_nothing "tag: a PR merged into another branch"

scenario head; merged_release 0.3.0 "chore(main): release 0.3.0" '.head.ref = "feat/1-x"'
run_step "tag: a PR from another branch" tag fail; wrote_nothing "tag: a PR from another branch"

scenario head-merged; merged_release 0.3.0 "chore(main): release 0.3.0" '.head.ref = "release-please--branches--main"'
run_step "tag: a PR from the merge plugin's branch" tag fail; wrote_nothing "tag: a PR from the merge plugin's branch"

scenario head-component; merged_release 0.3.0 "chore(main): release 0.3.0" '.head.ref = "release-please--branches--main--components--other"'
run_step "tag: a PR from another component's branch" tag fail; wrote_nothing "tag: a PR from another component's branch"

scenario fork; merged_release 0.3.0 "chore(main): release 0.3.0" '.head.repo.full_name = "someone/shift2eu"'
run_step "tag: a PR from a fork" tag fail; wrote_nothing "tag: a PR from a fork"

scenario files-other; merged_release 0.3.0 "chore(main): release 0.3.0"
respond GET "repos/${repo}/pulls/42/files?per_page=100" '[{"filename": "CHANGELOG.md"}, {"filename": ".github/workflows/release.yml"}]'
run_step "tag: a PR changing another file" tag fail; wrote_nothing "tag: a PR changing another file"

scenario files-third; merged_release 0.3.0 "chore(main): release 0.3.0"
respond GET "repos/${repo}/pulls/42/files?per_page=100" '[{"filename": "CHANGELOG.md"}, {"filename": ".release-please-manifest.json"}, {"filename": "go.mod"}]'
run_step "tag: a PR changing a third file" tag fail; wrote_nothing "tag: a PR changing a third file"

scenario files-renamed; merged_release 0.3.0 "chore(main): release 0.3.0"
respond GET "repos/${repo}/pulls/42/files?per_page=100" '[{"filename": "CHANGELOG.md", "previous_filename": "go.mod"}, {"filename": ".release-please-manifest.json"}]'
run_step "tag: a PR renaming another file" tag fail; wrote_nothing "tag: a PR renaming another file"

scenario files-no-manifest; merged_release 0.3.0 "chore(main): release 0.3.0"
respond GET "repos/${repo}/pulls/42/files?per_page=100" '[{"filename": "CHANGELOG.md"}]'
run_step "tag: a PR that leaves the manifest alone" tag fail; wrote_nothing "tag: a PR that leaves the manifest alone"

scenario diverged; merged_release 0.3.0 "chore(main): release 0.3.0" . diverged
run_step "tag: a merge commit not on main" tag fail; wrote_nothing "tag: a merge commit not on main"

scenario exists-elsewhere; merged_release 0.3.0 "chore(main): release 0.3.0"
respond GET "repos/${repo}/git/matching-refs/tags/v0.3.0" '[{"ref": "refs/tags/v0.3.0"}]'
respond GET "repos/${repo}/git/ref/tags/v0.3.0" "{\"object\": {\"type\": \"commit\", \"sha\": \"${other}\"}}"
run_step "tag: the tag exists on another commit" tag fail; wrote_nothing "tag: the tag exists on another commit"

scenario exists-here; merged_release 0.3.0 "chore(main): release 0.3.0"
respond GET "repos/${repo}/git/matching-refs/tags/v0.3.0" '[{"ref": "refs/tags/v0.3.0"}]'
respond GET "repos/${repo}/git/ref/tags/v0.3.0" "{\"object\": {\"type\": \"tag\", \"sha\": \"${object}\"}}"
respond GET "repos/${repo}/git/tags/${object}" "{\"object\": {\"type\": \"commit\", \"sha\": \"${merge}\"}}"
run_step "tag: the tag exists on the merge commit" tag pass
wrote "tag: the tag exists on the merge commit" "^POST repos/${repo}/issues/42/labels labels\[\]=autorelease: tagged$"
wrote "tag: the tag exists on the merge commit" "^DELETE repos/${repo}/issues/42/labels/autorelease:%20pending"
if grep -q "git/" <(grep -vE '^GET ' "${FAKE_GH}/calls"); then
  echo "FAIL tag: the tag exists on the merge commit: the step created a tag"; failures=$((failures + 1))
fi

if [ -e pwned ] || [ -e "${work}/pwned" ]; then echo "FAIL a manifest version ran as a command"; failures=$((failures + 1)); fi
if [ "${failures}" != 0 ]; then
  echo "${failures} failure(s)" >&2
  exit 1
fi
echo "every scenario passed"
