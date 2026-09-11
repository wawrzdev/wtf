# Releasing and package handoff

Tagged releases are automated. Maintainers create and push a reviewed `v*` tag; the release
workflow runs GoReleaser, publishes checksummed macOS/Linux archives and Debian/Arch packages, then
notifies `wawrzdev/packages` with a `wtf-release-published` repository dispatch.

The workflow requires the repository secret `PACKAGES_DISPATCH_TOKEN`, scoped to create repository
dispatches in `wawrzdev/packages`. Its payload identifies the immutable source commit, GitHub
release ID and tag, checksum asset ID and digest, checksum download URL, and release URL. The
dispatch fails when the release lacks `checksums.txt` or native package artifacts. The packages
repository verifies those immutable identifiers before updating its indexes.

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
