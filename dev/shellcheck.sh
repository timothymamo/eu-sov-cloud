#!/usr/bin/env bash
# Runs shellcheck on every shell script git tracks, for make lint and CI (issue #164,
# docs/adr/0022-lint-shell-scripts-and-workflows-with-shellcheck-and-actionlint.md).
#
# A shell script is a tracked file whose name ends in .sh, or whose first line is a #! shebang that runs
# sh or bash, directly or through env, skipping env's options and VAR=value assignments: the hooks under
# .githooks are scripts by their shebang. It is the rule IsShellScript in test/lint/shellcheckdirective.go
# states, and test/architecture's TestMakeLintRunsEveryReleaseTarget fails when the two pick different
# files. The list comes from git, so a new script is checked without editing anything.
#
# --severity=style reports everything, shellcheck's default, stated so it cannot drift: SC2086, the word
# splitting a quoting bug causes, is only info. A finding is fixed, or silenced on its line with
# "# shellcheck disable=SCnnnn # reason", which test/lint holds to a code and a reason.
#
# Usage: dev/shellcheck.sh   (from the repository root, where make runs it)
set -euo pipefail

# The list goes through a file, not a process substitution, so a failing git stops the script. Bash 3.2,
# macOS's, runs this too: no mapfile.
tracked="$(mktemp)"
trap 'rm -f "${tracked}"' EXIT
git ls-files -z >"${tracked}"

scripts=()
while IFS= read -r -d '' file; do
  [ -f "${file}" ] || continue # tracked but not in this checkout, as a sparse one has it
  if [[ "${file}" == *.sh ]]; then
    scripts+=("${file}")
    continue
  fi
  first=""
  IFS= read -r first <"${file}" || true
  first="${first%$'\r'}"
  [[ "${first}" == '#!'* ]] || continue
  read -r -a words <<<"${first#??}"
  [ "${#words[@]}" -gt 0 ] || continue
  interpreter="${words[0]##*/}"
  if [ "${interpreter}" = env ]; then
    interpreter=""
    for word in "${words[@]:1}"; do
      case "${word}" in
        -* | *=*) continue ;;
      esac
      interpreter="${word##*/}"
      break
    done
  fi
  case "${interpreter}" in
    sh | bash) scripts+=("${file}") ;;
  esac
done <"${tracked}"

if [ "${#scripts[@]}" -eq 0 ]; then
  echo >&2 "shellcheck: git tracks no shell script, so the walk is broken"
  exit 1
fi

shellcheck --severity=style "${scripts[@]}"
