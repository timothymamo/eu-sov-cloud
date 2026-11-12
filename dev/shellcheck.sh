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
# --norc keeps every .shellcheckrc out: shellcheck otherwise reads one from the script's directory and each
# parent, then ~/.shellcheckrc or $XDG_CONFIG_HOME/shellcheckrc, any of which could silence a check and
# make a native run report less than CI's. --severity=style reports everything, shellcheck's default,
# stated so it cannot drift: SC2086, the word splitting a quoting bug causes, is only info. "--" ends the
# options, so no file name is read as one. A finding is fixed, or silenced on its line with
# "# shellcheck disable=SCnnnn # reason", which test/lint holds to a code and a reason.
#
# The shellcheck run is whichever the PATH finds: the pinned release dev/lima.yaml installs in the VM and CI
# installs in lint-shell. Without one the script fails, rather than check nothing.
#
# Usage: dev/shellcheck.sh   (from the repository root, where make runs it)
set -euo pipefail

if ! command -v shellcheck >/dev/null; then
  echo >&2 "shellcheck: shellcheck is not installed: re-provision the VM or see docs/contributing/development.md"
  exit 1
fi

# The list goes through a file, not a process substitution, so a failing git stops the script. Bash 3.2,
# macOS's, runs this too: no mapfile.
tracked="$(mktemp)"
trap 'rm -f "${tracked}"' EXIT
git ls-files -z >"${tracked}"

scripts=()
while IFS= read -r -d '' file; do
  # Tracked but not in this checkout, as a sparse one has it, or not a regular file (a symbolic link or a
  # submodule): test/architecture's trackedShellScripts skips the same files.
  [ -f "${file}" ] && [ ! -L "${file}" ] || continue
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

shellcheck --norc --severity=style -- "${scripts[@]}"
