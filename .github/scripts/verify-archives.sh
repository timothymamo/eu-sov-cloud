#!/usr/bin/env bash
# The dry run of the release's archives (issue #147), run by ci.yml's test job: it builds them with a
# snapshot run of .goreleaser.yaml, which publishes nothing, and fails unless each archive holds exactly
# what it should. The CLI's archives, shift2eu_<version>_<os>_<arch>, hold shift2eu (shift2eu.exe on
# Windows) and not shift2eu-server; the server's, shift2eu-server_<version>_<os>_<arch>, hold
# shift2eu-server (.exe) and not shift2eu. Besides its own binary an archive may hold only LICENSE,
# README.md, Markdown pages under docs/ (no path segment may start with a dot, so no ., .. or hidden
# file), and exactly the samples docs/guide/try-it.md runs on (issue #188): examples/README.md and, for
# each of aws, azure and gcp, examples/<source>/README.md and examples/<source>/inventory.json, with
# their directories, and exactly the jq programs docs/guide/inventory.md builds an inventory with (issue
# #178): docs/guide/inventory/<source>.jq for aws, azure and gcp. Any other entry fails the run, so a
# stray file under docs/ (a .jq file there that is not one of the three), or any file under
# examples/ that is not a known sample (a real inventory committed as examples/<name>/inventory.json),
# never ships unnoticed. Each archive must hold every sample and jq program too. Every entry must be a regular file or
# a directory: a symbolic link, hard link or device fails the run too. Every built
# platform has one archive of each, and checksums.txt lists and matches every archive. checksums.txt also
# lists, exactly once, shift2eu-server_openapi.json, the bundled OpenAPI document the release publishes
# beside the archives (issue #139), with the sha256 of the committed api/openapi.bundled.json. GoReleaser
# v2.20.1 copies neither checksum nor release extra files into dist/, so, as release.yml's goreleaser job
# does after GoReleaser, this copies the committed file into dist/ as shift2eu-server_openapi.json (a copy
# GoReleaser itself left there must already be that file), and checksums.txt must then pass sha256sum -c in
# dist/ as a whole.
# It then mirrors release.yml's hand-over of the archives to attest-archives (issues #139, #156 and #163):
# the files its patterns pick must pass sha256sum -c checksums.txt, and the subjects they attest must be
# exactly the archives, the OpenAPI document and checksums.txt, which must also be exactly what GoReleaser
# would upload to the release (artifacts.json, plus the OpenAPI document, which a snapshot never records
# there), or release.yml's publish job would refuse the draft. check.GoReleaserArchives pins the
# configuration itself, and check.GitHubRelease its draft release.
#
# GoReleaser writes into a temporary directory, removed on exit, so the caller's dist/ is left alone.
#
# Usage: .github/scripts/verify-archives.sh   (from the repository root, with goreleaser and jq on PATH)
set -euo pipefail

for tool in goreleaser jq; do
  command -v "${tool}" >/dev/null || { echo "${tool} is not on PATH" >&2; exit 1; }
done

# goreleaser release takes no --dist flag: it builds from a copy of .goreleaser.yaml whose dist: names a
# fresh temporary directory (the original sets no dist:, so its default, dist/, is never touched). Should
# .goreleaser.yaml ever set a top-level dist:, the appended one would duplicate it, so that fails here.
if grep -qE '^dist[[:space:]]*:' .goreleaser.yaml; then
  echo ".goreleaser.yaml sets a top-level dist:, which this dry run replaces: remove it or update this script" >&2
  exit 1
fi
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

# details prints an archive's long listing, one line per entry, each starting with the entry's mode,
# whose first character is its type: - for a regular file, d for a directory, l for a symbolic link, h
# for a hard link (tar) and so on. zipinfo prints a header and a trailer around the entries; the entries
# are its lines that start with a mode, and check_archive holds their count to the number of entries,
# so a listing this misreads fails the run rather than passing it.
details() {
  case "$1" in
    *.tar.gz) tar -tvzf "$1" ;;
    *.zip) unzip -Z -s "$1" | grep -E '^[-a-z?][-rwxsStTlL?]{6,9}[[:space:]]' ;;
    *) echo "$1 is neither .tar.gz nor .zip" >&2; return 1 ;;
  esac
}

# sift runs grep with its arguments but counts status 1, no line selected, as success, so a filter that
# leaves nothing is not an error. Any other failure (status 2 or more: a bad pattern, an unreadable file)
# is passed on, so a check built on it fails closed rather than reading an error as no match.
sift() {
  local status=0
  grep "$@" || status=$?
  if [ "${status}" -gt 1 ]; then
    echo "grep $* failed with status ${status}" >&2
    return "${status}"
  fi
  return 0
}

