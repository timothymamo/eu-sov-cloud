#!/usr/bin/env bash
# The dry run of the release's archives (issue #147), run by ci.yml's test job: it builds them with a
# snapshot run of .goreleaser.yaml, which publishes nothing, and fails unless each archive holds exactly
# its own binary. The CLI's archives, shift2eu_<version>_<os>_<arch>, hold shift2eu (shift2eu.exe on
# Windows) and not shift2eu-server; the server's, shift2eu-server_<version>_<os>_<arch>, hold
# shift2eu-server (.exe) and not shift2eu. Every platform has one archive of each, and checksums.txt
# lists every archive. check.GoReleaserArchives pins the configuration itself.
#
# Usage: .github/scripts/verify-archives.sh   (from the repository root, with goreleaser on PATH)
set -euo pipefail

command -v goreleaser >/dev/null || { echo "goreleaser is not on PATH" >&2; exit 1; }

dist="dist"
goreleaser release --snapshot --clean >&2

fail=0
problem() {
  echo "$*" >&2
  fail=1
}

# entries prints the paths an archive holds, one per line.
entries() {
  case "$1" in
    *.tar.gz) tar -tzf "$1" ;;
    *.zip) unzip -Z1 "$1" ;;
    *) echo "$1 is neither .tar.gz nor .zip" >&2; return 1 ;;
  esac
}

# check_archive archive own other: the archive holds exactly one binary, own (or own.exe), at its root,
# and no file named other (or other.exe) anywhere.
check_archive() {
  local archive="$1" own="$2" other="$3" list
  list="$(entries "${archive}")"
  local owns
  owns="$(grep -cxE "${own}(\\.exe)?" <<<"${list}" || true)"
  if [ "${owns}" != 1 ]; then
    problem "${archive}: holds ${owns} ${own} binaries at its root, want exactly 1"
  fi
  if grep -qE "(^|/)${other}(\\.exe)?$" <<<"${list}"; then
    problem "${archive}: holds ${other}, which belongs in its own archive"
  fi
}

shopt -s nullglob
cli=("${dist}"/shift2eu_*.tar.gz "${dist}"/shift2eu_*.zip)
server=("${dist}"/shift2eu-server_*.tar.gz "${dist}"/shift2eu-server_*.zip)

[ "${#cli[@]}" -gt 0 ] || problem "no CLI archive (shift2eu_*) in ${dist}"
[ "${#server[@]}" -gt 0 ] || problem "no server archive (shift2eu-server_*) in ${dist}"

for archive in "${cli[@]}"; do
  check_archive "${archive}" shift2eu shift2eu-server
  # Every platform of the CLI has a server archive of the same version, OS and architecture.
  name="$(basename "${archive}")"
  twin="${dist}/shift2eu-server_${name#shift2eu_}"
  [ -f "${twin}" ] || problem "${archive}: no server archive for its platform, want ${twin}"
done
for archive in "${server[@]}"; do
  check_archive "${archive}" shift2eu-server shift2eu
  name="$(basename "${archive}")"
  twin="${dist}/shift2eu_${name#shift2eu-server_}"
  [ -f "${twin}" ] || problem "${archive}: no CLI archive for its platform, want ${twin}"
done

# One checksums.txt covers every archive.
for archive in "${cli[@]}" "${server[@]}"; do
  grep -qE "  $(basename "${archive}" | sed 's/\./\\./g')$" "${dist}/checksums.txt" ||
    problem "${dist}/checksums.txt does not list $(basename "${archive}")"
done
(cd "${dist}" && sha256sum --quiet -c checksums.txt) || problem "${dist}/checksums.txt does not match the archives"

if [ "${fail}" != 0 ]; then
  exit 1
fi
echo "${#cli[@]} CLI and ${#server[@]} server archives, each holding exactly its own binary, all in checksums.txt"
