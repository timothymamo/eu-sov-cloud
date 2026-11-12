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

# The fake gh: "gh api [--method M] [-H h] [--jq f] [--silent] [-f k=v ...] path". It logs
# "METHOD path k=v ..." to calls, and answers from the response stored for "METHOD path", or {} for a
# write without one; a read without one fails as an HTTP 404 would.
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
  PATH="${work}/bin:${PATH}" FAKE_GH="${FAKE_GH}" GH_TOKEN=fake GH_REPO="${repo}" \
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

runs() { echo "{\"total_count\": $1}"; }
# guard_runs answers the guard's read of release.yml's runs: queued and in_progress as given, the
# other unfinished statuses none.
guard_runs() {
  for status in requested waiting pending; do respond GET "repos/${repo}/actions/workflows/release.yml/runs?status=${status}&per_page=1" "$(runs 0)"; done
  respond GET "repos/${repo}/actions/workflows/release.yml/runs?status=queued&per_page=1" "$(runs "$1")"
  respond GET "repos/${repo}/actions/workflows/release.yml/runs?status=in_progress&per_page=1" "$(runs "$2")"
}

scenario guard-idle; guard_runs 0 0; run_step "guard: no release in flight" guard pass
scenario guard-queued; guard_runs 1 0; run_step "guard: a release run queued" guard fail
scenario guard-running; guard_runs 0 1; run_step "guard: a release run in progress" guard fail

pending="repos/${repo}/issues?state=closed&labels=autorelease:%20pending&per_page=100"

# merged_release answers for one merged release PR, #42, whose fields default to a well-formed release of
# version (the manifest's) with the given title; extra jq edits the pull request.
merged_release() {
  local version="$1" title="$2" edit="${3:-.}" ancestry="${4:-ahead}"
  respond GET "${pending}" '[{"number": 42, "pull_request": {}}, {"number": 7}]'
  respond GET "repos/${repo}/pulls/42" "$(jq -c "${edit}" <<<"{\"merged\": true, \"base\": {\"ref\": \"main\"}, \"head\": {\"ref\": \"release-please--branches--main--components--shift2eu\", \"repo\": {\"full_name\": \"${repo}\"}}, \"merge_commit_sha\": \"${merge}\", \"title\": \"${title}\"}")"
  respond GET "repos/${repo}/compare/${merge}...main" "{\"status\": \"${ancestry}\"}"
  respond GET "repos/${repo}/contents/.release-please-manifest.json?ref=${merge}" "{\".\": \"${version}\"}"
  respond GET "repos/${repo}/git/matching-refs/tags/v${version}" '[]'
  respond POST "repos/${repo}/git/tags" "{\"sha\": \"${object}\"}"
}

scenario none; respond GET "${pending}" '[{"number": 7}]'
run_step "tag: no merged release PR" tag pass; wrote_nothing "tag: no merged release PR"

scenario release; merged_release 0.3.0 "chore(main): release 0.3.0"
run_step "tag: a merged release PR" tag pass
wrote "tag: a merged release PR" "^POST repos/${repo}/git/tags tag=v0\.3\.0 message=v0\.3\.0 object=${merge} type=commit$"
wrote "tag: a merged release PR" "^POST repos/${repo}/git/refs ref=refs/tags/v0\.3\.0 sha=${object}$"
wrote "tag: a merged release PR" "^DELETE repos/${repo}/issues/42/labels/autorelease:%20pending"
wrote "tag: a merged release PR" "^POST repos/${repo}/issues/42/labels labels\[\]=autorelease: tagged$"

scenario two; merged_release 0.3.0 "chore(main): release 0.3.0"
respond GET "${pending}" '[{"number": 42, "pull_request": {}}, {"number": 43, "pull_request": {}}]'
run_step "tag: two pending release PRs" tag fail; wrote_nothing "tag: two pending release PRs"

for bad in "0.3" "0.3.0-rc.1" "v0.3.0" "0.3.0 ; touch pwned" '0.3.0$(touch pwned)'; do
  scenario "version-$(printf '%s' "${bad}" | sha256sum | cut -c1-8)"; merged_release "${bad}" "chore(main): release ${bad}"
  run_step "tag: manifest version '${bad}'" tag fail; wrote_nothing "tag: manifest version '${bad}'"
done

scenario title; merged_release 0.3.0 'chore(main): release 0.4.0'
run_step "tag: a title naming another version" tag fail; wrote_nothing "tag: a title naming another version"

scenario unmerged; merged_release 0.3.0 "chore(main): release 0.3.0" '.merged = false'
run_step "tag: a PR closed without merging" tag fail; wrote_nothing "tag: a PR closed without merging"

scenario base; merged_release 0.3.0 "chore(main): release 0.3.0" '.base.ref = "next"'
run_step "tag: a PR merged into another branch" tag fail; wrote_nothing "tag: a PR merged into another branch"

scenario head; merged_release 0.3.0 "chore(main): release 0.3.0" '.head.ref = "feat/1-x"'
run_step "tag: a PR from another branch" tag fail; wrote_nothing "tag: a PR from another branch"

scenario fork; merged_release 0.3.0 "chore(main): release 0.3.0" '.head.repo.full_name = "someone/shift2eu"'
run_step "tag: a PR from a fork" tag fail; wrote_nothing "tag: a PR from a fork"

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
if grep -q "git/" <(grep -vE '^GET ' "${FAKE_GH}/calls"); then
  echo "FAIL tag: the tag exists on the merge commit: the step created a tag"; failures=$((failures + 1))
fi

if [ -e pwned ] || [ -e "${work}/pwned" ]; then echo "FAIL a manifest version ran as a command"; failures=$((failures + 1)); fi
if [ "${failures}" != 0 ]; then
  echo "${failures} failure(s)" >&2
  exit 1
fi
echo "every scenario passed"
