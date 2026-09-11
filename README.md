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

Go 1.24 or newer builds the binary with `go build ./...`. `fzf` is required for pickers. `mdcat` is
optional and supplies rich terminal rendering and pagination when installed. The built-in renderer
keeps redirected output plain and portable.

Configuration is read from `$XDG_CONFIG_HOME/wtf/config.toml`, followed by uniquely prioritized
`$XDG_CONFIG_HOME/wtf/conf.d/*.toml` fragments. See [Configuration](docs/configuration.md),
[Providers and cache](docs/providers.md), and the [CLI reference](docs/reference.md).

## Development

```sh
go fmt ./...
go test ./...
go vet ./...
go test -race ./...
```

Tests use isolated temporary homes and local HTTP servers. They do not query public networks or
write operator home state.

## License

MIT
