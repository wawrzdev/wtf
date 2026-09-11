# Configuration

The base file is `$XDG_CONFIG_HOME/wtf/config.toml`. Files matching `conf.d/*.toml` are optional;
each fragment must contain a unique integer `priority`. Fragments apply from lowest to highest
priority. Array and scalar fields replace the preceding value. Invalid and unknown values identify
their source file.

```toml
[annotations]
paths = ["~/.zshrc", "~/.local/bin", "~/.local/share/wtf/annotations"]

[topics]
dirs = ["~/.local/share/wtf/topics"]
write_dir = "~/.local/share/wtf/topics"
post_write = ["chezmoi", "add", "--", "{path}"]

[providers.cheat]
enabled = true
```

`~`, `$HOME`, `$XDG_CONFIG_HOME`, `$XDG_DATA_HOME`, and `$XDG_CACHE_HOME` expand in paths. The
optional `pager` root key configures the plain renderer's pager. `mdcat`, when available, owns
pagination and follows its normal `MDCAT_PAGER`/`PAGER` behavior.

Markdown annotation and topic files use YAML-like scalar frontmatter with `schema`, `id`, `kind`,
`summary`, and optional `title`, followed by Markdown. Schema 1 accepts `command` and `topic` kinds.
Source files can also contain `# @tool`, `# @func`, `# @alias`, and `# @detail` comments. Later
configured sources overlay only fields they supply; a later nonempty body replaces an earlier body.

Topic writes invoke `post_write` directly as an argument vector after a changed, valid save.
`{path}` is substituted in each argument. No shell evaluates the hook.
