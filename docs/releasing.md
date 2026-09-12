# Releasing and package handoff

Release tags must be stable `vMAJOR.MINOR.PATCH` versions on commits already merged to the
current default branch. GoReleaser builds four macOS/Linux archives and four Debian/Arch packages,
uploading them plus `checksums.txt` to a **draft**. Before publishing, the workflow checks the exact
nine uploaded names, sizes, and SHA-256 digests against local artifacts and the checksum file.
Release runs are serialized across tags and reject versions no newer than the current latest.
Only then is the draft published and made immutable by the repository's enabled immutability setting.
`release.mode` is a release-notes policy, not an immutability control; see
[GoReleaser release behavior](https://goreleaser.com/customization/publish/scm/) and
[GitHub immutable-release publishing](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases).

A separate, checkout-free `dispatch` job verifies final immutable metadata and sends
`wtf-release-published` to `wawrzdev/packages`. Configure a `package-dispatch` environment in this
source repository, restricted to `v*` tags, with variable `PACKAGES_APP_CLIENT_ID` and secret
`PACKAGES_APP_PRIVATE_KEY`. Use the existing GitHub App installed **only on `wawrzdev/packages`**;
it does not need installation access to this source repository. The job requests a short-lived
installation token restricted to `packages` with Contents write permission only. Source release
lookup/publication uses the normal workflow token; the App key is never present during checkout,
builds, tests, or draft publication. No PAT is required.

If dispatch fails after publication, fix the environment and rerun only the failed dispatch job.
Do not rebuild or replace immutable release assets. The packages repository's scheduled or manual
reconciliation can also discover a published release without a successful dispatch. If draft
validation fails, leave it unpublished, correct the cause, and inspect/remove the failed draft
before retrying. Feed construction, PR auto-merge, signing, and deployment belong to `packages`.

Before tagging, run the development checks, `goreleaser check`, and a local snapshot:

```sh
go fmt ./...
go test ./...
go vet ./...
go test -race ./...
goreleaser check
goreleaser release --snapshot --clean
```

Confirm that `completions/wtf.bash`, `completions/wtf.zsh`, and `completions/wtf.fish` match the
runtime output. A release tag is the only publishing trigger; ordinary pushes and pull requests run
tests without publishing.
