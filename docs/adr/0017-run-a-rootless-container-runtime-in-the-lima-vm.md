# 0017. Run a rootless container runtime (nerdctl-full) and cosign in the Lima VM

- Status: Accepted
- Date: 2026-11-11
- Amended: 2026-11-11, how gh and cosign are verified (#100; see "Verifying gh and cosign"), then where
  their authenticity comes from (#100 review; see "Amendment: authenticity is checked in CI")

## Context

The Lima VM is the agents' isolation boundary ([ADR 0002](0002-lima-vm-for-agent-isolation.md)). It had
no container runtime and no cosign, so nothing in it could verify a release image
([ADR 0015](0015-publish-the-signed-image-to-ghcr-with-docker-buildx.md)), run it, or build the
`Dockerfile` (#95). The `release-tag` skill handed its last step, checking the published image, to a
human on the Mac, and changes to the `Dockerfile` could be proven only by CI.

Verifying needs cosign v3 or later and `gh attestation verify`, which Ubuntu's `gh` package (2.46) does
not have: it arrived in gh 2.49. Resolving a tag to its digest needs a registry client. Building needs
BuildKit for `linux/amd64` and `linux/arm64`.

A container runtime changes what the VM user can do. Membership of a rootful Docker daemon's `docker`
group is root on the VM: anyone in it can start a privileged container that mounts `/`. Mounting the
Mac's Docker Desktop socket would hand the agents a daemon outside the VM altogether.

## Decision

Install **nerdctl-full** (nerdctl, containerd, BuildKit, RootlessKit, runc, CNI plugins) from its
GitHub release into `/usr/local/nerdctl`, and run containerd and BuildKit **rootless**, as systemd user
services of the VM user, set up by `containerd-rootless-setuptool.sh`. Install **cosign**, a release
**gh** and **crane** beside it, root-owned in `/usr/local/bin`. Every version is a `param:` in
`dev/lima.yaml` with a `# renovate:` comment, and every download is authenticated, not only checked
against a checksum file from the same release:

- **nerdctl-full**: its `SHA256SUMS` must carry a valid GPG signature by a release manager pinned by
  full fingerprint in `dev/lima.yaml`, as nerdctl's `MAINTAINERS` lists them (Akihiro Suda, Chengyu Zhu).
- **cosign**: pinned by a SHA-256 per architecture in `dev/lima.yaml`, then checked against its own
  keyless signature (see below).
- **gh**: pinned by a SHA-256 per architecture in `dev/lima.yaml` (see below).
- **crane**: through `go install`, against the Go checksum database, as govulncheck is.

- **No rootful anything.** No root daemon, no `docker` group, no socket a VM process could use to
  become root. This adds nothing only because the VM user already has passwordless `sudo`. Lima's own
  `containerd:` stays off: its `user: true` mode is also rootless, but installs
  the nerdctl-full version the host's Lima bundles, which Renovate cannot pin.
- **One AppArmor profile.** Ubuntu lets only programs with an AppArmor profile create user namespaces
  (`kernel.apparmor_restrict_unprivileged_userns=1`), and its own `rootlesskit` profile covers only
  `/usr/bin/rootlesskit`. `dev/lima.yaml` adds the same unconfined `userns` profile for
  `/usr/local/nerdctl/bin/rootlesskit`, which is the profile RootlessKit itself suggests.
- **No host mounts or sockets.** Nothing new is shared with the Mac.
- **Read-only registry access.** No `nerdctl login`, `docker login` or credential helper is configured;
  the image is public, pulls are anonymous, and only `release.yml` pushes.
- **No emulation.** The `Dockerfile` cross-compiles in a `$BUILDPLATFORM` stage and its final stage has
  no `RUN`, so building for both platforms needs no QEMU. Registering QEMU with `binfmt_misc` needs root,
  so the VM does not; it runs only its own platform's image.
- **Digest by crane.** `nerdctl` has no `imagetools inspect`, and resolving the digest through
  `nerdctl pull` would download the image before it is verified. `crane digest` reads only the index.
  The README gives the Docker and the VM form wherever they differ, and `make image` runs `docker` or,
  where there is none, `nerdctl` (`DOCKER`), for the platforms in `PLATFORMS`.

## Alternatives considered

- **Rootless Docker.** The README's commands would run unchanged, `docker buildx imagetools inspect`
  included. But Docker's static binaries publish no checksums, so it would come from Docker's apt
  repository, whose `docker-ce` package installs and starts a rootful daemon and creates the `docker`
  group, both to be disabled after the fact; its version string is distribution-specific and has no
  Renovate datasource the `# renovate:` comments can name.
- **Lima's `containerd: user: true`.** Rootless and set up by Lima, but versioned by the host's Lima,
  not by `dev/lima.yaml`.
- **Rootful Docker or containerd and a `docker` group.** Root on the VM for every agent. Rejected.
- **Docker Desktop's socket mounted from the Mac.** Breaks ADR 0002: the agents would control a daemon
  on the host. Rejected.
- **Podman.** Rootless and in Ubuntu's archive, but at Ubuntu's version, and its `podman build` is
  Buildah, not the BuildKit CI uses.
- **cosign and gh only, no runtime.** Verifies the image, but cannot run or build it, which #95 needs.

## Consequences

- An agent can verify, pull, run and build images as the VM user, and the `release-tag` skill checks
  the image itself.
- **The isolation trade-off.** Rootless containers run as the VM user and subordinate UIDs, so a
  container escape gains no more than the VM user already has, which is root through passwordless
  `sudo`. But RootlessKit now creates user namespaces, which Ubuntu restricts because many kernel
  privilege escalations start from one, and the profile lets any process in the VM run RootlessKit and so
  create user namespaces (Ubuntu's own profile for `/usr/bin/rootlesskit` is identical). `uidmap` adds
  the setuid-root `newuidmap` and `newgidmap`. If `sudo` is ever removed from the VM user, this profile
  and `uidmap` must be revisited: they would then be a path to root. The VM is still the boundary: none
  of this reaches the Mac.
- **The in-VM image check is a smoke test, not the trust anchor.** The `release-tag` skill calls the
  root-owned binaries by absolute path, but every agent shares the VM user, which has passwordless
  `sudo`, so any of them can tamper with the check. An independent check on a GitHub-hosted runner, a
  post-publish job in `release.yml`, is the follow-up that makes it authoritative; #95 does not change
  `release.yml`.
- **Provisioning trusts the gh and cosign pins**, and the `vm-tools` workflow, run on every pull
  request, is what makes each pin authentic; it holds only if pins are merged with that check green,
  which GitHub does not yet enforce (see "Amendment: authenticity is checked in CI"). A nerdctl release
  signed by a manager not yet pinned fails provisioning until their fingerprint is checked against `MAINTAINERS` and added.
- Images, BuildKit's cache and containerd's state take space on the VM's 40 GiB disk, under
  `~/.local/share`.
- In a rootless runtime, container UID 0 is the VM user, not root. A container run with
  `--user "$(id -u):$(id -g)"` is a subordinate UID and cannot read the VM user's `0600` files; use
  `--user 0:0` there. The README says so.
- An existing VM keeps the template it was created with: it has to be recreated to get any of this.
- Renovate bumps nerdctl, cosign, gh and crane like the other `param:` versions.

## Verifying gh and cosign

The first version of this decision checked gh and cosign only against the checksum file published in
the same release. That proves the download is intact, not that it is authentic: whoever can replace the
binary can replace the checksum file too (#100, raised by the audit of #96).

- **cosign: pinned digest, then its own signature.** `dev/lima.yaml` pins the SHA-256 of
  `cosign-linux-amd64` and `cosign-linux-arm64` (`cosignSha256Amd64`, `cosignSha256Arm64`). There is no
  trusted cosign to verify the first one with, so the pin is what lets the downloaded binary run at all.
  It then verifies itself: `cosign verify-blob` with the release's Sigstore bundle
  (`cosign-linux-<arch>.sigstore.json`), which must hold a Fulcio certificate for the identity
  sigstore/cosign signs its releases with, `keyless@projectsigstore.iam.gserviceaccount.com` issued by
  `https://accounts.google.com` (cosign's installation docs), and a Rekor inclusion proof. The
  certificate in the v3 bundles carries that identity, not a GitHub Actions workflow identity, so
  `https://token.actions.githubusercontent.com` would reject every release. This self-check guards
  integrity and the bundle's consistency only, not authenticity: a replaced binary can simply exit 0, and
  the TUF trust root it checks against is embedded in the binary under test. In provisioning the pin is
  the trust anchor; the pin is made authentic in CI (see the amendment below). The check runs with
  `HOME` and `TUF_ROOT` under its temporary directory, like `GNUPGHOME` for nerdctl. With the v0.3
  bundles cosign publishes, the Rekor inclusion proof and signed entry timestamp are checked offline from
  the bundle; the only network it needs is Sigstore's TUF repository, `tuf-repo-cdn.sigstore.dev`.
- **gh: pinned digest.** `dev/lima.yaml` pins the SHA-256 of `gh_<version>_linux_<arch>.tar.gz`
  (`ghSha256Amd64`, `ghSha256Arm64`). gh publishes no Sigstore bundle with its release; its build
  provenance is a GitHub artifact attestation, fetched from the GitHub API. `gh attestation verify`
  needs a gh that is already trusted and a GitHub token, and system provisioning has neither (the token
  arrives later, through `make vm-env`, and only for the VM user). Fetching the attestation
  anonymously and checking it with `cosign verify-blob-attestation` was tried: cosign v3.1.4 crashes on
  that bundle, and the anonymous API allows 60 requests an hour. The pins are simple and need nothing
  but the download. After provisioning, `gh attestation verify` can confirm the release by hand
  (`docs/dev-environment.md`).
- **Fail closed, with a clear message.** A digest that differs, or is not set for the VM's
  architecture, stops provisioning (`exit 1` under `set -e`) with the file, both digests and the param to
  fix, before anything is installed; so does a cosign that fails its signature check. Each download runs
  in a subshell whose `EXIT` trap removes its temporary directory, so a rejected file is not left behind.
- **Renovate moves the pins with the version.** Each digest has its own
  `# renovate: datasource=github-release-attachments depName=<repo> digestVersion=<tag>` comment, read
  by a second regex custom manager in `.github/renovate.json5`. Renovate finds the pinned asset through
  the `digestVersion` release's checksum file, reads the same asset's digest from the new release's
  checksum file, and rewrites `digestVersion` and the digest; a `packageRules` group puts the version and
  both digests in one PR (`renovate/gh`, `renovate/cosign`). A dry run from v2.103.2 and v3.1.3 proposed
  exactly the digests pinned here. The new digest still comes from the new release's checksum file, so a
  Renovate PR by itself adds review, not authenticity, for either tool; the `vm-tools` check on that PR
  supplies it. If Renovate cannot map a digest, it leaves it unchanged while the version moves, and
  provisioning then fails with the message above: a bump never installs an unpinned binary.
- **Only on install.** As every step does, these run when the installed version differs from the pinned
  one. An existing VM keeps the binaries it has; recreate it to have them verified.

### Alternatives considered

- **Keep the checksum files.** Integrity only. Rejected (#100).
- **Verify `cosign_checksums.txt` with its bundle instead of the binary.** Equivalent, one step longer;
  the binary's own bundle verifies what is installed.
- **gh by `gh attestation verify` in provisioning.** Needs a trusted gh and a token; see above.
- **gh by `cosign verify-blob-attestation` of the API's attestation.** Fails on cosign v3.1.4 and is
  rate limited; revisit if gh starts publishing a Sigstore bundle with its release assets.

## Amendment: authenticity is checked in CI

The security audit of the first version found that cosign's self-check gives no authenticity: the
binary that is checked runs the check, with the trust root it carries. The real anchor was the pinned
digest, which Renovate copies from the same release's checksum file: the weakness #100 is about. Nothing
automated ran `gh attestation verify` either.

- **`.github/workflows/vm-tools.yml` verifies the pins on every pull request; merge only when it is
  green.** It is not yet enforced as a required status (see the last bullet). It runs on every pull
  request, whatever it changes, so it can be required without leaving other PRs waiting for a check that
  never starts; on each push to `main` that changes `dev/lima.yaml` or the workflow; and weekly. It reads
  `cosignVersion`, `ghVersion` and the four digests as YAML from the top-level `param:` key of
  `dev/lima.yaml`, where Lima reads them, not by matching lines (a same-named key elsewhere in the file
  would fool that), and rejects a missing or non-string value, a digest that is not 64 lowercase hex
  digits and a version that is not `v<major>.<minor>.<patch>`. Then, for amd64 and arm64, it:
  verifies `cosign-linux-<arch>` against its Sigstore bundle with the cosign of
  `sigstore/cosign-installer`, which that action pins by digest in its own source, for
  `keyless@projectsigstore.iam.gserviceaccount.com` issued by `https://accounts.google.com`; verifies
  `gh_<version>_linux_<arch>.tar.gz` with the runner's preinstalled gh, `gh attestation verify -R
  cli/cli --signer-workflow cli/cli/.github/workflows/deployment.yml --source-ref refs/heads/trunk
  --source-digest <commit> --deny-self-hosted-runners`; then requires each verified
  file's SHA-256 to equal its pin, and fails naming the param otherwise. Neither verifier comes from
  the release under test. Its permissions are `contents: read` and `attestations: read`.
- **gh's attestation is bound to the release by its commit, not its tag.** cli/cli builds a release by
  dispatching `deployment.yml` on `trunk` (`workflow_dispatch`), so the certificate's source ref is
  `refs/heads/trunk` for every release; `--source-ref refs/tags/<tag>` rejects them all (checked against
  v2.103.3 and v2.103.2). What ties the attestation to the pinned version is its source digest, the
  commit the build ran on: the job resolves the tag to its commit through the GitHub API
  (`repos/cli/cli/commits/refs/tags/<tag>`; `v2.103.3` is `63837fb7`, the attestation's
  `sourceRepositoryDigest`) and requires it with `--source-digest`. So an attestation for some other
  build of cli/cli, from another commit or a self-hosted runner, does not verify a pinned asset.
- **So the trust chain is:** the CI check proves the pinned digest is the publisher's signed asset;
  review merges it only with that check green (the Renovate PRs say so); provisioning installs only that
  digest. The self-check in provisioning stays, for integrity and bundle consistency, and claims no more.
- **The weekly run catches a replaced asset.** Renovate's `github-release-attachments` datasource keeps
  the pinned digest while the tag is unchanged, so it never reports an asset replaced under a pinned
  tag. The scheduled run then fails on the pin. A `packageRules` entry for `digest` updates of gh and
  cosign (label `security-review`, no automerge, a warning in the PR) is kept in case Renovate starts
  proposing them.
- **Not yet a required status.** `main`'s ruleset requires only Lint, Test, govulncheck and CodeQL, so
  until it changes, a green VM tools check is a merge rule for the reviewer, not something GitHub
  enforces. Follow-up for the user: add the status check `Verify the gh and cosign pins` (the job's
  `name`, which is the context GitHub reports) to the required checks of `main`'s ruleset.

### Alternatives considered

- **Chain of trust: verify the next cosign with the previously installed one.** Rejected. Lima copies
  the template when it creates a VM, so a Renovate bump reaches only new VMs, which have no previous
  cosign: the chain would start from the pin on almost every install. Where a previous cosign exists,
  the VM user can replace it (passwordless `sudo`), so it is no more trustworthy than the pin, and it
  adds a path where a bad old binary blesses a new one. The CI job is that chain, run once, on a
  machine the agents cannot touch.
- **Trust the self-check, as the first version did.** Rejected: it proves nothing a replaced binary
  cannot fake.