# check_archive archive own: the archive holds exactly one binary, own (or own.exe), at its root, and
# otherwise only LICENSE, README.md, Markdown pages under docs/, the inventory guide's jq programs
# (guide_programs) and the samples (with their directories, sample_dirs), each a regular file or a
# directory. The other binary is not on that list,
# so an archive holding it fails too. Under examples/ it may hold only the samples, and it must hold
# every one of them: one list, so a file there that is not a known sample fails the run.
check_archive() {
  local archive="$1" own="$2" list long special owns stray sample
  list="$(entries "${archive}")" || { problem "${archive}: cannot list its entries"; return; }
  long="$(details "${archive}")" || { problem "${archive}: cannot list its entries' types"; return; }
  if [ "$(wc -l <<<"${long}")" != "$(wc -l <<<"${list}")" ]; then
    problem "${archive}: lists $(wc -l <<<"${list}") entries but the types of $(wc -l <<<"${long}")"
  fi
  special="$(grep -v '^[-d]' <<<"${long}" || true)"
  if [ -n "${special}" ]; then
    problem "${archive}: holds entries that are neither a regular file nor a directory:"$'\n'"${special}"
  fi
  owns="$(grep -cxE "${own}(\\.exe)?" <<<"${list}" || true)"
  if [ "${owns}" != 1 ]; then
    problem "${archive}: holds ${owns} ${own} binaries at its root, want exactly 1"
  fi
  # Both greps run through sift and the pipeline's status is checked, so a grep that fails (status 2,
  # such as an unreadable allow list) fails the run rather than reading as no stray entries.
  if ! stray="$(sift -vxE "${own}(\\.exe)?|LICENSE|README\\.md|docs/([^/.][^/]*/)*[^/.][^/]*\\.md|docs/([^/.][^/]*/)*" <<<"${list}" | sift -vxFf "${allowed}")"; then
    problem "${archive}: cannot match its entries against the allow list (grep failed)"
    return
  fi
  if [ -n "${stray}" ]; then
    problem "${archive}: holds entries outside its allow list:"$'\n'"${stray}"
  fi
  for sample in "${samples[@]}" "${guide_programs[@]}"; do
    grep -qxF "${sample}" <<<"${list}" || problem "${archive}: does not hold ${sample}, which every archive must ship"
  done
}

# The samples docs/guide/try-it.md runs on (issue #188): every archive must hold each of them, and
# examples/ in an archive may hold nothing else but their directories (sample_dirs), so a new sample is
# added here as well as to .goreleaser.yaml.
samples=(examples/README.md)
sample_dirs=(examples/)
for source in aws azure gcp; do
  samples+=("examples/${source}/README.md" "examples/${source}/inventory.json")
  sample_dirs+=("examples/${source}/")
done
# The jq programs docs/guide/inventory.md runs (issue #178), which it tells users a release archive
# ships: every archive must hold each of them, and no other non-Markdown file under docs/ is allowed, so
# a new program is added here as well as under docs/guide/inventory/.
guide_programs=()
for source in aws azure gcp; do
  guide_programs+=("docs/guide/inventory/${source}.jq")
done
# The allow list beyond the Markdown pages, one exact path a line, for grep -f: a file, not a process
# substitution, whose status $? would never see.
allowed="${work}/allowed-files"
printf '%s\n' "${samples[@]}" "${sample_dirs[@]}" "${guide_programs[@]}" >"${allowed}"

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

