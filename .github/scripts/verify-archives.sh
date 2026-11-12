#!/usr/bin/env bash
# The dry run of the release's archives (issue #147), run by ci.yml's test job: it builds them with a
# snapshot run of .goreleaser.yaml, which publishes nothing, and fails unless each archive holds exactly
# what it should. The CLI's archives, shift2eu_<version>_<os>_<arch>, hold shift2eu (shift2eu.exe on
# Windows) and not shift2eu-server; the server's, shift2eu-server_<version>_<os>_<arch>, hold
# shift2eu-server (.exe) and not shift2eu. Besides its own binary an archive may hold only LICENSE,
# README.md and Markdown pages under docs/: any other entry fails the run, so a stray file under docs/
# never ships unnoticed. Every built platform has one archive of each, and checksums.txt lists and
# matches every archive. check.GoReleaserArchives pins the configuration itself.
#
# GoReleaser writes into a temporary directory, removed on exit, so the caller's dist/ is left alone.
#
# Usage: .github/scripts/verify-archives.sh   (from the repository root, with goreleaser and jq on PATH)
set -euo pipefail

for tool in goreleaser jq; do
  command -v "${tool}" >/dev/null || { echo "${tool} is not on PATH" >&2; exit 1; }
done

# goreleaser release takes no --dist flag: it builds from a copy of .goreleaser.yaml whose dist: names a
# fresh temporary directory (the original sets no dist:, so its default, dist/, is never touched).
work="$(mktemp -d)"
trap 'rm -rf "${work}"' EXIT
dist="${work}/dist"
config="${work}/goreleaser.yaml"
cp .goreleaser.yaml "${config}"
printf '\ndist: %s\n' "${dist}" >>"${config}"
goreleaser release --snapshot --clean --config "${config}" >&2

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

# check_archive archive own: the archive holds exactly one binary, own (or own.exe), at its root, and
# otherwise only LICENSE, README.md and Markdown pages under docs/ (with their directories). The other
# binary is not on that list, so an archive holding it fails too.
check_archive() {
  local archive="$1" own="$2" list owns stray
  list="$(entries "${archive}")" || { problem "${archive}: cannot list its entries"; return; }
  owns="$(grep -cxE "${own}(\\.exe)?" <<<"${list}" || true)"
  if [ "${owns}" != 1 ]; then
    problem "${archive}: holds ${owns} ${own} binaries at its root, want exactly 1"
  fi
  stray="$(grep -vxE "${own}(\\.exe)?|LICENSE|README\\.md|docs/([^/]+/)*[^/]+\\.md|docs/([^/]+/)*" <<<"${list}" || true)"
  if [ -n "${stray}" ]; then
    problem "${archive}: holds entries outside its allow list:"$'\n'"${stray}"
  fi
}

shopt -s nullglob
cli=("${dist}"/shift2eu_*.tar.gz "${dist}"/shift2eu_*.zip)
server=("${dist}"/shift2eu-server_*.tar.gz "${dist}"/shift2eu-server_*.zip)

# One archive per built platform of each binary: the platforms are those GoReleaser built, read from
# its own record of the run, so a platform dropped from both builds (.goreleaser.yaml, releasetarget)
# still changes the count it is held to. Each build must have built at least one.
built() {
  jq --arg id "$1" '[.[] | select(.type == "Binary" and .extra.ID == $id)] | length' "${dist}/artifacts.json"
}
cli_platforms="$(built shift2eu)"
server_platforms="$(built shift2eu-server)"
[ "${cli_platforms}" -gt 0 ] || problem "GoReleaser built no shift2eu binary"
[ "${cli_platforms}" = "${server_platforms}" ] ||
  problem "GoReleaser built shift2eu for ${cli_platforms} platforms and shift2eu-server for ${server_platforms}"
[ "${#cli[@]}" = "${cli_platforms}" ] ||
  problem "${#cli[@]} CLI archives (shift2eu_*), want one per built platform: ${cli_platforms}"
[ "${#server[@]}" = "${server_platforms}" ] ||
  problem "${#server[@]} server archives (shift2eu-server_*), want one per built platform: ${server_platforms}"

for archive in "${cli[@]}"; do
  check_archive "${archive}" shift2eu
  # Every platform of the CLI has a server archive of the same version, OS and architecture.
  name="$(basename "${archive}")"
  twin="${dist}/shift2eu-server_${name#shift2eu_}"
  [ -f "${twin}" ] || problem "${archive}: no server archive for its platform, want ${twin}"
done
for archive in "${server[@]}"; do
  check_archive "${archive}" shift2eu-server
  name="$(basename "${archive}")"
  twin="${dist}/shift2eu_${name#shift2eu-server_}"
  [ -f "${twin}" ] || problem "${archive}: no CLI archive for its platform, want ${twin}"
done

# One checksums.txt covers every archive.
for archive in "${cli[@]}" "${server[@]}"; do
  grep -qE "  $(basename "${archive}" | sed 's/\./\\./g')$" "${dist}/checksums.txt" ||
    problem "checksums.txt does not list $(basename "${archive}")"
done
(cd "${dist}" && sha256sum --quiet -c checksums.txt) || problem "checksums.txt does not match the archives"

if [ "${fail}" != 0 ]; then
  exit 1
fi
echo "${#cli[@]} CLI and ${#server[@]} server archives, one per built platform, each holding exactly its own binary, LICENSE, README.md and docs/**/*.md, all in checksums.txt"
