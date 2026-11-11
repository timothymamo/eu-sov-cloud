# 0017. Run a rootless container runtime (nerdctl-full) and cosign in the Lima VM

- Status: Accepted
- Date: 2026-11-11

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
`dev/lima.yaml` with a `# renovate:` comment, and every download is checked against its release's
checksums (crane, through `go install`, against the Go checksum database, as govulncheck is).
nerdctl-full's `SHA256SUMS` must also carry a valid GPG signature by a release manager pinned by full
fingerprint in `dev/lima.yaml`, as nerdctl's `MAINTAINERS` lists them (Akihiro Suda, Chengyu Zhu).

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
- **Integrity, not authenticity, for gh and cosign.** Their checksum files come from the same release
  as the binaries, so they catch a corrupt download, not a replaced release. Only nerdctl-full's are
  signed by a key pinned here. Follow-ups: pin per-architecture digests that Renovate updates, and verify
  `cosign_checksums.txt` keylessly with cosign's own signature. A nerdctl release signed by a manager not
  yet pinned fails provisioning until their fingerprint is checked against `MAINTAINERS` and added.
- Images, BuildKit's cache and containerd's state take space on the VM's 40 GiB disk, under
  `~/.local/share`.
- In a rootless runtime, container UID 0 is the VM user, not root. A container run with
  `--user "$(id -u):$(id -g)"` is a subordinate UID and cannot read the VM user's `0600` files; use
  `--user 0:0` there. The README says so.
- An existing VM keeps the template it was created with: it has to be recreated to get any of this.
- Renovate bumps nerdctl, cosign, gh and crane like the other `param:` versions.