# ... and the bundled OpenAPI document (issue #139): checksum.extra_files lists it under its release name
# with the sum of the committed file, which GoReleaser reads in place rather than copying into dist/.
bundle=api/openapi.bundled.json
bundle_asset=shift2eu-server_openapi.json
want_sum="$(sha256sum "${bundle}" | cut -d' ' -f1)"
bundle_lines="$(grep -E "  ${bundle_asset//./\\.}$" "${dist}/checksums.txt" || true)"
if [ "$(grep -c . <<<"${bundle_lines}")" != 1 ]; then
  problem "checksums.txt lists ${bundle_asset} $(grep -c . <<<"${bundle_lines}") times, want exactly once"
elif [ "${bundle_lines%% *}" != "${want_sum}" ]; then
  problem "checksums.txt lists ${bundle_asset} with ${bundle_lines%% *}, want ${want_sum}, the sha256 of ${bundle}"
fi
# GoReleaser v2.20.1 copies neither checksum.extra_files nor release.extra_files into dist/, and the
# release pipe does not run in a snapshot, so no copy exists today. Should a later GoReleaser put one
# there, it must already be the committed file: release.yml's cp replaces it with the file it attests,
# while GoReleaser uploads its own.
if [ -e "${dist}/${bundle_asset}" ] && [ "$(sha256sum "${dist}/${bundle_asset}" | cut -d' ' -f1)" != "${want_sum}" ]; then
  problem "GoReleaser left a ${dist}/${bundle_asset} that is not ${bundle}"
fi
# The copy release.yml's goreleaser job makes, the step Put the OpenAPI document beside the archives:
# from here on dist/ holds what that job hands on, and every line of checksums.txt is a file in it.
cp "${bundle}" "${dist}/${bundle_asset}"
(cd "${dist}" && sha256sum --quiet -c checksums.txt) ||
  problem "checksums.txt does not match the archives and ${bundle_asset} in dist/"

# The release's hand-over of the archives (issues #156 and #163), on this snapshot: release.yml's
# goreleaser job uploads the files its patterns match, dist/shift2eu_*.tar.gz, dist/shift2eu_*.zip,
# dist/shift2eu-server_*.tar.gz, dist/shift2eu-server_*.zip, the OpenAPI document it copied there,
# dist/shift2eu-server_openapi.json, and dist/checksums.txt, as the workflow
# artifact archives; attest-archives downloads it, runs sha256sum -c checksums.txt in it and attests
# every file the same patterns match, directories descended into, as actions/upload-artifact and
# actions/attest-build-provenance both expand them. A bare dist/shift2eu_* would also match
# GoReleaser's build directories (dist/shift2eu_linux_amd64_v1/ ...) and hand on their binaries.
# The artifact must pass sha256sum -c, so checksums.txt may list nothing the patterns leave out, and the
# subjects must be exactly the files checksums.txt lists (the archives and the OpenAPI document) and
# checksums.txt itself: a file left unattested, or one attested that the release never ships, fails here
# and not in a release.
artifact="${work}/artifact"
mkdir "${artifact}"
(cd "${dist}" && cp -R shift2eu_*.tar.gz shift2eu_*.zip shift2eu-server_*.tar.gz shift2eu-server_*.zip "${bundle_asset}" checksums.txt "${artifact}/")
(cd "${artifact}" && sha256sum --quiet -c checksums.txt) ||
  problem "checksums.txt does not pass sha256sum -c in the archives artifact release.yml hands to attest-archives"
subjects="$(cd "${artifact}" && find shift2eu_*.tar.gz shift2eu_*.zip shift2eu-server_*.tar.gz shift2eu-server_*.zip "${bundle_asset}" checksums.txt -type f | LC_ALL=C sort)"
want="$({ echo checksums.txt; awk '{ print $2 }' "${dist}/checksums.txt"; } | LC_ALL=C sort)"
if [ "${subjects}" != "${want}" ]; then
  problem "attest-archives would attest these files:"$'\n'"${subjects}"$'\n'"want exactly checksums.txt and the files it lists:"$'\n'"${want}"
fi

# What GoReleaser would upload to the draft release, read from its own record of the run: every
# artifact but its run metadata and the bare binaries its builds wrote (named by the binary and its
# extension), which it never uploads. That must be exactly the attested subjects too: the publish job
# refuses a draft holding anything checksums.txt does not list (issue #163), so a new asset registered
# by a pipe a snapshot runs (an SBOM, a signature, a source archive, checksum.extra_files) fails here,
# before a release is left a draft. release.extra_files does not: GoReleaser adds those only in the
# release pipe's publish step, which --snapshot skips, so artifacts.json never lists them. release: is a
# closed section that check.GitHubRelease lets hold only the bundled OpenAPI document there (issue #139),
# so that one file, and nothing else, is added to the list here by its asset name: should artifacts.json
# ever list it too, it is listed twice and fails. It fails closed: an artifact type this does not know
# counts as uploaded.
uploads="$({
  jq -r '.[]
    | select(.type != "Metadata")
    | select((.type == "Binary" and .name == (.extra.Binary + .extra.Ext)) | not)
    | .name' "${dist}/artifacts.json"
  echo "${bundle_asset}"
} | LC_ALL=C sort)"
if [ "${uploads}" != "${want}" ]; then
  problem "GoReleaser would upload these files to the release:"$'\n'"${uploads}"$'\n'"want exactly checksums.txt and the files it lists, what attest-archives attests:"$'\n'"${want}"
fi

if [ "${fail}" != 0 ]; then
  exit 1
fi
echo "${#cli[@]} CLI and ${#server[@]} server archives, one per built platform, each holding exactly its own binary, LICENSE, README.md, docs/**/*.md, the inventory guide's jq programs and exactly the samples under examples/, all in checksums.txt with ${bundle_asset}"
