# wtf

`wtf` is a local-first command discovery and documentation CLI. It combines commands installed by
supported package managers with source-adjacent annotations, authored Markdown topics, local tldr
pages, installed-version manual/help output, and eligible public cheat.sh pages. The first release
contains no AI commands.

```console
$ wtf                 # open the fzf picker
$ wtf rg              # show the unique composed document
$ wtf list            # stable TSV when redirected
$ wtf list --json     # stable structured records
$ wtf topic new "Git staging"
$ eval "$(wtf init zsh)"
```

## Install

`wtf` requires Go 1.24 or newer when installing from source and requires `fzf` for interactive
pickers. `mdcat` is optional and supplies rich terminal rendering and pagination.

```sh
go install github.com/wawrzdev/wtf@latest
```

Release archives and native Debian and Arch packages are also published from tagged releases. The
built-in renderer keeps redirected output plain and portable when `mdcat` is absent.

Configuration is read from `$XDG_CONFIG_HOME/wtf/config.toml`, with `$XDG_CONFIG_HOME` defaulting
to `~/.config`, followed by uniquely prioritized `conf.d/*.toml` fragments. Authored data uses
`$XDG_DATA_HOME` (default `~/.local/share`) and disposable caches use `$XDG_CACHE_HOME` (default
`~/.cache`). See [Configuration](docs/configuration.md),
[Providers and cache](docs/providers.md), and the [CLI reference](docs/reference.md).
Release maintainers should also read [Releasing and package handoff](docs/releasing.md).

## Development

```sh
go fmt ./...
go test ./...
go vet ./...
go test -race ./...
```

Tests use isolated temporary homes and local HTTP servers. They do not query public networks or
write operator home state.

Checked completion files are regenerated with `go generate ./...`; tests fail when they differ
from `wtf completion zsh|bash|fish`.

## License

MIT
